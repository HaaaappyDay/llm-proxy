package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestParseBrowserCallbackValidation(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "state mismatch", raw: "/auth/callback?code=abc&state=wrong", want: "state mismatch"},
		{name: "oauth error", raw: "/auth/callback?error=access_denied&state=ok", want: "authorization failed"},
		{name: "missing code", raw: "/auth/callback?state=ok", want: "missing authorization code"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.raw, nil)
			got := parseBrowserCallback(req, "ok")
			if got.err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(got.err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", got.err.Error(), tt.want)
			}
		})
	}
}

func TestCodexBrowserLoginSuccess(t *testing.T) {
	m := NewCodexOAuthManager(t.TempDir())
	var gotForm url.Values
	m.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost || r.URL.String() != CodexOAuthTokenURL {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			gotForm, err = url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse form: %v", err)
			}
			resp := oauthTokenResp{
				AccessToken:  testJWT(t, "acct_browser", "user@example.com"),
				RefreshToken: "refresh-token",
				IDToken:      testJWT(t, "acct_browser", "user@example.com"),
				ExpiresIn:    3600,
			}
			data, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(data))),
			}, nil
		}),
	}

	login := &BrowserLogin{
		CallbackURL:  "http://localhost:1455/auth/callback",
		resultCh:     make(chan browserCallbackResult, 1),
		codeVerifier: "verifier",
		redirectURI:  "http://localhost:1455/auth/callback",
	}
	login.resultCh <- browserCallbackResult{code: "auth-code"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	account, err := m.CompleteBrowserLogin(ctx, login)
	if err != nil {
		t.Fatalf("CompleteBrowserLogin: %v", err)
	}
	if account.ID != "acct_browser" || account.Login != "user@example.com" {
		t.Fatalf("account = %+v", account)
	}
	if gotForm.Get("code") != "auth-code" {
		t.Fatalf("code form value = %q", gotForm.Get("code"))
	}
	if gotForm.Get("redirect_uri") != login.CallbackURL {
		t.Fatalf("redirect_uri = %q, want %q", gotForm.Get("redirect_uri"), login.CallbackURL)
	}
	if gotForm.Get("code_verifier") == "" {
		t.Fatal("missing code_verifier")
	}

	data, err := os.ReadFile(filepath.Join(m.dataDir, "codex_oauth_auth.json"))
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if !strings.Contains(string(data), "refresh-token") {
		t.Fatalf("store does not contain refresh token: %s", string(data))
	}
}

func TestCodexBrowserAuthorizeURL(t *testing.T) {
	raw, err := codexBrowserAuthorizeURL("http://localhost:1455/auth/callback", "challenge", "state")
	if err != nil {
		t.Fatalf("codexBrowserAuthorizeURL: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	if u.String() == "" {
		t.Fatal("empty URL")
	}
	if u.Query().Get("state") != "state" {
		t.Fatalf("state = %q", u.Query().Get("state"))
	}
	if u.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method = %q", u.Query().Get("code_challenge_method"))
	}
	if u.Query().Get("redirect_uri") != "http://localhost:1455/auth/callback" {
		t.Fatalf("redirect_uri = %q", u.Query().Get("redirect_uri"))
	}
}

func testJWT(t *testing.T, accountID, email string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadBytes, err := json.Marshal(map[string]string{
		"chatgpt_account_id": accountID,
		"email":              email,
	})
	if err != nil {
		t.Fatalf("marshal jwt payload: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + "."
}
