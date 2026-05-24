package transform

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

type flusher interface {
	Flush()
}

func flushIfSupported(w io.Writer) {
	if f, ok := w.(flusher); ok {
		f.Flush()
	}
}

// PipeOpenAIChatStreamToAnthropic converts OpenAI chat completion SSE to Anthropic message stream events.
func PipeOpenAIChatStreamToAnthropic(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var textBuf strings.Builder
	messageStart := false
	blockOpen := false
	nextIndex := 0
	toolIndexes := map[float64]int{}

	writeEvent := func(eventType string, data map[string]any) error {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, "event: "+eventType+"\n")
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, "data: "+string(b)+"\n\n")
		flushIfSupported(w)
		return err
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			if messageStart {
				if blockOpen {
					_ = writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": nextIndex - 1})
				}
				_ = writeEvent("message_delta", map[string]any{
					"type":  "message_delta",
					"delta": map[string]any{"stop_reason": "end_turn"},
				})
				_ = writeEvent("message_stop", map[string]any{"type": "message_stop"})
				messageStart = false
				blockOpen = false
			}
			break
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}
		if !messageStart {
			messageStart = true
			_ = writeEvent("message_start", map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id":      str(chunk["id"]),
					"type":    "message",
					"role":    "assistant",
					"model":   model,
					"content": []any{},
				},
			})
		}
		if t, ok := delta["content"].(string); ok && t != "" {
			if !blockOpen {
				blockOpen = true
				_ = writeEvent("content_block_start", map[string]any{
					"type":          "content_block_start",
					"index":         nextIndex,
					"content_block": map[string]any{"type": "text", "text": ""},
				})
				nextIndex++
			}
			textBuf.WriteString(t)
			_ = writeEvent("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": nextIndex - 1,
				"delta": map[string]any{"type": "text_delta", "text": t},
			})
		}
		for _, tcm := range asMapSlice(delta["tool_calls"]) {
			idx, _ := tcm["index"].(float64)
			outIdx, ok := toolIndexes[idx]
			fn, _ := tcm["function"].(map[string]any)
			if !ok {
				if blockOpen {
					_ = writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": nextIndex - 1})
					blockOpen = false
				}
				outIdx = nextIndex
				nextIndex++
				toolIndexes[idx] = outIdx
				_ = writeEvent("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": outIdx,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    str(tcm["id"]),
						"name":  str(fn["name"]),
						"input": map[string]any{},
					},
				})
				blockOpen = true
			}
			if args := str(fn["arguments"]); args != "" {
				_ = writeEvent("content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": outIdx,
					"delta": map[string]any{"type": "input_json_delta", "partial_json": args},
				})
			}
		}
	}
	if messageStart && textBuf.Len() == 0 {
		if blockOpen {
			_ = writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": nextIndex - 1})
		}
		_ = writeEvent("message_stop", map[string]any{"type": "message_stop"})
	}
	return scanner.Err()
}

// PipeResponsesStreamToAnthropic converts OpenAI Responses SSE to Anthropic stream (text deltas).
func PipeResponsesStreamToAnthropic(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	messageStart := false
	blockOpen := false
	nextIndex := 0

	writeEvent := func(eventType string, data map[string]any) error {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, "event: "+eventType+"\ndata: "+string(b)+"\n\n")
		flushIfSupported(w)
		return err
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			if messageStart {
				_ = writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
				_ = writeEvent("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}})
				_ = writeEvent("message_stop", map[string]any{"type": "message_stop"})
			}
			break
		}
		var event map[string]any
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		typ := str(event["type"])
		switch typ {
		case "response.output_text.delta":
			delta, _ := event["delta"].(string)
			if !messageStart {
				messageStart = true
				_ = writeEvent("message_start", map[string]any{
					"type": "message_start",
					"message": map[string]any{
						"type": "message", "role": "assistant", "model": model,
					},
				})
			}
			if delta != "" {
				if !blockOpen {
					blockOpen = true
					_ = writeEvent("content_block_start", map[string]any{
						"type": "content_block_start", "index": nextIndex,
						"content_block": map[string]any{"type": "text", "text": ""},
					})
					nextIndex++
				}
				_ = writeEvent("content_block_delta", map[string]any{
					"type": "content_block_delta", "index": nextIndex - 1,
					"delta": map[string]any{"type": "text_delta", "text": delta},
				})
			}
		case "response.output_item.added":
			item, _ := event["item"].(map[string]any)
			if str(item["type"]) == "function_call" || str(item["type"]) == "custom_tool_call" {
				if !messageStart {
					messageStart = true
					_ = writeEvent("message_start", map[string]any{
						"type":    "message_start",
						"message": map[string]any{"type": "message", "role": "assistant", "model": model},
					})
				}
				if blockOpen {
					_ = writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": nextIndex - 1})
					blockOpen = false
				}
				_ = writeEvent("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": nextIndex,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    firstNonEmpty(str(item["call_id"]), str(item["id"])),
						"name":  str(item["name"]),
						"input": map[string]any{},
					},
				})
				blockOpen = true
				nextIndex++
			}
		case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
			if delta, ok := event["delta"].(string); ok && blockOpen {
				_ = writeEvent("content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": nextIndex - 1,
					"delta": map[string]any{"type": "input_json_delta", "partial_json": delta},
				})
			}
		case "response.completed", "response.done":
			if messageStart {
				if blockOpen {
					_ = writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": nextIndex - 1})
				}
				_ = writeEvent("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}})
				_ = writeEvent("message_stop", map[string]any{"type": "message_stop"})
				messageStart = false
				blockOpen = false
			}
		}
	}
	return scanner.Err()
}

// CollectOpenAIChatStream aggregates SSE into a single chat completion JSON object.
func CollectOpenAIChatStream(r io.Reader) (map[string]any, error) {
	scanner := bufio.NewScanner(r)
	var content strings.Builder
	finish := "stop"
	var id, model string
	toolCalls := map[float64]map[string]any{}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if id == "" {
			id = str(chunk["id"])
		}
		if model == "" {
			model = str(chunk["model"])
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		if fr := str(choice["finish_reason"]); fr != "" {
			finish = fr
		}
		delta, _ := choice["delta"].(map[string]any)
		if delta != nil {
			if t, ok := delta["content"].(string); ok {
				content.WriteString(t)
			}
			for _, tcm := range asMapSlice(delta["tool_calls"]) {
				idx, _ := tcm["index"].(float64)
				existing := toolCalls[idx]
				if existing == nil {
					existing = map[string]any{"id": str(tcm["id"]), "type": "function", "function": map[string]any{}}
					toolCalls[idx] = existing
				}
				if str(tcm["id"]) != "" {
					existing["id"] = str(tcm["id"])
				}
				fn, _ := existing["function"].(map[string]any)
				deltaFn, _ := tcm["function"].(map[string]any)
				if str(deltaFn["name"]) != "" {
					fn["name"] = str(deltaFn["name"])
				}
				if str(deltaFn["arguments"]) != "" {
					fn["arguments"] = str(fn["arguments"]) + str(deltaFn["arguments"])
				}
			}
		}
	}
	msg := map[string]any{
		"role":    "assistant",
		"content": content.String(),
	}
	if len(toolCalls) > 0 {
		var calls []any
		for i := 0; i < len(toolCalls); i++ {
			if call := toolCalls[float64(i)]; call != nil {
				calls = append(calls, call)
			}
		}
		msg["tool_calls"] = calls
	}
	body := map[string]any{
		"id":    id,
		"model": model,
		"choices": []any{
			map[string]any{
				"message":       msg,
				"finish_reason": finish,
			},
		},
	}
	return body, scanner.Err()
}

// CollectResponsesStream aggregates Responses API SSE into one JSON object.
func CollectResponsesStream(r io.Reader) (map[string]any, error) {
	scanner := bufio.NewScanner(r)
	var text strings.Builder
	var id, model, status string
	var output []any
	var currentFunction map[string]any

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var event map[string]any
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		switch str(event["type"]) {
		case "response.created":
			if resp, ok := event["response"].(map[string]any); ok {
				id = str(resp["id"])
				model = str(resp["model"])
			}
		case "response.output_text.delta":
			if d, ok := event["delta"].(string); ok {
				text.WriteString(d)
			}
		case "response.output_item.added":
			item, _ := event["item"].(map[string]any)
			switch str(item["type"]) {
			case "function_call", "custom_tool_call":
				currentFunction = item
			}
		case "response.function_call_arguments.delta":
			if currentFunction != nil {
				currentFunction["arguments"] = str(currentFunction["arguments"]) + str(event["delta"])
			}
		case "response.custom_tool_call_input.delta":
			if currentFunction != nil {
				currentFunction["input"] = str(currentFunction["input"]) + str(event["delta"])
			}
		case "response.output_item.done":
			if item, ok := event["item"].(map[string]any); ok {
				output = append(output, item)
			} else if currentFunction != nil {
				output = append(output, currentFunction)
			}
			currentFunction = nil
		case "response.completed":
			if resp, ok := event["response"].(map[string]any); ok {
				id = str(resp["id"])
				model = str(resp["model"])
				status = str(resp["status"])
				if items := asAnySlice(resp["output"]); len(items) > 0 {
					output = items
				}
			}
			if currentFunction != nil && len(output) == 0 {
				output = append(output, currentFunction)
				currentFunction = nil
			}
		}
	}
	if status == "" {
		status = "completed"
	}
	if text.Len() > 0 {
		output = append([]any{
			map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "output_text", "text": text.String()},
				},
			},
		}, output...)
	}
	if len(output) == 0 {
		output = []any{
			map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "output_text", "text": text.String()},
				},
			},
		}
	}
	return map[string]any{
		"id":     id,
		"model":  model,
		"status": status,
		"output": output,
	}, scanner.Err()
}

// IsStreamRequested checks anthropic or openai payloads for stream flag.
func IsStreamRequested(payload map[string]any) bool {
	if s, ok := payload["stream"].(bool); ok {
		return s
	}
	return false
}

func ReadJSONBody(raw []byte) (map[string]any, error) {
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
