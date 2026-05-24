package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CopilotAuthManager struct {
	dataDir          string
	storagePath      string
	httpClient       *http.Client
	mu               sync.RWMutex
	accounts         map[string]CopilotAccountData
	defaultAccountID string
	copilotTokens    map[string]CopilotToken
	apiEndpoints     map[string]string
	refreshLocks     map[string]*sync.Mutex
	pendingFlows     map[string]pendingCopilotFlow
}

type pendingCopilotFlow struct {
	Interval  uint64
	ExpiresAt int64
}

type githubDeviceResp struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       uint64 `json:"expires_in"`
	Interval        uint64 `json:"interval"`
}

type githubOAuthResp struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type copilotTokenAPIResp struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

type copilotUserResp struct {
	Endpoints struct {
		API string `json:"api"`
	} `json:"endpoints"`
}

func NewCopilotAuthManager(dataDir string) *CopilotAuthManager {
	m := &CopilotAuthManager{
		dataDir:       dataDir,
		storagePath:   filepath.Join(dataDir, "copilot_auth.json"),
		httpClient:    &http.Client{Timeout: 60 * time.Second},
		accounts:      make(map[string]CopilotAccountData),
		copilotTokens: make(map[string]CopilotToken),
		apiEndpoints:  make(map[string]string),
		refreshLocks:  make(map[string]*sync.Mutex),
		pendingFlows:  make(map[string]pendingCopilotFlow),
	}
	_ = ensureDataDir(dataDir)
	_ = m.loadFromDisk()
	return m
}

func accountKey(domain string, userID uint64) string {
	if domain == GitHubDomain {
		return strconv.FormatUint(userID, 10)
	}
	return domain + ":" + strconv.FormatUint(userID, 10)
}

func (m *CopilotAuthManager) loadFromDisk() error {
	var store CopilotAuthStore
	if err := loadJSON(m.storagePath, &store); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if store.Version == 0 {
		store.Version = 3
	}
	if store.Accounts != nil {
		m.accounts = store.Accounts
	}
	m.defaultAccountID = store.DefaultAccountID
	return nil
}

func (m *CopilotAuthManager) saveToDisk() error {
	m.mu.RLock()
	store := CopilotAuthStore{
		Version:          3,
		Accounts:         m.accounts,
		DefaultAccountID: m.defaultAccountID,
	}
	m.mu.RUnlock()
	return atomicWriteJSON(m.storagePath, store)
}

func (m *CopilotAuthManager) StartDeviceFlow() (*DeviceCodeResponse, error) {
	domain := GitHubDomain
	form := url.Values{}
	form.Set("client_id", GitHubClientID)
	form.Set("scope", "read:user")

	req, err := http.NewRequest(http.MethodPost, GitHubDeviceCodeURL(domain), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", CopilotUserAgent)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code failed: %s - %s", resp.Status, string(b))
	}
	var device githubDeviceResp
	if err := json.NewDecoder(resp.Body).Decode(&device); err != nil {
		return nil, err
	}
	interval := device.Interval
	if interval == 0 {
		interval = 5
	}
	expiresAt := time.Now().Unix() + int64(device.ExpiresIn)

	m.mu.Lock()
	m.pendingFlows[device.DeviceCode] = pendingCopilotFlow{
		Interval:  interval,
		ExpiresAt: expiresAt,
	}
	m.mu.Unlock()

	return &DeviceCodeResponse{
		DeviceCode:      device.DeviceCode,
		UserCode:        device.UserCode,
		VerificationURI: device.VerificationURI,
		ExpiresIn:       device.ExpiresIn,
		Interval:        interval,
	}, nil
}

func (m *CopilotAuthManager) PollForToken(deviceCode string) (*Account, error) {
	m.mu.RLock()
	flow, ok := m.pendingFlows[deviceCode]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("device flow not found; restart login")
	}
	if time.Now().Unix() >= flow.ExpiresAt {
		return nil, ErrExpiredToken
	}

	form := url.Values{}
	form.Set("client_id", GitHubClientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequest(http.MethodPost, GitHubOAuthTokenURL(GitHubDomain), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", CopilotUserAgent)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var gh githubOAuthResp
	_ = json.NewDecoder(resp.Body).Decode(&gh)

	if gh.Error == "authorization_pending" {
		return nil, ErrAuthPending
	}
	if gh.Error == "access_denied" {
		return nil, ErrAccessDenied
	}
	if gh.Error == "expired_token" {
		return nil, ErrExpiredToken
	}
	if gh.AccessToken == "" {
		return nil, ErrAuthPending
	}

	user, err := m.fetchGitHubUser(gh.AccessToken, GitHubDomain)
	if err != nil {
		return nil, err
	}

	copilotTok, err := m.fetchCopilotToken(gh.AccessToken, GitHubDomain)
	if err != nil {
		return nil, err
	}

	accID := accountKey(GitHubDomain, user.ID)
	m.mu.Lock()
	delete(m.pendingFlows, deviceCode)
	m.accounts[accID] = CopilotAccountData{
		GitHubToken:     gh.AccessToken,
		User:            *user,
		AuthenticatedAt: time.Now().Unix(),
		GitHubDomain:    GitHubDomain,
	}
	m.copilotTokens[accID] = *copilotTok
	if m.defaultAccountID == "" {
		m.defaultAccountID = accID
	}
	m.mu.Unlock()

	_ = m.refreshAPIEndpoint(accID, gh.AccessToken)

	if err := m.saveToDisk(); err != nil {
		return nil, err
	}
	return &Account{
		ID:              accID,
		Login:           user.Login,
		AvatarURL:       user.AvatarURL,
		AuthenticatedAt: time.Now().Unix(),
		GitHubDomain:    GitHubDomain,
		Provider:        ProviderGitHubCopilot,
	}, nil
}

func (m *CopilotAuthManager) fetchGitHubUser(token, domain string) (*GitHubUser, error) {
	req, err := http.NewRequest(http.MethodGet, GitHubUserURL(domain), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", CopilotUserAgent)
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github user failed: %s - %s", resp.Status, string(b))
	}
	var user GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (m *CopilotAuthManager) fetchCopilotToken(githubToken, domain string) (*CopilotToken, error) {
	req, err := http.NewRequest(http.MethodGet, CopilotTokenURL(domain), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", CopilotUserAgent)
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("copilot token failed: %s - %s", resp.Status, string(b))
	}
	var out copilotTokenAPIResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &CopilotToken{Token: out.Token, ExpiresAt: out.ExpiresAt}, nil
}

func (m *CopilotAuthManager) refreshAPIEndpoint(accountID, githubToken string) error {
	req, err := http.NewRequest(http.MethodGet, CopilotUserURL(GitHubDomain), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+githubToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", CopilotUserAgent)
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("copilot user info failed: %s", resp.Status)
	}
	var info copilotUserResp
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return err
	}
	if info.Endpoints.API == "" {
		return errors.New("missing copilot api endpoint")
	}
	m.mu.Lock()
	m.apiEndpoints[accountID] = strings.TrimSuffix(info.Endpoints.API, "/")
	m.mu.Unlock()
	return nil
}

func (m *CopilotAuthManager) getRefreshLock(accountID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.refreshLocks[accountID] == nil {
		m.refreshLocks[accountID] = &sync.Mutex{}
	}
	return m.refreshLocks[accountID]
}

func (m *CopilotAuthManager) GetValidCopilotToken(accountID string) (string, error) {
	if accountID == "" {
		m.mu.RLock()
		accountID = m.defaultAccountID
		m.mu.RUnlock()
	}
	if accountID == "" {
		return "", ErrAccountMissing
	}

	m.mu.RLock()
	if tok, ok := m.copilotTokens[accountID]; ok && !tok.IsExpiringSoon() {
		t := tok.Token
		m.mu.RUnlock()
		return t, nil
	}
	m.mu.RUnlock()

	lock := m.getRefreshLock(accountID)
	lock.Lock()
	defer lock.Unlock()

	m.mu.RLock()
	acc, ok := m.accounts[accountID]
	if !ok {
		m.mu.RUnlock()
		return "", ErrAccountMissing
	}
	githubToken := acc.GitHubToken
	m.mu.RUnlock()

	copilotTok, err := m.fetchCopilotToken(githubToken, acc.GitHubDomain)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.copilotTokens[accountID] = *copilotTok
	m.mu.Unlock()
	return copilotTok.Token, nil
}

func (m *CopilotAuthManager) GetAPIEndpoint(accountID string) (string, error) {
	m.mu.RLock()
	ep, ok := m.apiEndpoints[accountID]
	acc, hasAcc := m.accounts[accountID]
	m.mu.RUnlock()
	if ok && ep != "" {
		return ep, nil
	}
	if !hasAcc {
		return "", ErrAccountMissing
	}
	if err := m.refreshAPIEndpoint(accountID, acc.GitHubToken); err != nil {
		return "", err
	}
	m.mu.RLock()
	ep = m.apiEndpoints[accountID]
	m.mu.RUnlock()
	if ep == "" {
		return "", errors.New("copilot api endpoint unavailable")
	}
	return ep, nil
}

func (m *CopilotAuthManager) ListAccounts() []Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Account, 0, len(m.accounts))
	for id, a := range m.accounts {
		out = append(out, Account{
			ID:              id,
			Login:           a.User.Login,
			AvatarURL:       a.User.AvatarURL,
			AuthenticatedAt: a.AuthenticatedAt,
			GitHubDomain:    a.GitHubDomain,
			Provider:        ProviderGitHubCopilot,
		})
	}
	return out
}

func (m *CopilotAuthManager) DefaultAccountID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultAccountID
}

// CopilotHeaders returns headers required for Copilot API requests.
func CopilotHeaders(token string) http.Header {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+token)
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", CopilotUserAgent)
	h.Set("Editor-Version", "vscode/1.95.0")
	h.Set("Editor-Plugin-Version", "copilot-chat/0.22.0")
	h.Set("Copilot-Integration-Id", "vscode-chat")
	return h
}

// CodexHeaders returns headers for Codex upstream.
func CodexHeaders(accessToken, accountID string) http.Header {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+accessToken)
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", CodexUserAgent)
	h.Set("Originator", "llm-proxy")
	if accountID != "" {
		h.Set("Chatgpt-Account-Id", accountID)
	}
	return h
}

func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}
