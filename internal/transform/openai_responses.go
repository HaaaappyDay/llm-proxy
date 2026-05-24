package transform

import "encoding/json"

func RequestFromOpenAIResponses(payload map[string]any) UnifiedRequest {
	req := UnifiedRequest{
		Model:              str(payload["model"]),
		System:             str(payload["instructions"]),
		Messages:           messagesFromResponsesInput(payload["input"]),
		Metadata:           mapAny(payload["metadata"]),
		PreviousResponseID: str(payload["previous_response_id"]),
		ToolChoice:         toolChoiceFromResponses(payload["tool_choice"]),
		ResponseFormat:     mapAny(payload["text"]),
	}
	for _, tm := range asMapSlice(payload["tools"]) {
		if str(tm["type"]) != "function" {
			req.Tools = append(req.Tools, UnifiedTool{Type: str(tm["type"]), Raw: tm, Name: str(tm["name"]), Description: str(tm["description"])})
			continue
		}
		req.Tools = append(req.Tools, UnifiedTool{
			Name:        str(tm["name"]),
			Description: str(tm["description"]),
			Parameters:  mapAny(tm["parameters"]),
			Type:        "function",
			Strict:      boolPtr(tm["strict"]),
		})
	}
	if v, ok := numberFloat(payload["temperature"]); ok {
		req.Temperature = &v
	}
	if v, ok := numberFloat(payload["top_p"]); ok {
		req.TopP = &v
	}
	if n, ok := numberInt(payload["max_output_tokens"]); ok {
		req.MaxTokens = &n
	}
	if v, ok := payload["parallel_tool_calls"].(bool); ok {
		req.ParallelToolCalls = &v
	}
	return req
}

func RequestToOpenAIResponses(req UnifiedRequest) map[string]any {
	input := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		input = append(input, messagesToResponsesInput(m)...)
	}
	out := map[string]any{"input": input}
	if req.Model != "" {
		out["model"] = req.Model
	}
	if req.System != "" {
		out["instructions"] = req.System
	}
	if req.PreviousResponseID != "" {
		out["previous_response_id"] = req.PreviousResponseID
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			if t.Type != "" && t.Type != "function" {
				tools = append(tools, t.Raw)
				continue
			}
			tool := map[string]any{
				"type":       "function",
				"name":       t.Name,
				"parameters": t.Parameters,
			}
			if t.Description != "" {
				tool["description"] = t.Description
			}
			if t.Strict != nil {
				tool["strict"] = *t.Strict
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
		out["max_output_tokens"] = *req.MaxTokens
	}
	if req.ToolChoice != nil {
		out["tool_choice"] = toolChoiceToResponses(req.ToolChoice)
	}
	if req.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *req.ParallelToolCalls
	}
	if len(req.ResponseFormat) > 0 {
		out["text"] = req.ResponseFormat
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
				} else if t == "refusal" {
					blocks = append(blocks, UnknownBlock(t, cm))
				}
			}
		case "function_call":
			id := str(im["call_id"])
			if id == "" {
				id = str(im["id"])
			}
			blocks = append(blocks, ToolCallBlock(id, str(im["name"]), jsonObject(im["arguments"])))
		case "custom_tool_call":
			blocks = append(blocks, UnifiedContentBlock{
				Type: ContentToolCall,
				ToolCall: &UnifiedToolCall{
					ID:       str(im["call_id"]),
					Name:     str(im["name"]),
					RawInput: str(im["input"]),
					Kind:     "custom",
					Status:   str(im["status"]),
				},
			})
		case "reasoning":
			blocks = append(blocks, ReasoningBlock(UnifiedReasoning{Text: str(im["summary"])}))
		default:
			blocks = append(blocks, UnknownBlock(str(im["type"]), im))
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
			if b.ToolCall.Kind == "custom" {
				output = append(output, map[string]any{
					"type":    "custom_tool_call",
					"call_id": b.ToolCall.ID,
					"name":    b.ToolCall.Name,
					"input":   b.ToolCall.RawInput,
				})
			} else {
				args, _ := json.Marshal(b.ToolCall.Arguments)
				output = append(output, map[string]any{
					"type":      "function_call",
					"call_id":   b.ToolCall.ID,
					"name":      b.ToolCall.Name,
					"arguments": string(args),
				})
			}
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
				switch str(cm["type"]) {
				case "input_text", "output_text", "text":
					blocks = append(blocks, TextBlock(str(cm["text"])))
				case "input_image":
					img := imageFromOpenAIURL(str(cm["image_url"]))
					img.FileID = str(cm["file_id"])
					img.Detail = str(cm["detail"])
					blocks = append(blocks, ImageBlock(UnifiedImage{
						URL:       img.URL,
						Data:      img.Data,
						MediaType: img.MediaType,
						FileID:    img.FileID,
						Detail:    img.Detail,
					}))
				case "input_file":
					blocks = append(blocks, FileBlock(UnifiedFile{
						FileID:   str(cm["file_id"]),
						FileData: str(cm["file_data"]),
						FileURL:  str(cm["file_url"]),
						FileName: str(cm["filename"]),
					}))
				default:
					blocks = append(blocks, UnknownBlock(str(cm["type"]), cm))
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
		case "custom_tool_call_output":
			out = append(out, UnifiedMessage{
				Role: RoleTool,
				Content: []UnifiedContentBlock{
					ToolResultBlock(str(m["call_id"]), str(m["output"])),
				},
			})
		case "function_call":
			out = append(out, UnifiedMessage{
				Role: RoleAssistant,
				Content: []UnifiedContentBlock{
					ToolCallBlock(str(m["call_id"]), str(m["name"]), jsonObject(m["arguments"])),
				},
			})
		case "custom_tool_call":
			out = append(out, UnifiedMessage{
				Role: RoleAssistant,
				Content: []UnifiedContentBlock{
					{
						Type: ContentToolCall,
						ToolCall: &UnifiedToolCall{
							ID:       str(m["call_id"]),
							Name:     str(m["name"]),
							RawInput: str(m["input"]),
							Kind:     "custom",
						},
					},
				},
			})
		}
	}
	return out
}

func messagesToResponsesInput(m UnifiedMessage) []map[string]any {
	if m.Role == RoleTool {
		var out []map[string]any
		for _, b := range m.Content {
			if b.Type == ContentToolResult {
				out = append(out, map[string]any{
					"type":    "function_call_output",
					"call_id": b.ToolCallID,
					"output":  b.Text,
				})
			}
		}
		return out
	}
	var content []map[string]any
	var items []map[string]any
	for _, b := range m.Content {
		if b.Type == ContentText {
			content = append(content, map[string]any{"type": responsesTextType(m.Role), "text": b.Text})
		}
		if b.Type == ContentImage && b.Image != nil {
			item := map[string]any{"type": "input_image"}
			if b.Image.URL != "" {
				item["image_url"] = b.Image.URL
			}
			if b.Image.FileID != "" {
				item["file_id"] = b.Image.FileID
			}
			if b.Image.Detail != "" {
				item["detail"] = b.Image.Detail
			}
			content = append(content, item)
		}
		if b.Type == ContentFile && b.File != nil {
			item := map[string]any{"type": "input_file"}
			if b.File.FileID != "" {
				item["file_id"] = b.File.FileID
			}
			if b.File.FileData != "" {
				item["file_data"] = b.File.FileData
			}
			if b.File.FileURL != "" {
				item["file_url"] = b.File.FileURL
			}
			if b.File.FileName != "" {
				item["filename"] = b.File.FileName
			}
			content = append(content, item)
		}
		if b.Type == ContentToolCall && b.ToolCall != nil {
			if b.ToolCall.Kind == "custom" {
				items = append(items, map[string]any{
					"type":    "custom_tool_call",
					"call_id": b.ToolCall.ID,
					"name":    b.ToolCall.Name,
					"input":   b.ToolCall.RawInput,
				})
				continue
			}
			args, _ := json.Marshal(b.ToolCall.Arguments)
			items = append(items, map[string]any{
				"type":      "function_call",
				"call_id":   b.ToolCall.ID,
				"name":      b.ToolCall.Name,
				"arguments": string(args),
			})
		}
	}
	var out []map[string]any
	if len(content) > 0 || len(items) == 0 {
		out = append(out, map[string]any{
			"type":    "message",
			"role":    string(m.Role),
			"content": content,
		})
	}
	out = append(out, items...)
	return out
}

func responsesTextType(role Role) string {
	if role == RoleAssistant {
		return "output_text"
	}
	return "input_text"
}

func usageFromOpenAIResponses(v any) *UnifiedUsage {
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

func toolChoiceFromResponses(v any) *UnifiedToolChoice {
	if s, ok := v.(string); ok {
		return &UnifiedToolChoice{Mode: s}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	choice := &UnifiedToolChoice{Mode: str(m["type"]), Name: str(m["name"]), Raw: m}
	if choice.Mode == "function" || choice.Mode == "custom" {
		choice.Mode = "tool"
	}
	return choice
}

func toolChoiceToResponses(choice *UnifiedToolChoice) any {
	if choice == nil {
		return nil
	}
	switch choice.Mode {
	case "none", "auto", "required":
		return choice.Mode
	case "any":
		return "required"
	case "tool":
		if choice.Type == "custom" {
			return map[string]any{"type": "custom", "name": choice.Name}
		}
		return map[string]any{"type": "function", "name": choice.Name}
	default:
		return choice.Mode
	}
}
