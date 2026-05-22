package transform

import "encoding/json"

func RequestFromOpenAIResponses(payload map[string]any) UnifiedRequest {
	req := UnifiedRequest{
		Model:    str(payload["model"]),
		System:   str(payload["instructions"]),
		Messages: messagesFromResponsesInput(payload["input"]),
		Metadata: mapAny(payload["metadata"]),
	}
	for _, tm := range asMapSlice(payload["tools"]) {
		if str(tm["type"]) != "function" {
			continue
		}
		req.Tools = append(req.Tools, UnifiedTool{
			Name:        str(tm["name"]),
			Description: str(tm["description"]),
			Parameters:  mapAny(tm["parameters"]),
		})
	}
	if v, ok := payload["temperature"].(float64); ok {
		req.Temperature = &v
	}
	if v, ok := payload["max_output_tokens"].(float64); ok {
		n := int(v)
		req.MaxTokens = &n
	}
	return req
}

func RequestToOpenAIResponses(req UnifiedRequest) map[string]any {
	input := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		input = append(input, messageToResponsesInput(m))
	}
	out := map[string]any{"input": input}
	if req.Model != "" {
		out["model"] = req.Model
	}
	if req.System != "" {
		out["instructions"] = req.System
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tool := map[string]any{
				"type":       "function",
				"name":       t.Name,
				"parameters": t.Parameters,
			}
			if t.Description != "" {
				tool["description"] = t.Description
			}
			tools = append(tools, tool)
		}
		out["tools"] = tools
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		out["max_output_tokens"] = *req.MaxTokens
	}
	if len(req.Metadata) > 0 {
		out["metadata"] = req.Metadata
	}
	return out
}

func ResponseFromOpenAIResponses(payload map[string]any) UnifiedResponse {
	var blocks []UnifiedContentBlock
	for _, im := range asMapSlice(payload["output"]) {
		switch str(im["type"]) {
		case "message":
			for _, cm := range asMapSlice(im["content"]) {
				t := str(cm["type"])
				if t == "output_text" || t == "text" {
					blocks = append(blocks, TextBlock(str(cm["text"])))
				}
			}
		case "function_call":
			id := str(im["call_id"])
			if id == "" {
				id = str(im["id"])
			}
			blocks = append(blocks, ToolCallBlock(id, str(im["name"]), jsonObject(im["arguments"])))
		}
	}
	if len(blocks) == 0 && str(payload["output_text"]) != "" {
		blocks = append(blocks, TextBlock(str(payload["output_text"])))
	}
	if len(blocks) == 0 {
		blocks = append(blocks, TextBlock(""))
	}
	return UnifiedResponse{
		ID:           str(payload["id"]),
		Model:        str(payload["model"]),
		Message:      UnifiedMessage{Role: RoleAssistant, Content: blocks},
		FinishReason: str(payload["status"]),
		Usage:        usageFromOpenAIResponses(payload["usage"]),
	}
}

func ResponseToOpenAIResponses(resp UnifiedResponse) map[string]any {
	output := []map[string]any{}
	text := joinNonEmpty(textParts(resp.Message.Content))
	if text != "" {
		output = append(output, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]any{{"type": "output_text", "text": text}},
		})
	}
	for _, b := range resp.Message.Content {
		if b.Type == ContentToolCall && b.ToolCall != nil {
			args, _ := json.Marshal(b.ToolCall.Arguments)
			output = append(output, map[string]any{
				"type":      "function_call",
				"call_id":   b.ToolCall.ID,
				"name":      b.ToolCall.Name,
				"arguments": string(args),
			})
		}
	}
	out := map[string]any{"output": output}
	if resp.ID != "" {
		out["id"] = resp.ID
	}
	if resp.Model != "" {
		out["model"] = resp.Model
	}
	if resp.FinishReason != "" {
		out["status"] = resp.FinishReason
	}
	if resp.Usage != nil {
		u := map[string]any{}
		in, ot := 0, 0
		if resp.Usage.InputTokens != nil {
			in = *resp.Usage.InputTokens
			u["input_tokens"] = in
		}
		if resp.Usage.OutputTokens != nil {
			ot = *resp.Usage.OutputTokens
			u["output_tokens"] = ot
		}
		u["total_tokens"] = in + ot
		out["usage"] = u
	}
	return out
}

func messagesFromResponsesInput(input any) []UnifiedMessage {
	var out []UnifiedMessage
	for _, m := range asMapSlice(input) {
		role := Role(str(m["role"]))
		if role == "" {
			role = RoleUser
		}
		switch str(m["type"]) {
		case "message":
			var blocks []UnifiedContentBlock
			for _, cm := range asMapSlice(m["content"]) {
				if str(cm["type"]) == "input_text" || str(cm["type"]) == "text" {
					blocks = append(blocks, TextBlock(str(cm["text"])))
				}
			}
			if len(blocks) == 0 {
				if s, ok := m["content"].(string); ok {
					blocks = append(blocks, TextBlock(s))
				}
			}
			out = append(out, UnifiedMessage{Role: role, Content: blocks})
		case "function_call_output":
			out = append(out, UnifiedMessage{
				Role: RoleTool,
				Content: []UnifiedContentBlock{
					ToolResultBlock(str(m["call_id"]), str(m["output"])),
				},
			})
		}
	}
	return out
}

func messageToResponsesInput(m UnifiedMessage) map[string]any {
	if m.Role == RoleTool {
		for _, b := range m.Content {
			if b.Type == ContentToolResult {
				return map[string]any{
					"type":    "function_call_output",
					"call_id": b.ToolCallID,
					"output":  b.Text,
				}
			}
		}
	}
	var content []map[string]any
	for _, b := range m.Content {
		if b.Type == ContentText {
			content = append(content, map[string]any{"type": "input_text", "text": b.Text})
		}
		if b.Type == ContentToolCall && b.ToolCall != nil {
			args, _ := json.Marshal(b.ToolCall.Arguments)
			return map[string]any{
				"type":      "function_call",
				"call_id":   b.ToolCall.ID,
				"name":      b.ToolCall.Name,
				"arguments": string(args),
			}
		}
	}
	return map[string]any{
		"type":    "message",
		"role":    string(m.Role),
		"content": content,
	}
}

func usageFromOpenAIResponses(v any) *UnifiedUsage {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	u := &UnifiedUsage{}
	if n, ok := m["input_tokens"].(float64); ok {
		i := int(n)
		u.InputTokens = &i
	}
	if n, ok := m["output_tokens"].(float64); ok {
		i := int(n)
		u.OutputTokens = &i
	}
	return u
}
