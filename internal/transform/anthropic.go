package transform

import (
	"encoding/json"
	"strings"
)

func RequestFromAnthropic(payload map[string]any) UnifiedRequest {
	req := UnifiedRequest{
		Model:    str(payload["model"]),
		System:   systemText(payload["system"]),
		Metadata: mapAny(payload["metadata"]),
	}
	for _, mm := range asMapSlice(payload["messages"]) {
		req.Messages = append(req.Messages, messageFromAnthropic(mm))
	}
	for _, tm := range asMapSlice(payload["tools"]) {
		req.Tools = append(req.Tools, UnifiedTool{
			Name:        str(tm["name"]),
			Description: str(tm["description"]),
			Parameters:  mapAny(tm["input_schema"]),
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

func RequestToAnthropic(req UnifiedRequest) map[string]any {
	out := map[string]any{
		"messages": messagesToAnthropic(req.Messages),
	}
	if req.Model != "" {
		out["model"] = req.Model
	}
	if req.System != "" {
		out["system"] = req.System
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tool := map[string]any{
				"name":         t.Name,
				"input_schema": t.Parameters,
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
		out["max_tokens"] = *req.MaxTokens
	}
	if len(req.Metadata) > 0 {
		out["metadata"] = req.Metadata
	}
	return out
}

func ResponseFromAnthropic(payload map[string]any) UnifiedResponse {
	var blocks []UnifiedContentBlock
	for _, cm := range asMapSlice(payload["content"]) {
		blocks = append(blocks, blockFromAnthropic(cm))
	}
	return UnifiedResponse{
		ID:           str(payload["id"]),
		Model:        str(payload["model"]),
		Message:      UnifiedMessage{Role: RoleAssistant, Content: blocks},
		FinishReason: str(payload["stop_reason"]),
		Usage:        usageFromAnthropic(payload["usage"]),
	}
}

func ResponseToAnthropic(resp UnifiedResponse) map[string]any {
	out := map[string]any{
		"type":    "message",
		"role":    "assistant",
		"content": blocksToAnthropic(resp.Message.Content),
	}
	if resp.ID != "" {
		out["id"] = resp.ID
	}
	if resp.Model != "" {
		out["model"] = resp.Model
	}
	if resp.FinishReason != "" {
		out["stop_reason"] = resp.FinishReason
	}
	if resp.Usage != nil {
		u := map[string]any{}
		if resp.Usage.InputTokens != nil {
			u["input_tokens"] = *resp.Usage.InputTokens
		}
		if resp.Usage.OutputTokens != nil {
			u["output_tokens"] = *resp.Usage.OutputTokens
		}
		out["usage"] = u
	}
	return out
}

func messageFromAnthropic(m map[string]any) UnifiedMessage {
	role := Role(str(m["role"]))
	content := m["content"]
	if s, ok := content.(string); ok {
		return UnifiedMessage{Role: role, Content: []UnifiedContentBlock{TextBlock(s)}}
	}
	var blocks []UnifiedContentBlock
	for _, cm := range asMapSlice(content) {
		blocks = append(blocks, blockFromAnthropic(cm))
	}
	return UnifiedMessage{Role: role, Content: blocks}
}

func messagesToAnthropic(msgs []UnifiedMessage) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, map[string]any{
			"role":    string(m.Role),
			"content": blocksToAnthropic(m.Content),
		})
	}
	return out
}

func blockFromAnthropic(b map[string]any) UnifiedContentBlock {
	switch str(b["type"]) {
	case "text":
		return TextBlock(str(b["text"]))
	case "tool_use":
		return ToolCallBlock(str(b["id"]), str(b["name"]), mapAny(b["input"]))
	case "tool_result":
		return ToolResultBlock(str(b["tool_use_id"]), anthropicToolResultText(b["content"]))
	default:
		return UnifiedContentBlock{Type: ContentText, Text: "", Extra: map[string]any{"anthropic": b}}
	}
}

func blocksToAnthropic(blocks []UnifiedContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case ContentText:
			out = append(out, map[string]any{"type": "text", "text": b.Text})
		case ContentToolCall:
			if b.ToolCall != nil {
				out = append(out, map[string]any{
					"type":  "tool_use",
					"id":    b.ToolCall.ID,
					"name":  b.ToolCall.Name,
					"input": b.ToolCall.Arguments,
				})
			}
		case ContentToolResult:
			out = append(out, map[string]any{
				"type":        "tool_result",
				"tool_use_id": b.ToolCallID,
				"content":     b.Text,
			})
		}
	}
	return out
}

func systemText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if arr, ok := v.([]any); ok {
		var parts []string
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				parts = append(parts, str(m["text"]))
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func anthropicToolResultText(v any) string {
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
		return strings.Join(parts, "\n")
	}
	return ""
}

func usageFromAnthropic(v any) *UnifiedUsage {
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

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mapAny(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func jsonObject(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case string:
		if t == "" {
			return map[string]any{}
		}
		var out map[string]any
		if json.Unmarshal([]byte(t), &out) == nil {
			return out
		}
	}
	return map[string]any{}
}
