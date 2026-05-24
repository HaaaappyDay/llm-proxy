package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrAuthPending    = errors.New("authorization pending")
	ErrAccessDenied   = errors.New("access denied")
	ErrExpiredToken   = errors.New("device code expired")
	ErrRefreshInvalid = errors.New("refresh token invalid")
	ErrAccountMissing = errors.New("account not found")
)

type CodexOAuthManager struct {
	dataDir            string
	storagePath        string
	httpClient         *http.Client
	mu                 sync.RWMutex
	accounts           map[string]CodexAccountData
	defaultAccountID   string
	accessTokens       map[string]CachedAccessToken
	pendingDeviceCodes map[string]pendingDevice
	refreshLocks       map[string]*sync.Mutex
}

type pendingDevice struct {
	UserCode    string
	ExpiresAtMs int64
}

type openAIDeviceCodeResp struct {
	DeviceAuthID string          `json:"device_auth_id"`
	UserCode     string          `json:"user_code"`
	Interval     json.RawMessage `json:"interval"`
	ExpiresIn    *uint64         `json:"expires_in"`
}

type devicePollSuccess struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
}

type oauthTokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type idTokenClaims struct {
	ChatGPTAccountID string `json:"chatgpt_account_id"`
	Email            string `json:"email"`
	OpenAIAuth       *struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
	} `json:"https://api.openai.com/auth"`
	Organizations []struct {
		ID string `json:"id"`
	} `json:"organizations"`
}

func NewCodexOAuthManager(dataDir string) *CodexOAuthManager {
	m := &CodexOAuthManager{
		dataDir:            dataDir,
		storagePath:        filepath.Join(dataDir, "codex_oauth_auth.json"),
		httpClient:         &http.Client{Timeout: 60 * time.Second},
		accounts:           make(map[string]CodexAccountData),
		accessTokens:       make(map[string]CachedAccessToken),
		pendingDeviceCodes: make(map[string]pendingDevice),
		refreshLocks:       make(map[string]*sync.Mutex),
	}
	_ = ensureDataDir(dataDir)
	_ = m.loadFromDisk()
	return m
}

func (m *CodexOAuthManager) loadFromDisk() error {
	var store CodexOAuthStore
	if err := loadJSON(m.storagePath, &store); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if store.Accounts != nil {
		m.accounts = store.Accounts
	}
	m.defaultAccountID = store.DefaultAccountID
	return nil
}

func (m *CodexOAuthManager) saveToDisk() error {
	m.mu.RLock()
	store := CodexOAuthStore{
		Version:          1,
		Accounts:         m.accounts,
		DefaultAccountID: m.defaultAccountID,
	}
	m.mu.RUnlock()
	return atomicWriteJSON(m.storagePath, store)
}

func parseInterval(raw json.RawMessage) uint64 {
	if len(raw) == 0 {
		return 5
	}
	var n uint64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		return n
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if v, err := strconv.ParseUint(s, 10, 64); err == nil && v > 0 {
			return v
		}
	}
	return 5
}

func (m *CodexOAuthManager) StartDeviceFlow() (*DeviceCodeResponse, error) {
	body, _ := json.Marshal(map[string]string{"client_id": CodexClientID})
	req, err := http.NewRequest(http.MethodPost, CodexDeviceAuthUsercodeURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", CodexUserAgent)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed: %s - %s", resp.Status, string(b))
	}

	var device openAIDeviceCodeResp
	if err := json.NewDecoder(resp.Body).Decode(&device); err != nil {
		return nil, err
	}
	expiresIn := uint64(DeviceCodeDefaultExpires)
	if device.ExpiresIn != nil {
		expiresIn = *device.ExpiresIn
	}
	expiresAtMs := time.Now().UnixMilli() + int64(expiresIn)*1000

	m.mu.Lock()
	now := time.Now().UnixMilli()
	for k, v := range m.pendingDeviceCodes {
		if v.ExpiresAtMs <= now {
			delete(m.pendingDeviceCodes, k)
		}
	}
	m.pendingDeviceCodes[device.DeviceAuthID] = pendingDevice{
		UserCode:    device.UserCode,
		ExpiresAtMs: expiresAtMs,
	}
	m.mu.Unlock()

	return &DeviceCodeResponse{
		DeviceCode:      device.DeviceAuthID,
		UserCode:        device.UserCode,
		VerificationURI: CodexDeviceVerificationURL,
		ExpiresIn:       expiresIn,
		Interval:        parseInterval(device.Interval),
	}, nil
}

func (m *CodexOAuthManager) PollForToken(deviceCode string) (*Account, error) {
	m.mu.RLock()
	entry, ok := m.pendingDeviceCodes[deviceCode]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("device flow not found; restart login")
	}
	if entry.ExpiresAtMs <= time.Now().UnixMilli() {
		m.mu.Lock()
		delete(m.pendingDeviceCodes, deviceCode)
		m.mu.Unlock()
		return nil, ErrExpiredToken
	}

	body, _ := json.Marshal(map[string]string{
		"device_auth_id": deviceCode,
		"user_code":      entry.UserCode,
	})
	req, err := http.NewRequest(http.MethodPost, CodexDeviceAuthTokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", CodexUserAgent)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusNotFound:
		return nil, ErrAuthPending
	case http.StatusGone:
		return nil, ErrExpiredToken
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("poll failed: %s - %s", resp.Status, string(b))
	}

	var success devicePollSuccess
	if err := json.NewDecoder(resp.Body).Decode(&success); err != nil {
		return nil, err
	}

	tokens, err := m.exchangeCodeForTokens(success.AuthorizationCode, success.CodeVerifier)
	if err != nil {
		return nil, err
	}
	if tokens.RefreshToken == "" {
		return nil, errors.New("missing refresh_token")
	}

	accountID, email := extractIdentityFromTokens(tokens)
	if accountID == "" {
		return nil, errors.New("cannot extract account_id from token")
	}

	m.mu.Lock()
	delete(m.pendingDeviceCodes, deviceCode)
	m.accessTokens[accountID] = CachedAccessToken{
		Token:       tokens.AccessToken,
		ExpiresAtMs: computeExpiresAtMs(tokens.ExpiresIn),
	}
	m.mu.Unlock()

	return m.addAccount(accountID, tokens.RefreshToken, email)
}

func (m *CodexOAuthManager) exchangeCodeForTokens(code, verifier string) (*oauthTokenResp, error) {
	form := fmt.Sprintf(
		"grant_type=authorization_code&code=%s&redirect_uri=%s&client_id=%s&code_verifier=%s",
		code, CodexDeviceRedirectURI, CodexClientID, verifier,
	)
	req, err := http.NewRequest(http.MethodPost, CodexOAuthTokenURL, strings.NewReader(form))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", CodexUserAgent)
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s - %s", resp.Status, string(b))
	}
	var out oauthTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (m *CodexOAuthManager) refreshWithToken(refreshToken string) (*oauthTokenResp, error) {
	form := fmt.Sprintf(
		"grant_type=refresh_token&refresh_token=%s&client_id=%s&scope=%s",
		refreshToken, CodexClientID, "openid profile email",
	)
	req, err := http.NewRequest(http.MethodPost, CodexOAuthTokenURL, strings.NewReader(form))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", CodexUserAgent)
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrRefreshInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("refresh failed: %s - %s", resp.Status, string(b))
	}
	var out oauthTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func extractIdentityFromTokens(tokens *oauthTokenResp) (accountID, email string) {
	if tokens.IDToken != "" {
		if id, em := parseIDToken(tokens.IDToken); id != "" {
			return id, em
		}
	}
	if tokens.AccessToken != "" {
		if id, em := parseIDToken(tokens.AccessToken); id != "" {
			return id, em
		}
	}
	return "", ""
}

func parseIDToken(token string) (accountID, email string) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ""
	}
	var claims idTokenClaims
	if json.Unmarshal(payload, &claims) != nil {
		return "", ""
	}
	email = claims.Email
	if claims.ChatGPTAccountID != "" {
		return claims.ChatGPTAccountID, email
	}
	if claims.OpenAIAuth != nil && claims.OpenAIAuth.ChatGPTAccountID != "" {
		return claims.OpenAIAuth.ChatGPTAccountID, email
	}
	if len(claims.Organizations) > 0 && claims.Organizations[0].ID != "" {
		return claims.Organizations[0].ID, email
	}
	return "", email
}

func (m *CodexOAuthManager) addAccount(accountID, refreshToken, email string) (*Account, error) {
	m.mu.Lock()
	login := email
	if login == "" {
		login = "ChatGPT (" + accountID + ")"
	}
	m.accounts[accountID] = CodexAccountData{
		AccountID:       accountID,
		Email:           email,
		RefreshToken:    refreshToken,
		AuthenticatedAt: time.Now().Unix(),
	}
	if m.defaultAccountID == "" {
		m.defaultAccountID = accountID
	}
	m.mu.Unlock()
	if err := m.saveToDisk(); err != nil {
		return nil, err
	}
	return &Account{
		ID:              accountID,
		Login:           login,
		AuthenticatedAt: time.Now().Unix(),
		Provider:        ProviderCodexOAuth,
	}, nil
}

func (m *CodexOAuthManager) getRefreshLock(accountID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.refreshLocks[accountID] == nil {
		m.refreshLocks[accountID] = &sync.Mutex{}
	}
	return m.refreshLocks[accountID]
}

func (m *CodexOAuthManager) GetValidToken(accountID string) (string, error) {
	if accountID == "" {
		m.mu.RLock()
		accountID = m.defaultAccountID
		m.mu.RUnlock()
	}
	if accountID == "" {
		return "", ErrAccountMissing
	}

	m.mu.RLock()
	if cached, ok := m.accessTokens[accountID]; ok && !cached.IsExpiringSoon() {
		tok := cached.Token
		m.mu.RUnlock()
		return tok, nil
	}
	m.mu.RUnlock()

	lock := m.getRefreshLock(accountID)
	lock.Lock()
	defer lock.Unlock()

	m.mu.RLock()
	if cached, ok := m.accessTokens[accountID]; ok && !cached.IsExpiringSoon() {
		tok := cached.Token
		m.mu.RUnlock()
		return tok, nil
	}
	acc, ok := m.accounts[accountID]
	m.mu.RUnlock()
	if !ok {
		return "", ErrAccountMissing
	}

	tokens, err := m.refreshWithToken(acc.RefreshToken)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.accessTokens[accountID] = CachedAccessToken{
		Token:       tokens.AccessToken,
		ExpiresAtMs: computeExpiresAtMs(tokens.ExpiresIn),
	}
	if tokens.RefreshToken != "" {
		acc.RefreshToken = tokens.RefreshToken
		m.accounts[accountID] = acc
	}
	m.mu.Unlock()
	_ = m.saveToDisk()
	return tokens.AccessToken, nil
}

func (m *CodexOAuthManager) ListAccounts() []Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Account, 0, len(m.accounts))
	for id, a := range m.accounts {
		login := a.Email
		if login == "" {
			login = "ChatGPT (" + id + ")"
		}
		out = append(out, Account{
			ID:              id,
			Login:           login,
			AuthenticatedAt: a.AuthenticatedAt,
			Provider:        ProviderCodexOAuth,
		})
	}
	return out
}

func (m *CodexOAuthManager) DefaultAccountID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultAccountID
}
