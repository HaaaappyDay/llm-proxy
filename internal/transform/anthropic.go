package transform

import (
	"encoding/json"
	"strings"
)

func RequestFromAnthropic(payload map[string]any) UnifiedRequest {
	req := UnifiedRequest{
		Model:      str(payload["model"]),
		System:     systemText(payload["system"]),
		Metadata:   mapAny(payload["metadata"]),
		ToolChoice: toolChoiceFromAnthropic(payload["tool_choice"]),
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
	if v, ok := numberFloat(payload["temperature"]); ok {
		req.Temperature = &v
	}
	if v, ok := numberFloat(payload["top_p"]); ok {
		req.TopP = &v
	}
	if n, ok := numberInt(payload["max_tokens"]); ok {
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
			if t.Type != "" && t.Type != "function" {
				continue
			}
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
	if req.TopP != nil {
		out["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		out["max_tokens"] = *req.MaxTokens
	}
	if req.ToolChoice != nil {
		out["tool_choice"] = toolChoiceToAnthropic(req.ToolChoice)
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
		role := m.Role
		if role == RoleTool {
			role = RoleUser
		}
		out = append(out, map[string]any{
			"role":    string(role),
			"content": blocksToAnthropic(m.Content),
		})
	}
	return out
}

func blockFromAnthropic(b map[string]any) UnifiedContentBlock {
	switch str(b["type"]) {
	case "text":
		return TextBlock(str(b["text"]))
	case "image":
		return imageFromAnthropic(b)
	case "document":
		return fileFromAnthropic(b)
	case "tool_use":
		return ToolCallBlock(str(b["id"]), str(b["name"]), mapAny(b["input"]))
	case "tool_result":
		return ToolResultBlock(str(b["tool_use_id"]), anthropicToolResultText(b["content"]))
	case "thinking":
		return ReasoningBlock(UnifiedReasoning{Text: str(b["thinking"]), Signature: str(b["signature"])})
	default:
		return UnknownBlock(str(b["type"]), b)
	}
}

func blocksToAnthropic(blocks []UnifiedContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case ContentText:
			out = append(out, map[string]any{"type": "text", "text": b.Text})
		case ContentImage:
			if b.Image != nil {
				out = append(out, imageToAnthropic(*b.Image))
			}
		case ContentFile:
			if b.File != nil {
				out = append(out, fileToAnthropic(*b.File))
			}
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
		case ContentReasoning:
			if b.Reasoning != nil {
				block := map[string]any{"type": "thinking", "thinking": b.Reasoning.Text}
				if b.Reasoning.Signature != "" {
					block["signature"] = b.Reasoning.Signature
				}
				out = append(out, block)
			}
		}
	}
	return out
}

func imageFromAnthropic(b map[string]any) UnifiedContentBlock {
	src, _ := b["source"].(map[string]any)
	img := UnifiedImage{
		URL:       str(src["url"]),
		Data:      str(src["data"]),
		MediaType: str(src["media_type"]),
		FileID:    str(src["file_id"]),
	}
	return ImageBlock(img)
}

func imageToAnthropic(img UnifiedImage) map[string]any {
	source := map[string]any{}
	switch {
	case img.URL != "":
		source["type"] = "url"
		source["url"] = img.URL
	case img.Data != "":
		source["type"] = "base64"
		source["media_type"] = img.MediaType
		source["data"] = img.Data
	case img.FileID != "":
		source["type"] = "file"
		source["file_id"] = img.FileID
	}
	return map[string]any{"type": "image", "source": source}
}

func fileFromAnthropic(b map[string]any) UnifiedContentBlock {
	src, _ := b["source"].(map[string]any)
	return FileBlock(UnifiedFile{
		FileID:    str(src["file_id"]),
		FileData:  str(src["data"]),
		FileURL:   str(src["url"]),
		FileName:  str(b["title"]),
		MediaType: str(src["media_type"]),
	})
}

func fileToAnthropic(file UnifiedFile) map[string]any {
	source := map[string]any{}
	switch {
	case file.FileID != "":
		source["type"] = "file"
		source["file_id"] = file.FileID
	case file.FileURL != "":
		source["type"] = "url"
		source["url"] = file.FileURL
	case file.FileData != "":
		source["type"] = "base64"
		source["media_type"] = file.MediaType
		source["data"] = file.FileData
	}
	out := map[string]any{"type": "document", "source": source}
	if file.FileName != "" {
		out["title"] = file.FileName
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
	if i, ok := numberInt(m["input_tokens"]); ok {
		u.InputTokens = &i
	}
	if i, ok := numberInt(m["output_tokens"]); ok {
		u.OutputTokens = &i
	}
	return u
}

func toolChoiceFromAnthropic(v any) *UnifiedToolChoice {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	choice := &UnifiedToolChoice{Mode: str(m["type"]), Name: str(m["name"]), Raw: m}
	if choice.Mode == "any" {
		choice.Mode = "required"
	}
	return choice
}

func toolChoiceToAnthropic(choice *UnifiedToolChoice) map[string]any {
	if choice == nil {
		return nil
	}
	switch choice.Mode {
	case "none", "auto":
		return map[string]any{"type": choice.Mode}
	case "required", "any":
		return map[string]any{"type": "any"}
	case "tool":
		return map[string]any{"type": "tool", "name": choice.Name}
	default:
		if choice.Name != "" {
			return map[string]any{"type": "tool", "name": choice.Name}
		}
		return map[string]any{"type": "auto"}
	}
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
