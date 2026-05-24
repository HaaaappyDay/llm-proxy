package transform

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestAnthropicRequestRoundTrip(t *testing.T) {
	payload := map[string]any{
		"model":  "claude-test",
		"system": "Be concise.",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "Weather?"},
				},
			},
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type":  "tool_use",
						"id":    "call_1",
						"name":  "get_weather",
						"input": map[string]any{"city": "Shanghai"},
					},
				},
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "call_1",
						"content":     "Sunny",
					},
				},
			},
		},
		"tools": []any{
			map[string]any{
				"name":        "get_weather",
				"description": "Get weather",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
				},
			},
		},
		"temperature": 0.2,
		"max_tokens":  1024.0,
	}

	unified := RequestFromAnthropic(payload)
	chat := RequestToOpenAIChat(unified)
	back := RequestToAnthropic(RequestFromOpenAIChat(chat))

	if str(back["model"]) != "claude-test" {
		t.Fatalf("model mismatch: %v", back["model"])
	}
	if str(back["system"]) != "Be concise." {
		t.Fatalf("system mismatch: %v", back["system"])
	}
}

func TestOpenAIChatResponseRoundTrip(t *testing.T) {
	payload := map[string]any{
		"id":    "chatcmpl-1",
		"model": "gpt-test",
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": "Hello",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     10.0,
			"completion_tokens": 5.0,
		},
	}
	unified := ResponseFromOpenAIChat(payload)
	anthropic := ResponseToAnthropic(unified)
	if str(anthropic["stop_reason"]) != "stop" {
		t.Fatalf("stop_reason: %v", anthropic["stop_reason"])
	}
}

func TestResponsesRequestRoundTrip(t *testing.T) {
	payload := map[string]any{
		"model":        "gpt-5",
		"instructions": "Help the user.",
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "Hi"}},
			},
		},
	}
	unified := RequestFromOpenAIResponses(payload)
	back := RequestToOpenAIResponses(unified)
	if str(back["instructions"]) != "Help the user." {
		t.Fatalf("instructions: %v", back["instructions"])
	}
}

func TestAssistantMessageToResponsesUsesOutputText(t *testing.T) {
	out := RequestToOpenAIResponses(UnifiedRequest{
		Model: "gpt-test",
		Messages: []UnifiedMessage{
			TextMessage(RoleUser, "Hi"),
			TextMessage(RoleAssistant, "Hello"),
		},
	})
	input := asMapSlice(out["input"])
	if len(input) != 2 {
		t.Fatalf("len(input) = %d", len(input))
	}
	assistantContent := asMapSlice(input[1]["content"])
	if str(assistantContent[0]["type"]) != "output_text" {
		t.Fatalf("assistant text type = %v", assistantContent[0]["type"])
	}
}

func TestOpenAIChatImageToAnthropic(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-test",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "describe"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png", "detail": "high"}},
				},
			},
		},
	}
	req := RequestFromOpenAIChat(payload)
	if err := ValidateRequest(req, FormatOpenAIChat, FormatAnthropic); err != nil {
		t.Fatalf("validate: %v", err)
	}
	anthropic := RequestToAnthropic(req)
	msgs := asMapSlice(anthropic["messages"])
	content := asMapSlice(msgs[0]["content"])
	if str(content[1]["type"]) != "image" {
		t.Fatalf("content[1].type = %v", content[1]["type"])
	}
}

func TestResponsesFileCannotConvertToAnthropic(t *testing.T) {
	req := RequestFromOpenAIResponses(map[string]any{
		"model": "gpt-test",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_file", "file_id": "file_123"},
				},
			},
		},
	})
	err := ValidateRequest(req, FormatOpenAIResponses, FormatAnthropic)
	if err == nil {
		t.Fatal("expected unsupported file error")
	}
	if unsupported, ok := err.(*UnsupportedFeatureError); !ok || unsupported.Feature != "file_content" {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestOpenAIChatAudioCannotConvertToAnthropic(t *testing.T) {
	req := RequestFromOpenAIChat(map[string]any{
		"model": "gpt-test",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "abc", "format": "mp3"}},
				},
			},
		},
	})
	err := ValidateRequest(req, FormatOpenAIChat, FormatAnthropic)
	if err == nil {
		t.Fatal("expected unsupported audio error")
	}
}

func TestOpenAIChatToolCallStreamToAnthropic(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\""}}]}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"Shanghai\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		``,
	}, "\n")
	var out strings.Builder
	if err := PipeOpenAIChatStreamToAnthropic(strings.NewReader(stream), &out, "gpt-test"); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `"type":"tool_use"`) {
		t.Fatalf("missing tool_use in stream: %s", got)
	}
	if !strings.Contains(got, `"type":"input_json_delta"`) {
		t.Fatalf("missing input_json_delta in stream: %s", got)
	}
}

func TestOpenAIChatStreamFlushesEvents(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"content":"Hi"}}]}`,
		`data: [DONE]`,
		``,
	}, "\n")
	out := &flushBuffer{}
	if err := PipeOpenAIChatStreamToAnthropic(strings.NewReader(stream), out, "gpt-test"); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if out.flushes == 0 {
		t.Fatal("expected stream writer to be flushed")
	}
}

func TestResponsesStreamFlushesEvents(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.completed"}`,
		``,
	}, "\n")
	out := &flushBuffer{}
	if err := PipeResponsesStreamToAnthropic(strings.NewReader(stream), out, "gpt-test"); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if out.flushes == 0 {
		t.Fatal("expected stream writer to be flushed")
	}
}

func TestCodexUpstreamPatch(t *testing.T) {
	out := ApplyCodexUpstreamRequest(map[string]any{
		"model":             "codex",
		"temperature":       0.5,
		"max_output_tokens": 100.0,
		"metadata":          map[string]any{"source": "client"},
	}, true)
	if out["store"] != false {
		t.Fatal("expected store false")
	}
	if out["stream"] != true {
		t.Fatal("expected stream true")
	}
	if _, ok := out["temperature"]; ok {
		t.Fatal("temperature should be stripped")
	}
	if _, ok := out["metadata"]; ok {
		t.Fatal("metadata should be stripped")
	}
	if str(out["instructions"]) != DefaultCodexInstructions {
		t.Fatalf("instructions: %v", out["instructions"])
	}
}

func TestCodexUpstreamPatchPreservesInstructions(t *testing.T) {
	out := ApplyCodexUpstreamRequest(map[string]any{
		"model":        "codex",
		"instructions": "Use short answers.",
	}, false)
	if str(out["instructions"]) != "Use short answers." {
		t.Fatalf("instructions: %v", out["instructions"])
	}
}

func TestCollectResponsesStreamCanConvertToAnthropicJSON(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test"}}`,
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		`data: {"type":"response.output_text.delta","delta":" world"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed"}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	collected, err := CollectResponsesStream(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	anthropic := ResponseToAnthropic(ResponseFromOpenAIResponses(collected))
	content := asMapSlice(anthropic["content"])
	if len(content) != 1 || str(content[0]["text"]) != "Hello world" {
		t.Fatalf("content = %#v", anthropic["content"])
	}
}

func TestRequestConversionMatrixPreservesMixedConversation(t *testing.T) {
	strict := true
	parallel := true
	base := UnifiedRequest{
		Model:  "gpt-test",
		System: "System rules.\nDeveloper rules.",
		Messages: []UnifiedMessage{
			{
				Role: RoleUser,
				Content: []UnifiedContentBlock{
					TextBlock("Look"),
					ImageBlock(UnifiedImage{URL: "https://example.com/a.png", Detail: "high"}),
				},
			},
			{
				Role: RoleAssistant,
				Content: []UnifiedContentBlock{
					TextBlock("I will call a tool."),
					ToolCallBlock("call_1", "get_weather", map[string]any{"city": "Shanghai"}),
				},
			},
			{
				Role: RoleTool,
				Content: []UnifiedContentBlock{
					ToolResultBlock("call_1", "Sunny"),
				},
			},
			TextMessage(RoleAssistant, "Done"),
		},
		Tools: []UnifiedTool{
			{
				Name:        "get_weather",
				Description: "Get weather",
				Type:        "function",
				Strict:      &strict,
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
				},
			},
		},
		ToolChoice:        &UnifiedToolChoice{Mode: "tool", Name: "get_weather", Type: "function"},
		ParallelToolCalls: &parallel,
	}

	tests := []struct {
		name   string
		target Format
		to     func(UnifiedRequest) map[string]any
		from   func(map[string]any) UnifiedRequest
	}{
		{"anthropic", FormatAnthropic, RequestToAnthropic, RequestFromAnthropic},
		{"chat", FormatOpenAIChat, RequestToOpenAIChat, RequestFromOpenAIChat},
		{"responses", FormatOpenAIResponses, RequestToOpenAIResponses, RequestFromOpenAIResponses},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateRequest(base, FormatOpenAIResponses, tt.target); err != nil {
				t.Fatalf("validate: %v", err)
			}
			got := tt.from(tt.to(base))
			assertRequestCore(t, got, base)
		})
	}
}

func TestOpenAIChatRequestSystemDeveloperAndEmptyContent(t *testing.T) {
	req := RequestFromOpenAIChat(map[string]any{
		"model": "gpt-test",
		"messages": []any{
			map[string]any{"role": "system", "content": "System rules."},
			map[string]any{"role": "developer", "content": []any{
				map[string]any{"type": "text", "text": "Developer rules."},
			}},
			map[string]any{"role": "assistant", "content": ""},
		},
	})
	if req.System != "System rules.\nDeveloper rules." {
		t.Fatalf("system = %q", req.System)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 1 || req.Messages[0].Content[0].Text != "" {
		t.Fatalf("empty assistant content not preserved: %#v", req.Messages)
	}
}

func TestMultimodalImageSourcesAcrossFormats(t *testing.T) {
	dataURL := "data:image/png;base64,abc123"
	chatReq := RequestFromOpenAIChat(map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
			}},
		},
	})
	anthropic := RequestToAnthropic(chatReq)
	anthropicSource := asMapSlice(asMapSlice(anthropic["messages"])[0]["content"])[0]["source"].(map[string]any)
	if anthropicSource["type"] != "base64" || anthropicSource["media_type"] != "image/png" || anthropicSource["data"] != "abc123" {
		t.Fatalf("chat data URL was not converted to Anthropic base64 source: %#v", anthropicSource)
	}

	anthropicReq := RequestFromAnthropic(map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image", "source": map[string]any{"type": "file", "file_id": "file_img"}},
			}},
		},
	})
	responses := RequestToOpenAIResponses(anthropicReq)
	respImage := asMapSlice(asMapSlice(responses["input"])[0]["content"])[0]
	if respImage["file_id"] != "file_img" {
		t.Fatalf("Anthropic image file_id not preserved in Responses: %#v", respImage)
	}
}

func TestToolResultRoleConvertsToAnthropicUserMessage(t *testing.T) {
	req := UnifiedRequest{
		Messages: []UnifiedMessage{{
			Role:    RoleTool,
			Content: []UnifiedContentBlock{ToolResultBlock("call_1", "ok")},
		}},
	}
	anthropic := RequestToAnthropic(req)
	msg := asMapSlice(anthropic["messages"])[0]
	if msg["role"] != "user" {
		t.Fatalf("Anthropic tool_result role = %v", msg["role"])
	}
	content := asMapSlice(msg["content"])
	if content[0]["type"] != "tool_result" || content[0]["tool_use_id"] != "call_1" {
		t.Fatalf("tool result content = %#v", content)
	}
}

func TestUnsupportedFeatureMatrix(t *testing.T) {
	custom := UnifiedRequest{Messages: []UnifiedMessage{{
		Role: RoleAssistant,
		Content: []UnifiedContentBlock{{
			Type:     ContentToolCall,
			ToolCall: &UnifiedToolCall{ID: "call_custom", Name: "shell", RawInput: "ls", Kind: "custom"},
		}},
	}}}
	assertUnsupportedFeature(t, ValidateRequest(custom, FormatOpenAIResponses, FormatOpenAIChat), "custom_tool_call")
	assertUnsupportedFeature(t, ValidateRequest(custom, FormatOpenAIResponses, FormatAnthropic), "custom_tool_call")
	if err := ValidateRequest(custom, FormatOpenAIResponses, FormatOpenAIResponses); err != nil {
		t.Fatalf("custom tool call should stay valid for Responses: %v", err)
	}

	hostedTool := UnifiedRequest{Tools: []UnifiedTool{{Type: "web_search_preview", Name: "web_search_preview"}}}
	assertUnsupportedFeature(t, ValidateRequest(hostedTool, FormatOpenAIResponses, FormatOpenAIChat), "hosted_or_custom_tool:web_search_preview")
}

func TestResponseConversionsPreserveToolReasoningUsageAndFinish(t *testing.T) {
	responsesPayload := map[string]any{
		"id":     "resp_1",
		"model":  "gpt-test",
		"status": "completed",
		"output": []any{
			map[string]any{"type": "message", "role": "assistant", "content": []any{
				map[string]any{"type": "output_text", "text": "Hello"},
			}},
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "get_weather", "arguments": `{"city":"Shanghai"}`},
			map[string]any{"type": "reasoning", "summary": "short reasoning"},
		},
		"usage": map[string]any{"input_tokens": 11.0, "output_tokens": 7.0},
	}
	resp := ResponseFromOpenAIResponses(responsesPayload)
	if resp.FinishReason != "completed" {
		t.Fatalf("finish = %q", resp.FinishReason)
	}
	if len(resp.Message.Content) != 3 || resp.Message.Content[1].ToolCall.ID != "call_1" {
		t.Fatalf("content = %#v", resp.Message.Content)
	}
	assertUnsupportedFeature(t, ValidateResponse(resp, FormatOpenAIResponses, FormatAnthropic), "reasoning_or_thinking_content")

	resp.Message.Content = resp.Message.Content[:2]
	chat := ResponseToOpenAIChat(resp)
	usage := chat["usage"].(map[string]any)
	if usage["prompt_tokens"] != 11 || usage["completion_tokens"] != 7 || usage["total_tokens"] != 18 {
		t.Fatalf("usage = %#v", usage)
	}
	msg := asMapSlice(chat["choices"])[0]["message"].(map[string]any)
	if msg["content"] != "Hello" || len(asMapSlice(msg["tool_calls"])) != 1 {
		t.Fatalf("chat message = %#v", msg)
	}
}

func TestRequestToResponsesSplitsAssistantTextAndToolCallsWithoutDroppingBlocks(t *testing.T) {
	out := RequestToOpenAIResponses(UnifiedRequest{
		Messages: []UnifiedMessage{{
			Role: RoleAssistant,
			Content: []UnifiedContentBlock{
				TextBlock("Checking"),
				ToolCallBlock("call_1", "get_weather", map[string]any{"city": "Shanghai"}),
			},
		}},
	})
	input := asMapSlice(out["input"])
	if len(input) != 2 {
		t.Fatalf("input len = %d, input = %#v", len(input), input)
	}
	if input[0]["type"] != "message" || asMapSlice(input[0]["content"])[0]["text"] != "Checking" {
		t.Fatalf("message item = %#v", input[0])
	}
	if input[1]["type"] != "function_call" || input[1]["call_id"] != "call_1" {
		t.Fatalf("function item = %#v", input[1])
	}
}

func TestOpenAIChatStreamEventsCloseToolBlock(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\""}}]}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"Shanghai\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		``,
	}, "\n")
	var out strings.Builder
	if err := PipeOpenAIChatStreamToAnthropic(strings.NewReader(stream), &out, "gpt-test"); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	events := parseAnthropicSSE(t, out.String())
	assertEventTypes(t, events, []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	})
	if events[1].Data["index"] != float64(0) || events[4].Data["index"] != float64(0) {
		t.Fatalf("tool block indexes = start %#v stop %#v", events[1].Data, events[4].Data)
	}
}

func TestCollectStreamsMatchNonStreamingConversions(t *testing.T) {
	chatStream := strings.Join([]string{
		`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"content":"Hello "}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"content":"world"}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\""}}]}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"Shanghai\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		``,
	}, "\n")
	chatCollected, err := CollectOpenAIChatStream(strings.NewReader(chatStream))
	if err != nil {
		t.Fatalf("collect chat: %v", err)
	}
	chatResp := ResponseFromOpenAIChat(chatCollected)
	if !reflect.DeepEqual(blockSummaries(chatResp.Message.Content), []string{"text:Hello world", "tool_call:call_1:get_weather:{\"city\":\"Shanghai\"}"}) {
		t.Fatalf("chat blocks = %#v", blockSummaries(chatResp.Message.Content))
	}

	responsesStream := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test"}}`,
		`data: {"type":"response.output_text.delta","delta":"Hello world"}`,
		`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"get_weather"}}`,
		`data: {"type":"response.function_call_arguments.delta","delta":"{\"city\""}`,
		`data: {"type":"response.function_call_arguments.delta","delta":":\"Shanghai\"}"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed"}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	responsesCollected, err := CollectResponsesStream(strings.NewReader(responsesStream))
	if err != nil {
		t.Fatalf("collect responses: %v", err)
	}
	responsesResp := ResponseFromOpenAIResponses(responsesCollected)
	if !reflect.DeepEqual(blockSummaries(responsesResp.Message.Content), []string{"text:Hello world", "tool_call:call_1:get_weather:{\"city\":\"Shanghai\"}"}) {
		t.Fatalf("responses blocks = %#v", blockSummaries(responsesResp.Message.Content))
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func assertRequestCore(t *testing.T, got, want UnifiedRequest) {
	t.Helper()
	if got.System != want.System {
		t.Fatalf("system = %q, want %q", got.System, want.System)
	}
	if !reflect.DeepEqual(messageSummaries(got.Messages), messageSummaries(want.Messages)) {
		t.Fatalf("messages = %#v, want %#v", messageSummaries(got.Messages), messageSummaries(want.Messages))
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "get_weather" || got.Tools[0].Description != "Get weather" {
		t.Fatalf("tools = %#v", got.Tools)
	}
	if got.ToolChoice == nil || got.ToolChoice.Mode != "tool" || got.ToolChoice.Name != "get_weather" {
		t.Fatalf("tool_choice = %#v", got.ToolChoice)
	}
}

func messageSummaries(msgs []UnifiedMessage) []string {
	msgs = normalizeMessagesForAssert(msgs)
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		onlyToolResults := len(m.Content) > 0
		for _, b := range m.Content {
			if b.Type != ContentToolResult {
				onlyToolResults = false
				break
			}
		}
		if onlyToolResults {
			role = RoleTool
		}
		out = append(out, string(role)+"|"+strings.Join(blockSummaries(m.Content), ","))
	}
	return out
}

func normalizeMessagesForAssert(msgs []UnifiedMessage) []UnifiedMessage {
	var out []UnifiedMessage
	for _, msg := range msgs {
		if len(out) > 0 && out[len(out)-1].Role == msg.Role {
			out[len(out)-1].Content = append(out[len(out)-1].Content, msg.Content...)
			continue
		}
		out = append(out, msg)
	}
	return out
}

func blockSummaries(blocks []UnifiedContentBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case ContentText:
			out = append(out, "text:"+b.Text)
		case ContentImage:
			if b.Image != nil {
				out = append(out, "image:"+firstNonEmpty(b.Image.URL, b.Image.FileID, b.Image.MediaType+":"+b.Image.Data))
			}
		case ContentToolCall:
			if b.ToolCall != nil {
				out = append(out, "tool_call:"+b.ToolCall.ID+":"+b.ToolCall.Name+":"+mustJSON(b.ToolCall.Arguments))
			}
		case ContentToolResult:
			out = append(out, "tool_result:"+b.ToolCallID+":"+b.Text)
		case ContentReasoning:
			if b.Reasoning != nil {
				out = append(out, "reasoning:"+b.Reasoning.Text)
			}
		default:
			out = append(out, string(b.Type))
		}
	}
	return out
}

func assertUnsupportedFeature(t *testing.T, err error, feature string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected unsupported feature %q", feature)
	}
	unsupported, ok := err.(*UnsupportedFeatureError)
	if !ok {
		t.Fatalf("err = %#v, want *UnsupportedFeatureError", err)
	}
	if unsupported.Feature != feature {
		t.Fatalf("feature = %q, want %q", unsupported.Feature, feature)
	}
}

type sseEvent struct {
	Event string
	Data  map[string]any
}

func parseAnthropicSSE(t *testing.T, raw string) []sseEvent {
	t.Helper()
	chunks := strings.Split(strings.TrimSpace(raw), "\n\n")
	events := make([]sseEvent, 0, len(chunks))
	for _, chunk := range chunks {
		var event sseEvent
		for _, line := range strings.Split(chunk, "\n") {
			if strings.HasPrefix(line, "event: ") {
				event.Event = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event.Data); err != nil {
					t.Fatalf("bad data line %q: %v", line, err)
				}
			}
		}
		if event.Event != "" {
			events = append(events, event)
		}
	}
	return events
}

func assertEventTypes(t *testing.T, events []sseEvent, want []string) {
	t.Helper()
	got := make([]string, 0, len(events))
	for _, event := range events {
		got = append(got, event.Event)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

type flushBuffer struct {
	bytes.Buffer
	flushes int
}

func (b *flushBuffer) Flush() {
	b.flushes++
}
