package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/HaaapyDay/llm-proxy/internal/app"
	"github.com/HaaapyDay/llm-proxy/internal/auth"
	"github.com/HaaapyDay/llm-proxy/internal/config"
)

func TestProxyAnthropicMessagesToCodexResponses(t *testing.T) {
	application := testAppWithCachedAuth(t)
	transport := &captureTransport{body: responsesSSE("resp_1", "gpt-test", "Hello")}
	forwarder := NewForwarder(application)
	forwarder.requestClient = &http.Client{Transport: transport}

	rec := &auth.APIKeyRecord{Provider: auth.ProviderCodexOAuth, AccountID: "acct_1"}
	raw := []byte(`{"model":"gpt-test","system":"Be brief","messages":[{"role":"user","content":"Hi"}]}`)
	w := httptest.NewRecorder()

	if err := forwarder.HandleAnthropicMessages(w, rec, raw); err != nil {
		t.Fatalf("HandleAnthropicMessages: %v", err)
	}
	if transport.url != auth.CodexUpstreamResponsesURL {
		t.Fatalf("upstream url = %s", transport.url)
	}
	var upstream map[string]any
	mustDecodeJSON(t, transport.requestBody, &upstream)
	if _, ok := upstream["messages"]; ok {
		t.Fatalf("unexpected Anthropic messages in upstream body: %#v", upstream)
	}
	if upstream["instructions"] != "Be brief" || len(asMapSliceProxy(upstream["input"])) != 1 {
		t.Fatalf("upstream body = %#v", upstream)
	}
	var out map[string]any
	mustDecodeJSON(t, w.Body.Bytes(), &out)
	if out["type"] != "message" || asMapSliceProxy(out["content"])[0]["text"] != "Hello" {
		t.Fatalf("response = %s", w.Body.String())
	}
}

func TestProxyChatCompletionsToCodexResponses(t *testing.T) {
	application := testAppWithCachedAuth(t)
	transport := &captureTransport{body: responsesSSE("resp_1", "gpt-test", "Hello")}
	forwarder := NewForwarder(application)
	forwarder.requestClient = &http.Client{Transport: transport}

	rec := &auth.APIKeyRecord{Provider: auth.ProviderCodexOAuth, AccountID: "acct_1"}
	raw := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"Hi"}]}`)
	w := httptest.NewRecorder()

	if err := forwarder.HandleOpenAIChat(w, rec, raw); err != nil {
		t.Fatalf("HandleOpenAIChat: %v", err)
	}
	var upstream map[string]any
	mustDecodeJSON(t, transport.requestBody, &upstream)
	if _, ok := upstream["messages"]; ok {
		t.Fatalf("unexpected Chat messages in upstream body: %#v", upstream)
	}
	if len(asMapSliceProxy(upstream["input"])) != 1 {
		t.Fatalf("upstream body = %#v", upstream)
	}
	var out map[string]any
	mustDecodeJSON(t, w.Body.Bytes(), &out)
	choice := asMapSliceProxy(out["choices"])[0]
	msg := choice["message"].(map[string]any)
	if msg["content"] != "Hello" {
		t.Fatalf("response = %s", w.Body.String())
	}
}

func TestProxyResponsesDirectPathDoesNotConvertRequestOrResponse(t *testing.T) {
	application := testAppWithCachedAuth(t)
	upstreamResp := `{"id":"resp_direct","model":"gpt-test","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Direct"}]}]}`
	transport := &captureTransport{body: upstreamResp}
	forwarder := NewForwarder(application)
	forwarder.requestClient = &http.Client{Transport: transport}

	rec := &auth.APIKeyRecord{Provider: auth.ProviderCodexOAuth, AccountID: "acct_1"}
	raw := []byte(`{"model":"gpt-test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Hi"}]}]}`)
	w := httptest.NewRecorder()

	if err := forwarder.HandleOpenAIResponses(w, rec, raw); err != nil {
		t.Fatalf("HandleOpenAIResponses: %v", err)
	}
	var upstream map[string]any
	mustDecodeJSON(t, transport.requestBody, &upstream)
	if _, ok := upstream["messages"]; ok {
		t.Fatalf("unexpected converted messages in upstream body: %#v", upstream)
	}
	if len(asMapSliceProxy(upstream["input"])) != 1 {
		t.Fatalf("upstream body = %#v", upstream)
	}
	if !strings.Contains(w.Body.String(), `"id":"resp_direct"`) || !strings.Contains(w.Body.String(), `"output_text"`) {
		t.Fatalf("response should be copied from upstream: %s", w.Body.String())
	}
}

func TestProxyAnthropicMessagesToCopilotChatAndResponses(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		wantPath   string
		body       string
		wantField  string
		wantAbsent string
	}{
		{
			name:       "chat",
			model:      "claude-test",
			wantPath:   "/chat/completions",
			body:       `{"id":"chatcmpl_1","model":"claude-test","choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}]}`,
			wantField:  "messages",
			wantAbsent: "input",
		},
		{
			name:       "responses",
			model:      "gpt-test",
			wantPath:   "/responses",
			body:       `{"id":"resp_1","model":"gpt-test","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}]}`,
			wantField:  "input",
			wantAbsent: "messages",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			application := testAppWithCachedAuth(t)
			transport := &captureTransport{body: tt.body}
			forwarder := NewForwarder(application)
			forwarder.requestClient = &http.Client{Transport: transport}

			rec := &auth.APIKeyRecord{Provider: auth.ProviderGitHubCopilot, AccountID: "acct_1"}
			raw := []byte(`{"model":"` + tt.model + `","messages":[{"role":"user","content":"Hi"}]}`)
			w := httptest.NewRecorder()

			if err := forwarder.HandleAnthropicMessages(w, rec, raw); err != nil {
				t.Fatalf("HandleAnthropicMessages: %v", err)
			}
			if !strings.HasSuffix(transport.url, tt.wantPath) {
				t.Fatalf("upstream url = %s", transport.url)
			}
			var upstream map[string]any
			mustDecodeJSON(t, transport.requestBody, &upstream)
			if _, ok := upstream[tt.wantField]; !ok {
				t.Fatalf("missing %q in upstream body: %#v", tt.wantField, upstream)
			}
			if _, ok := upstream[tt.wantAbsent]; ok {
				t.Fatalf("unexpected %q in upstream body: %#v", tt.wantAbsent, upstream)
			}
			if !strings.Contains(w.Body.String(), `"type":"message"`) || !strings.Contains(w.Body.String(), `"text":"Hello"`) {
				t.Fatalf("response = %s", w.Body.String())
			}
		})
	}
}

type captureTransport struct {
	url         string
	requestBody []byte
	body        string
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.url = req.URL.String()
	var err error
	t.requestBody, err = io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(t.body)),
	}, nil
}

func testAppWithCachedAuth(t *testing.T) *app.App {
	t.Helper()
	application := app.New(&config.Config{DataDir: t.TempDir()})
	setPrivateField(t, application.Codex, "accessTokens", map[string]auth.CachedAccessToken{
		"acct_1": {Token: "codex-token", ExpiresAtMs: time.Now().Add(time.Hour).UnixMilli()},
	})
	setPrivateField(t, application.Copilot, "copilotTokens", map[string]auth.CopilotToken{
		"acct_1": {Token: "copilot-token", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	})
	setPrivateField(t, application.Copilot, "apiEndpoints", map[string]string{
		"acct_1": "https://copilot.example.test",
	})
	return application
}

func setPrivateField(t *testing.T, target any, name string, value any) {
	t.Helper()
	field := reflect.ValueOf(target).Elem().FieldByName(name)
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func responsesSSE(id, model, text string) string {
	payload, _ := json.Marshal(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":     id,
			"model":  model,
			"status": "completed",
			"output": []map[string]any{{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": text,
				}},
			}},
		},
	})
	return "data: " + string(payload) + "\n\ndata: [DONE]\n\n"
}

func mustDecodeJSON(t *testing.T, raw []byte, out any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		t.Fatalf("decode json %q: %v", string(raw), err)
	}
}

func asMapSliceProxy(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		out := make([]map[string]any, 0, len(t))
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}
