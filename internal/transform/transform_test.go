package transform

import (
	"encoding/json"
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
						"type":         "tool_result",
						"tool_use_id":  "call_1",
						"content":      "Sunny",
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

func TestCodexUpstreamPatch(t *testing.T) {
	out := ApplyCodexUpstreamRequest(map[string]any{
		"model":             "codex",
		"temperature":       0.5,
		"max_output_tokens": 100.0,
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
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
