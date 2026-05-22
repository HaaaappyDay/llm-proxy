package transform

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// PipeOpenAIChatStreamToAnthropic converts OpenAI chat completion SSE to Anthropic message stream events.
func PipeOpenAIChatStreamToAnthropic(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var textBuf strings.Builder
	messageStart := false

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
				_ = writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
				_ = writeEvent("message_delta", map[string]any{
					"type": "message_delta",
					"delta": map[string]any{"stop_reason": "end_turn"},
				})
				_ = writeEvent("message_stop", map[string]any{"type": "message_stop"})
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
					"id":    str(chunk["id"]),
					"type":  "message",
					"role":  "assistant",
					"model": model,
					"content": []any{},
				},
			})
			_ = writeEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": 0,
				"content_block": map[string]any{"type": "text", "text": ""},
			})
		}
		if t, ok := delta["content"].(string); ok && t != "" {
			textBuf.WriteString(t)
			_ = writeEvent("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{"type": "text_delta", "text": t},
			})
		}
	}
	if messageStart && textBuf.Len() == 0 {
		_ = writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		_ = writeEvent("message_stop", map[string]any{"type": "message_stop"})
	}
	return scanner.Err()
}

// PipeResponsesStreamToAnthropic converts OpenAI Responses SSE to Anthropic stream (text deltas).
func PipeResponsesStreamToAnthropic(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	messageStart := false

	writeEvent := func(eventType string, data map[string]any) error {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, "event: "+eventType+"\ndata: "+string(b)+"\n\n")
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
				_ = writeEvent("content_block_start", map[string]any{
					"type": "content_block_start", "index": 0,
					"content_block": map[string]any{"type": "text", "text": ""},
				})
			}
			if delta != "" {
				_ = writeEvent("content_block_delta", map[string]any{
					"type": "content_block_delta", "index": 0,
					"delta": map[string]any{"type": "text_delta", "text": delta},
				})
			}
		case "response.completed", "response.done":
			if messageStart {
				_ = writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
				_ = writeEvent("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}})
				_ = writeEvent("message_stop", map[string]any{"type": "message_stop"})
				messageStart = false
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
		}
	}
	body := map[string]any{
		"id":    id,
		"model": model,
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": content.String(),
				},
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
		case "response.completed":
			if resp, ok := event["response"].(map[string]any); ok {
				id = str(resp["id"])
				model = str(resp["model"])
				status = str(resp["status"])
			}
		}
	}
	if status == "" {
		status = "completed"
	}
	return map[string]any{
		"id":     id,
		"model":  model,
		"status": status,
		"output": []any{
			map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "output_text", "text": text.String()},
				},
			},
		},
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
