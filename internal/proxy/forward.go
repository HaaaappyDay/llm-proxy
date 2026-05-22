package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/lotus/llm-proxy/internal/app"
	"github.com/lotus/llm-proxy/internal/auth"
	"github.com/lotus/llm-proxy/internal/transform"
)

type Forwarder struct {
	app    *app.App
	client *http.Client
}

func NewForwarder(application *app.App) *Forwarder {
	return &Forwarder{
		app: application,
		client: &http.Client{
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

func (f *Forwarder) postUpstream(url string, headers http.Header, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	return f.client.Do(req)
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
	upstreamBody := transform.AnthropicToCodexResponses(anthropic)
	body, err := json.Marshal(upstreamBody)
	if err != nil {
		return err
	}
	headers := auth.CodexHeaders(token, accountID)
	resp, err := f.postUpstream(auth.CodexUpstreamResponsesURL, headers, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, string(b))
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	return transform.PipeResponsesStreamToAnthropic(resp.Body, w, model)
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
		upstream := transform.RequestToOpenAIResponses(unified)
		if stream {
			upstream["stream"] = true
		}
		body, _ := json.Marshal(upstream)
		url := endpoint + "/responses"
		headers := auth.CopilotHeaders(token)
		resp, err := f.postUpstream(url, headers, body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("upstream %d: %s", resp.StatusCode, string(b))
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
	if stream {
		upstream["stream"] = true
	}
	body, _ := json.Marshal(upstream)
	url := endpoint + "/chat/completions"
	headers := auth.CopilotHeaders(token)
	resp, err := f.postUpstream(url, headers, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, string(b))
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
		resp, err := f.postUpstream(endpoint+"/chat/completions", auth.CopilotHeaders(token), body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		copyUpstream(w, resp)
		return nil
	case auth.ProviderCodexOAuth:
		unified := transform.RequestFromOpenAIChat(payload)
		upstream := transform.ApplyCodexUpstreamRequest(transform.RequestToOpenAIResponses(unified), stream)
		token, err := f.app.Codex.GetValidToken(accountID)
		if err != nil {
			return err
		}
		body, _ := json.Marshal(upstream)
		resp, err := f.postUpstream(auth.CodexUpstreamResponsesURL, auth.CodexHeaders(token, accountID), body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if stream {
			copyUpstream(w, resp)
			return nil
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
		resp, err := f.postUpstream(auth.CodexUpstreamResponsesURL, auth.CodexHeaders(token, accountID), body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		copyUpstream(w, resp)
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
		resp, err := f.postUpstream(endpoint+"/responses", auth.CopilotHeaders(token), body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		copyUpstream(w, resp)
		return nil
	default:
		return fmt.Errorf("unsupported provider")
	}
}

func copyUpstream(w http.ResponseWriter, resp *http.Response) {
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

// Doctor checks data directory permissions.
func Doctor(dataDir string) []string {
	var issues []string
	if dataDir == "" {
		issues = append(issues, "data directory not configured")
		return issues
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			issues = append(issues, "data directory does not exist yet (will be created on first login)")
		} else {
			issues = append(issues, "cannot stat data directory: "+err.Error())
		}
		return issues
	}
	if !info.IsDir() {
		issues = append(issues, "data path is not a directory")
	}
	return issues
}
