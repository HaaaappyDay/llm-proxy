package transform

import (
	"encoding/json"
	"strings"
)

func RequestFromOpenAIChat(payload map[string]any) UnifiedRequest {
	var systemParts []string
	var messages []UnifiedMessage
	for _, m := range asMapSlice(payload["messages"]) {
		if str(m["role"]) == "system" || str(m["role"]) == "developer" {
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
			req.Tools = append(req.Tools, UnifiedTool{Type: str(tm["type"]), Raw: tm})
			continue
		}
		fn, _ := tm["function"].(map[string]any)
		req.Tools = append(req.Tools, UnifiedTool{
			Name:        str(fn["name"]),
			Description: str(fn["description"]),
			Parameters:  mapAny(fn["parameters"]),
			Type:        "function",
			Strict:      boolPtr(fn["strict"]),
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
	if n, ok := numberInt(payload["max_completion_tokens"]); ok {
		req.MaxTokens = &n
	}
	req.ToolChoice = toolChoiceFromOpenAIChat(payload["tool_choice"])
	req.ResponseFormat = mapAny(payload["response_format"])
	if v, ok := payload["parallel_tool_calls"].(bool); ok {
		req.ParallelToolCalls = &v
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
			if t.Type != "" && t.Type != "function" {
				continue
			}
			fn := map[string]any{"name": t.Name, "parameters": t.Parameters}
			if t.Description != "" {
				fn["description"] = t.Description
			}
			if t.Strict != nil {
				fn["strict"] = *t.Strict
			}
			tools = append(tools, map[string]any{"type": "function", "function": fn})
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
		out["tool_choice"] = toolChoiceToOpenAIChat(req.ToolChoice)
	}
	if len(req.ResponseFormat) > 0 {
		out["response_format"] = req.ResponseFormat
	}
	if req.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *req.ParallelToolCalls
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
	resp := UnifiedResponse{
		ID:           str(payload["id"]),
		Model:        str(payload["model"]),
		Message:      messageFromOpenAIChat(msg),
		FinishReason: str(choice["finish_reason"]),
		Usage:        usageFromOpenAIChat(payload["usage"]),
	}
	if len(choices) > 1 {
		resp.Extra = map[string]any{"unsupported": "multiple_choices"}
	}
	return resp
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
	switch content := m["content"].(type) {
	case string:
		if content != "" {
			blocks = append(blocks, TextBlock(content))
		}
	default:
		for _, cm := range asMapSlice(m["content"]) {
			blocks = append(blocks, blockFromOpenAIChatContent(cm))
		}
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
					"role":         "tool",
					"tool_call_id": b.ToolCallID,
					"content":      b.Text,
				})
			}
		}
		return out
	}
	content := blocksToOpenAIChatContent(m.Content)
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
	msg := map[string]any{"role": string(m.Role), "content": content}
	if onlyText, ok := singleTextContent(content); ok {
		msg["content"] = onlyText
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}
	return []map[string]any{msg}
}

func blockFromOpenAIChatContent(b map[string]any) UnifiedContentBlock {
	switch str(b["type"]) {
	case "text":
		return TextBlock(str(b["text"]))
	case "image_url":
		imageURL, _ := b["image_url"].(map[string]any)
		img := imageFromOpenAIURL(str(imageURL["url"]))
		img.Detail = str(imageURL["detail"])
		return ImageBlock(img)
	case "input_audio":
		audio, _ := b["input_audio"].(map[string]any)
		return AudioBlock(UnifiedAudio{Data: str(audio["data"]), Format: str(audio["format"])})
	case "file":
		file, _ := b["file"].(map[string]any)
		return FileBlock(UnifiedFile{FileID: str(file["file_id"]), FileData: str(file["file_data"]), FileName: str(file["filename"])})
	default:
		return UnknownBlock(str(b["type"]), b)
	}
}

func blocksToOpenAIChatContent(blocks []UnifiedContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case ContentText:
			out = append(out, map[string]any{"type": "text", "text": b.Text})
		case ContentImage:
			if b.Image != nil {
				img := map[string]any{"url": imageURLForOpenAI(*b.Image)}
				if b.Image.Detail != "" {
					img["detail"] = b.Image.Detail
				}
				out = append(out, map[string]any{"type": "image_url", "image_url": img})
			}
		case ContentAudio:
			if b.Audio != nil {
				out = append(out, map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": b.Audio.Data, "format": b.Audio.Format}})
			}
		}
	}
	return out
}

func imageURLForOpenAI(img UnifiedImage) string {
	if img.URL != "" {
		return img.URL
	}
	if img.Data != "" && img.MediaType != "" {
		return "data:" + img.MediaType + ";base64," + img.Data
	}
	return ""
}

func imageFromOpenAIURL(url string) UnifiedImage {
	if strings.HasPrefix(url, "data:") {
		if mediaType, data, ok := splitDataURL(url); ok {
			return UnifiedImage{Data: data, MediaType: mediaType}
		}
	}
	return UnifiedImage{URL: url}
}

func singleTextContent(content []map[string]any) (string, bool) {
	if len(content) != 1 || str(content[0]["type"]) != "text" {
		return "", false
	}
	return str(content[0]["text"]), true
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
	if i, ok := numberInt(m["prompt_tokens"]); ok {
		u.InputTokens = &i
	}
	if i, ok := numberInt(m["completion_tokens"]); ok {
		u.OutputTokens = &i
	}
	if i, ok := numberInt(m["total_tokens"]); ok {
		u.TotalTokens = &i
	}
	return u
}

func toolChoiceFromOpenAIChat(v any) *UnifiedToolChoice {
	if s, ok := v.(string); ok {
		return &UnifiedToolChoice{Mode: s}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	choice := &UnifiedToolChoice{Mode: str(m["type"]), Raw: m}
	if fn, ok := m["function"].(map[string]any); ok {
		choice.Name = str(fn["name"])
		choice.Mode = "tool"
		choice.Type = "function"
	}
	return choice
}

func toolChoiceToOpenAIChat(choice *UnifiedToolChoice) any {
	if choice == nil {
		return nil
	}
	switch choice.Mode {
	case "none", "auto", "required":
		return choice.Mode
	case "any":
		return "required"
	case "tool":
		return map[string]any{"type": "function", "function": map[string]any{"name": choice.Name}}
	default:
		return choice.Mode
	}
}

func boolPtr(v any) *bool {
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
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
