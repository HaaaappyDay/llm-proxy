package auth

// Codex OAuth (aligned with cc-switch / official Codex CLI).
const (
	CodexClientID              = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexDeviceAuthUsercodeURL = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	CodexDeviceAuthTokenURL    = "https://auth.openai.com/api/accounts/deviceauth/token"
	CodexOAuthTokenURL         = "https://auth.openai.com/oauth/token"
	CodexOAuthAuthorizeURL     = "https://auth.openai.com/oauth/authorize"
	CodexDeviceVerificationURL = "https://auth.openai.com/codex/device"
	CodexDeviceRedirectURI     = "https://auth.openai.com/deviceauth/callback"
	CodexBrowserCallbackHost   = "127.0.0.1"
	CodexBrowserRedirectHost   = "localhost"
	CodexBrowserCallbackPath   = "/auth/callback"
	CodexBrowserOAuthScope     = "openid profile email"
	CodexBrowserLoginTimeout   = 10 * 60
	CodexUserAgent             = "llm-proxy-codex-oauth"
	CodexUpstreamResponsesURL  = "https://chatgpt.com/backend-api/codex/responses"
)

// Copilot OAuth (github.com only for MVP).
const (
	GitHubClientID           = "Iv1.b507a08c87ecfe98"
	GitHubDomain             = "github.com"
	CopilotUserAgent         = "llm-proxy-copilot"
	TokenRefreshBufferMs     = 60_000
	TokenRefreshBufferSec    = 60
	DeviceCodeDefaultExpires = 900
	PollSafetyMarginSec      = 3
)

func GitHubDeviceCodeURL(domain string) string {
	return "https://" + domain + "/login/device/code"
}

func GitHubOAuthTokenURL(domain string) string {
	return "https://" + domain + "/login/oauth/access_token"
}

func GitHubAPIBase(domain string) string {
	if domain == GitHubDomain {
		return "https://api.github.com"
	}
	return "https://" + domain + "/api/v3"
}

func CopilotTokenURL(domain string) string {
	return GitHubAPIBase(domain) + "/copilot_internal/v2/token"
}

func GitHubUserURL(domain string) string {
	return GitHubAPIBase(domain) + "/user"
}

func CopilotUserURL(domain string) string {
	return GitHubAPIBase(domain) + "/copilot_internal/user"
}

// Provider identifiers stored on API keys.
const (
	ProviderCodexOAuth    = "codex_oauth"
	ProviderGitHubCopilot = "github_copilot"
)
