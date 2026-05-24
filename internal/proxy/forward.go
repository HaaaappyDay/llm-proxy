package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/HaaapyDay/llm-proxy/internal/app"
	"github.com/HaaapyDay/llm-proxy/internal/auth"
	"github.com/HaaapyDay/llm-proxy/internal/transform"
)

const (
	defaultUpstreamTimeout           = 2 * time.Minute
	maxUpstreamErrorBodyPreviewBytes = 4 << 10
)

type Forwarder struct {
	app           *app.App
	requestClient *http.Client
	streamClient  *http.Client
}

func NewForwarder(application *app.App) *Forwarder {
	return &Forwarder{
		app: application,
		requestClient: &http.Client{
			Timeout: defaultUpstreamTimeout,
		},
		streamClient: &http.Client{
			Timeout: 0, // streaming has no timeout
		},
	}
}

func (f *Forwarder) accountID(rec *auth.APIKeyRecord) string {
	if rec.AccountID != "" {
		return rec.AccountID
	}
	switch rec.Provider {
	case auth.ProviderCodexOAuth:
		return f.app.Codex.DefaultAccountID()
	case auth.ProviderGitHubCopilot:
		return f.app.Copilot.DefaultAccountID()
	}
	return ""
}

func (f *Forwarder) postUpstream(label, model, url string, headers http.Header, body []byte, stream bool) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	return f.doUpstream(label, model, url, stream, req)
}

func (f *Forwarder) getUpstream(label, model, url string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	return f.doUpstream(label, model, url, false, req)
}

func (f *Forwarder) doUpstream(label, model, url string, stream bool, req *http.Request) (*http.Response, error) {
	start := time.Now()
	client := f.requestClient
	if stream {
		client = f.streamClient
	}
	resp, err := client.Do(req)
	if err != nil {
		f.debugf("upstream method=%s label=%s model=%s stream=%t url=%s error=%q duration=%s", req.Method, label, safeLogValue(model), stream, url, err.Error(), time.Since(start).Round(time.Millisecond))
		return nil, err
	}
	f.debugf("upstream method=%s label=%s model=%s stream=%t url=%s status=%d duration=%s", req.Method, label, safeLogValue(model), stream, url, resp.StatusCode, time.Since(start).Round(time.Millisecond))
	return resp, nil
}

func (f *Forwarder) debugf(format string, args ...any) {
	if f.app == nil || f.app.Config == nil || !f.app.Config.Debug {
		return
	}
	fmt.Fprintf(os.Stderr, "llm-proxy debug: "+format+"\n", args...)
}

func (f *Forwarder) HandleModels(w http.ResponseWriter, rec *auth.APIKeyRecord) error {
	accountID := f.accountID(rec)

	switch rec.Provider {
	case auth.ProviderCodexOAuth:
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(openAIModelList(defaultCodexModels()))
	case auth.ProviderGitHubCopilot:
		token, err := f.app.Copilot.GetValidCopilotToken(accountID)
		if err != nil {
			return err
		}
		endpoint, err := f.app.Copilot.GetAPIEndpoint(accountID)
		if err != nil {
			return err
		}
		resp, err := f.getUpstream("copilot.models", "", endpoint+"/models", auth.CopilotHeaders(token))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		f.copyUpstream("copilot.models", "", w, resp)
		return nil
	default:
		return fmt.Errorf("unsupported provider: %s", rec.Provider)
	}
}

func (f *Forwarder) HandleAnthropicMessages(w http.ResponseWriter, rec *auth.APIKeyRecord, raw []byte) error {
	payload, err := transform.ReadJSONBody(raw)
	if err != nil {
		return err
	}
	stream := transform.IsStreamRequested(payload)
	model := strMap(payload, "model")

	switch rec.Provider {
	case auth.ProviderCodexOAuth:
		return f.forwardCodexAnthropic(w, rec, payload, stream, model)
	case auth.ProviderGitHubCopilot:
		return f.forwardCopilotAnthropic(w, rec, payload, stream, model)
	default:
		return fmt.Errorf("unsupported provider: %s", rec.Provider)
	}
}

func (f *Forwarder) forwardCodexAnthropic(w http.ResponseWriter, rec *auth.APIKeyRecord, anthropic map[string]any, stream bool, model string) error {
	accountID := f.accountID(rec)
	token, err := f.app.Codex.GetValidToken(accountID)
	if err != nil {
		return err
	}
	if err := transform.ValidateRequest(transform.RequestFromAnthropic(anthropic), transform.FormatAnthropic, transform.FormatOpenAIResponses); err != nil {
		return err
	}
	upstreamBody := transform.AnthropicToCodexResponses(anthropic)
	body, err := json.Marshal(upstreamBody)
	if err != nil {
		return err
	}
	headers := auth.CodexHeaders(token, accountID)
	resp, err := f.postUpstream("codex.anthropic.messages", model, auth.CodexUpstreamResponsesURL, headers, body, stream)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := f.checkUpstreamStatus("codex.anthropic.messages", model, resp); err != nil {
		return err
	}
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		return transform.PipeResponsesStreamToAnthropic(resp.Body, w, model)
	}
	collected, err := transform.CollectResponsesStream(resp.Body)
	if err != nil {
		return err
	}
	anth := transform.ResponseToAnthropic(transform.ResponseFromOpenAIResponses(collected))
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(anth)
}

func (f *Forwarder) forwardCopilotAnthropic(w http.ResponseWriter, rec *auth.APIKeyRecord, anthropic map[string]any, stream bool, model string) error {
	accountID := f.accountID(rec)
	token, err := f.app.Copilot.GetValidCopilotToken(accountID)
	if err != nil {
		return err
	}
	endpoint, err := f.app.Copilot.GetAPIEndpoint(accountID)
	if err != nil {
		return err
	}
	unified := transform.RequestFromAnthropic(anthropic)
	useResponses := isOpenAIVendorModel(model)

	if useResponses {
		if err := transform.ValidateRequest(unified, transform.FormatAnthropic, transform.FormatOpenAIResponses); err != nil {
			return err
		}
		upstream := transform.RequestToOpenAIResponses(unified)
		if stream {
			upstream["stream"] = true
		}
		body, _ := json.Marshal(upstream)
		url := endpoint + "/responses"
		headers := auth.CopilotHeaders(token)
		resp, err := f.postUpstream("copilot.anthropic.responses", model, url, headers, body, stream)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if err := f.checkUpstreamStatus("copilot.anthropic.responses", model, resp); err != nil {
			return err
		}
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			return transform.PipeResponsesStreamToAnthropic(resp.Body, w, model)
		}
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return err
		}
		anth := transform.ResponseToAnthropic(transform.ResponseFromOpenAIResponses(out))
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(anth)
	}

	upstream := transform.RequestToOpenAIChat(unified)
	if err := transform.ValidateRequest(unified, transform.FormatAnthropic, transform.FormatOpenAIChat); err != nil {
		return err
	}
	if stream {
		upstream["stream"] = true
	}
	body, _ := json.Marshal(upstream)
	url := endpoint + "/chat/completions"
	headers := auth.CopilotHeaders(token)
	resp, err := f.postUpstream("copilot.anthropic.chat", model, url, headers, body, stream)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := f.checkUpstreamStatus("copilot.anthropic.chat", model, resp); err != nil {
		return err
	}
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		return transform.PipeOpenAIChatStreamToAnthropic(resp.Body, w, model)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	anth := transform.ResponseToAnthropic(transform.ResponseFromOpenAIChat(out))
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(anth)
}

func (f *Forwarder) HandleOpenAIChat(w http.ResponseWriter, rec *auth.APIKeyRecord, raw []byte) error {
	payload, err := transform.ReadJSONBody(raw)
	if err != nil {
		return err
	}
	stream := transform.IsStreamRequested(payload)
	accountID := f.accountID(rec)

	switch rec.Provider {
	case auth.ProviderGitHubCopilot:
		token, err := f.app.Copilot.GetValidCopilotToken(accountID)
		if err != nil {
			return err
		}
		endpoint, err := f.app.Copilot.GetAPIEndpoint(accountID)
		if err != nil {
			return err
		}
		if stream {
			payload["stream"] = true
		}
		body, _ := json.Marshal(payload)
		resp, err := f.postUpstream("copilot.chat.completions", strMap(payload, "model"), endpoint+"/chat/completions", auth.CopilotHeaders(token), body, stream)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		f.copyUpstream("copilot.chat.completions", strMap(payload, "model"), w, resp)
		return nil
	case auth.ProviderCodexOAuth:
		unified := transform.RequestFromOpenAIChat(payload)
		if err := transform.ValidateRequest(unified, transform.FormatOpenAIChat, transform.FormatOpenAIResponses); err != nil {
			return err
		}
		upstream := transform.ApplyCodexUpstreamRequest(transform.RequestToOpenAIResponses(unified), stream)
		token, err := f.app.Codex.GetValidToken(accountID)
		if err != nil {
			return err
		}
		body, _ := json.Marshal(upstream)
		resp, err := f.postUpstream("codex.chat.completions", unified.Model, auth.CodexUpstreamResponsesURL, auth.CodexHeaders(token, accountID), body, stream)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if stream {
			f.copyUpstream("codex.chat.completions", unified.Model, w, resp)
			return nil
		}
		if err := f.checkUpstreamStatus("codex.chat.completions", unified.Model, resp); err != nil {
			return err
		}
		collected, err := transform.CollectResponsesStream(resp.Body)
		if err != nil {
			return err
		}
		out := transform.ResponseToOpenAIChat(transform.ResponseFromOpenAIResponses(collected))
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(out)
	default:
		return fmt.Errorf("unsupported provider")
	}
}

func (f *Forwarder) HandleOpenAIResponses(w http.ResponseWriter, rec *auth.APIKeyRecord, raw []byte) error {
	payload, err := transform.ReadJSONBody(raw)
	if err != nil {
		return err
	}
	stream := transform.IsStreamRequested(payload)
	accountID := f.accountID(rec)

	switch rec.Provider {
	case auth.ProviderCodexOAuth:
		token, err := f.app.Codex.GetValidToken(accountID)
		if err != nil {
			return err
		}
		upstream := transform.ApplyCodexUpstreamRequest(payload, stream)
		body, _ := json.Marshal(upstream)
		resp, err := f.postUpstream("codex.responses", strMap(payload, "model"), auth.CodexUpstreamResponsesURL, auth.CodexHeaders(token, accountID), body, stream)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		f.copyUpstream("codex.responses", strMap(payload, "model"), w, resp)
		return nil
	case auth.ProviderGitHubCopilot:
		token, err := f.app.Copilot.GetValidCopilotToken(accountID)
		if err != nil {
			return err
		}
		endpoint, err := f.app.Copilot.GetAPIEndpoint(accountID)
		if err != nil {
			return err
		}
		if stream {
			payload["stream"] = true
		}
		body, _ := json.Marshal(payload)
		resp, err := f.postUpstream("copilot.responses", strMap(payload, "model"), endpoint+"/responses", auth.CopilotHeaders(token), body, stream)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		f.copyUpstream("copilot.responses", strMap(payload, "model"), w, resp)
		return nil
	default:
		return fmt.Errorf("unsupported provider")
	}
}

func copyUpstream(w http.ResponseWriter, resp *http.Response) {
	if resp.StatusCode >= 400 {
		writeUpstreamStatusError(w, newUpstreamStatusError(resp))
		return
	}
	for k, vals := range resp.Header {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (f *Forwarder) copyUpstream(label, model string, w http.ResponseWriter, resp *http.Response) {
	if resp.StatusCode >= 400 {
		err := newUpstreamStatusError(resp)
		f.debugf("upstream_error label=%s model=%s status=%d truncated=%t preview=%q", label, safeLogValue(model), err.StatusCode, err.Truncated, err.Preview)
		writeUpstreamStatusError(w, err)
		return
	}
	copyUpstream(w, resp)
}

type UpstreamStatusError struct {
	StatusCode int
	Preview    string
	Truncated  bool
}

func (e *UpstreamStatusError) Error() string {
	return fmt.Sprintf("upstream returned status %d", e.StatusCode)
}

func checkUpstreamStatus(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	return newUpstreamStatusError(resp)
}

func (f *Forwarder) checkUpstreamStatus(label, model string, resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	err := newUpstreamStatusError(resp)
	f.debugf("upstream_error label=%s model=%s status=%d truncated=%t preview=%q", label, safeLogValue(model), err.StatusCode, err.Truncated, err.Preview)
	return err
}

func newUpstreamStatusError(resp *http.Response) *UpstreamStatusError {
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamErrorBodyPreviewBytes+1))
	if err != nil {
		return &UpstreamStatusError{
			StatusCode: resp.StatusCode,
			Preview:    "could not read upstream error body: " + err.Error(),
		}
	}
	truncated := int64(len(b)) > maxUpstreamErrorBodyPreviewBytes
	if truncated {
		b = b[:maxUpstreamErrorBodyPreviewBytes]
	}
	return &UpstreamStatusError{
		StatusCode: resp.StatusCode,
		Preview:    strings.TrimSpace(string(b)),
		Truncated:  truncated,
	}
}

func writeUpstreamStatusError(w http.ResponseWriter, err *UpstreamStatusError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.StatusCode)
	_ = json.NewEncoder(w).Encode(upstreamErrorResponse(err))
}

func upstreamErrorResponse(err *UpstreamStatusError) map[string]any {
	out := map[string]any{
		"error": map[string]any{
			"type":            "upstream_error",
			"message":         err.Error(),
			"upstream_status": err.StatusCode,
		},
	}
	if err.Truncated {
		out["error"].(map[string]any)["body_truncated"] = true
	}
	return out
}

func isOpenAIVendorModel(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "gpt-") || strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "o4")
}

func strMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func safeLogValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if len(value) > 128 {
		return value[:128] + "...(truncated)"
	}
	return value
}

func defaultCodexModels() []string {
	return []string{
		"gpt-5.4",
		"gpt-5.2",
		"gpt-5.2-codex",
		"gpt-5.1-codex",
		"gpt-5-codex",
	}
}

func openAIModelList(ids []string) map[string]any {
	data := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		data = append(data, map[string]any{
			"id":       id,
			"object":   "model",
			"created":  0,
			"owned_by": "llm-proxy",
		})
	}
	return map[string]any{
		"object": "list",
		"data":   data,
	}
}
