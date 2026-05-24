package auth

import "time"

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       uint64 `json:"expires_in"`
	Interval        uint64 `json:"interval"`
}

type Account struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	AvatarURL       string `json:"avatar_url,omitempty"`
	AuthenticatedAt int64  `json:"authenticated_at"`
	GitHubDomain    string `json:"github_domain,omitempty"`
	Provider        string `json:"provider"`
}

type CodexAccountData struct {
	AccountID       string `json:"account_id"`
	Email           string `json:"email,omitempty"`
	RefreshToken    string `json:"refresh_token"`
	AuthenticatedAt int64  `json:"authenticated_at"`
}

type CodexOAuthStore struct {
	Version          uint32                      `json:"version"`
	Accounts         map[string]CodexAccountData `json:"accounts"`
	DefaultAccountID string                      `json:"default_account_id,omitempty"`
}

type GitHubUser struct {
	Login     string `json:"login"`
	ID        uint64 `json:"id"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type CopilotAccountData struct {
	GitHubToken     string     `json:"github_token"`
	User            GitHubUser `json:"user"`
	AuthenticatedAt int64      `json:"authenticated_at"`
	GitHubDomain    string     `json:"github_domain"`
}

type CopilotAuthStore struct {
	Version          uint32                        `json:"version"`
	Accounts         map[string]CopilotAccountData `json:"accounts"`
	DefaultAccountID string                        `json:"default_account_id,omitempty"`
}

type CopilotToken struct {
	Token     string
	ExpiresAt int64
}

func (t *CopilotToken) IsExpiringSoon() bool {
	return t.ExpiresAt-time.Now().Unix() < TokenRefreshBufferSec
}

type CachedAccessToken struct {
	Token       string
	ExpiresAtMs int64
}

func (c *CachedAccessToken) IsExpiringSoon() bool {
	return c.ExpiresAtMs-time.Now().UnixMilli() < TokenRefreshBufferMs
}

func computeExpiresAtMs(expiresInSec int64) int64 {
	if expiresInSec <= 0 {
		expiresInSec = 3600
	}
	return time.Now().UnixMilli() + expiresInSec*1000
}
