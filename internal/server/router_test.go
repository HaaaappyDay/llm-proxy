package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HaaapyDay/llm-proxy/internal/app"
	"github.com/HaaapyDay/llm-proxy/internal/auth"
	"github.com/HaaapyDay/llm-proxy/internal/config"
	"github.com/HaaapyDay/llm-proxy/internal/server"
)

func TestManagementRoutesAreNotExposed(t *testing.T) {
	router := server.NewRouter(app.New(&config.Config{
		ListenHost: config.DefaultListenHost,
		ListenPort: config.DefaultListenPort,
		DataDir:    t.TempDir(),
	}))

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/auth/codex/device", ""},
		{http.MethodPost, "/api/v1/auth/codex/poll", `{"device_code":"x"}`},
		{http.MethodPost, "/api/v1/auth/copilot/device", ""},
		{http.MethodPost, "/api/v1/auth/copilot/poll", `{"device_code":"x"}`},
		{http.MethodGet, "/api/v1/accounts", ""},
		{http.MethodPost, "/api/v1/keys", `{"provider":"codex"}`},
		{http.MethodGet, "/api/v1/keys", ""},
		{http.MethodDelete, "/api/v1/keys/key-id", ""},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRevokedAPIKeyIsRejected(t *testing.T) {
	application := app.New(&config.Config{
		ListenHost: config.DefaultListenHost,
		ListenPort: config.DefaultListenPort,
		DataDir:    t.TempDir(),
	})
	result, err := application.APIKeys.Create(auth.CreateKeyInput{
		Label:     "test",
		Provider:  auth.ProviderCodexOAuth,
		AccountID: "acct_1",
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := application.APIKeys.Delete(result.Record.ID); err != nil {
		t.Fatalf("delete key: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"gpt-test","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+result.Plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.NewRouter(application).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	requireErrorType(t, rec.Body.Bytes(), "invalid_api_key")
}

func TestModelsRequiresAPIKey(t *testing.T) {
	application := app.New(&config.Config{
		ListenHost: config.DefaultListenHost,
		ListenPort: config.DefaultListenPort,
		DataDir:    t.TempDir(),
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	server.NewRouter(application).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	requireErrorType(t, rec.Body.Bytes(), "invalid_api_key")
}

func TestCodexModelsReturnsOpenAICompatibleList(t *testing.T) {
	application := app.New(&config.Config{
		ListenHost: config.DefaultListenHost,
		ListenPort: config.DefaultListenPort,
		DataDir:    t.TempDir(),
	})
	result, err := application.APIKeys.Create(auth.CreateKeyInput{
		Label:     "test",
		Provider:  auth.ProviderCodexOAuth,
		AccountID: "acct_1",
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+result.Plaintext)
	rec := httptest.NewRecorder()

	server.NewRouter(application).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"object":"list"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"gpt-5.2"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestUnsupportedFeatureReturnsStructuredClientError(t *testing.T) {
	application := app.New(&config.Config{
		ListenHost: config.DefaultListenHost,
		ListenPort: config.DefaultListenPort,
		DataDir:    t.TempDir(),
	})
	result, err := application.APIKeys.Create(auth.CreateKeyInput{
		Label:     "test",
		Provider:  auth.ProviderCodexOAuth,
		AccountID: "acct_1",
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	body := `{
		"model": "gpt-test",
		"messages": [{
			"role": "user",
			"content": [{"type": "input_audio", "input_audio": {"data": "abc", "format": "mp3"}}]
		}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+result.Plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.NewRouter(application).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported_feature") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestRequestBodyTooLargeReturns413(t *testing.T) {
	application := app.New(&config.Config{
		ListenHost: config.DefaultListenHost,
		ListenPort: config.DefaultListenPort,
		DataDir:    t.TempDir(),
	})
	result, err := application.APIKeys.Create(auth.CreateKeyInput{
		Label:     "test",
		Provider:  auth.ProviderCodexOAuth,
		AccountID: "acct_1",
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	body := bytes.NewReader(bytes.Repeat([]byte("x"), 32<<20+1))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("Authorization", "Bearer "+result.Plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.NewRouter(application).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	requireErrorType(t, rec.Body.Bytes(), "request_too_large")
}

func TestInvalidJSONReturnsStructuredClientError(t *testing.T) {
	application := app.New(&config.Config{
		ListenHost: config.DefaultListenHost,
		ListenPort: config.DefaultListenPort,
		DataDir:    t.TempDir(),
	})
	result, err := application.APIKeys.Create(auth.CreateKeyInput{
		Label:     "test",
		Provider:  auth.ProviderCodexOAuth,
		AccountID: "acct_1",
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":`))
	req.Header.Set("Authorization", "Bearer "+result.Plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.NewRouter(application).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	requireErrorType(t, rec.Body.Bytes(), "invalid_request")
}

func requireErrorType(t *testing.T, body []byte, want string) {
	t.Helper()
	var out struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("response is not json: %v; body = %s", err, body)
	}
	if out.Error.Type != want {
		t.Fatalf("error.type = %q, want %q; body = %s", out.Error.Type, want, body)
	}
}
