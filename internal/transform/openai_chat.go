package transform

import "encoding/json"

func RequestFromOpenAIChat(payload map[string]any) UnifiedRequest {
	var systemParts []string
	var messages []UnifiedMessage
	for _, m := range asMapSlice(payload["messages"]) {
			if str(m["role"]) == "system" {
				systemParts = append(systemParts, contentText(m["content"]))
		} else {
			messages = append(messages, messageFromOpenAIChat(m))
		}
	}
	req := UnifiedRequest{
		Model:    str(payload["model"]),
		System:   joinNonEmpty(systemParts),
		Messages: messages,
		Metadata: mapAny(payload["metadata"]),
	}
	for _, tm := range asMapSlice(payload["tools"]) {
			if str(tm["type"]) != "function" {
				continue
			}
			fn, _ := tm["function"].(map[string]any)
		req.Tools = append(req.Tools, UnifiedTool{
			Name:        str(fn["name"]),
			Description: str(fn["description"]),
			Parameters:  mapAny(fn["parameters"]),
		})
	}
	if v, ok := payload["temperature"].(float64); ok {
		req.Temperature = &v
	}
	if v, ok := payload["max_tokens"].(float64); ok {
		n := int(v)
		req.MaxTokens = &n
	}
	return req
}

func RequestToOpenAIChat(req UnifiedRequest) map[string]any {
	messages := []map[string]any{}
	if req.System != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		messages = append(messages, messagesToOpenAIChat(m)...)
	}
	out := map[string]any{"messages": messages}
	if req.Model != "" {
		out["model"] = req.Model
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			fn := map[string]any{"name": t.Name, "parameters": t.Parameters}
			if t.Description != "" {
				fn["description"] = t.Description
			}
			tools = append(tools, map[string]any{"type": "function", "function": fn})
		}
		out["tools"] = tools
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		out["max_tokens"] = *req.MaxTokens
	}
	if len(req.Metadata) > 0 {
		out["metadata"] = req.Metadata
	}
	return out
}

func ResponseFromOpenAIChat(payload map[string]any) UnifiedResponse {
	choices := asMapSlice(payload["choices"])
	choice := map[string]any{}
	if len(choices) > 0 {
		choice = choices[0]
	}
	msg, _ := choice["message"].(map[string]any)
	if msg == nil {
		msg = map[string]any{"role": "assistant", "content": ""}
	}
	return UnifiedResponse{
		ID:           str(payload["id"]),
		Model:        str(payload["model"]),
		Message:      messageFromOpenAIChat(msg),
		FinishReason: str(choice["finish_reason"]),
		Usage:        usageFromOpenAIChat(payload["usage"]),
	}
}

func ResponseToOpenAIChat(resp UnifiedResponse) map[string]any {
	msgs := messagesToOpenAIChat(resp.Message)
	msg := map[string]any{"role": "assistant", "content": ""}
	if len(msgs) > 0 {
		msg = msgs[0]
	}
	out := map[string]any{
		"choices": []map[string]any{{"message": msg, "finish_reason": resp.FinishReason}},
	}
	if resp.ID != "" {
		out["id"] = resp.ID
	}
	if resp.Model != "" {
		out["model"] = resp.Model
	}
	if resp.Usage != nil {
		u := map[string]any{}
		inTok, outTok := 0, 0
		if resp.Usage.InputTokens != nil {
			inTok = *resp.Usage.InputTokens
			u["prompt_tokens"] = inTok
		}
		if resp.Usage.OutputTokens != nil {
			outTok = *resp.Usage.OutputTokens
			u["completion_tokens"] = outTok
		}
		u["total_tokens"] = inTok + outTok
		out["usage"] = u
	}
	return out
}

func messageFromOpenAIChat(m map[string]any) UnifiedMessage {
	role := Role(str(m["role"]))
	if role == RoleTool {
		return UnifiedMessage{
			Role: RoleTool,
			Content: []UnifiedContentBlock{
				ToolResultBlock(str(m["tool_call_id"]), contentText(m["content"])),
			},
		}
	}
	var blocks []UnifiedContentBlock
	if text := contentText(m["content"]); text != "" {
		blocks = append(blocks, TextBlock(text))
	}
	for _, tcm := range asMapSlice(m["tool_calls"]) {
			fn, _ := tcm["function"].(map[string]any)
		blocks = append(blocks, ToolCallBlock(
			str(tcm["id"]),
			str(fn["name"]),
			jsonObject(fn["arguments"]),
		))
	}
	if len(blocks) == 0 {
		blocks = []UnifiedContentBlock{TextBlock("")}
	}
	return UnifiedMessage{Role: role, Content: blocks}
}

func messagesToOpenAIChat(m UnifiedMessage) []map[string]any {
	if m.Role == RoleTool {
		out := []map[string]any{}
		for _, b := range m.Content {
			if b.Type == ContentToolResult {
				out = append(out, map[string]any{
					"role":          "tool",
					"tool_call_id":  b.ToolCallID,
					"content":       b.Text,
				})
			}
		}
		return out
	}
	text := joinNonEmpty(textParts(m.Content))
	var toolCalls []map[string]any
	for _, b := range m.Content {
		if b.Type == ContentToolCall && b.ToolCall != nil {
			args, _ := json.Marshal(b.ToolCall.Arguments)
			toolCalls = append(toolCalls, map[string]any{
				"id":   b.ToolCall.ID,
				"type": "function",
				"function": map[string]any{
					"name":      b.ToolCall.Name,
					"arguments": string(args),
				},
			})
		}
	}
	msg := map[string]any{"role": string(m.Role), "content": text}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}
	return []map[string]any{msg}
}

func contentText(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if arr, ok := v.([]any); ok {
		var parts []string
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok && str(m["type"]) == "text" {
				parts = append(parts, str(m["text"]))
			}
		}
		return joinNonEmpty(parts)
	}
	return ""
}

func textParts(blocks []UnifiedContentBlock) []string {
	var parts []string
	for _, b := range blocks {
		if b.Type == ContentText && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return parts
}

func usageFromOpenAIChat(v any) *UnifiedUsage {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	u := &UnifiedUsage{}
	if n, ok := m["prompt_tokens"].(float64); ok {
		i := int(n)
		u.InputTokens = &i
	}
	if n, ok := m["completion_tokens"].(float64); ok {
		i := int(n)
		u.OutputTokens = &i
	}
	return u
}

func joinNonEmpty(parts []string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return ""
	}
	result := out[0]
	for i := 1; i < len(out); i++ {
		result += "\n" + out[i]
	}
	return result
}
