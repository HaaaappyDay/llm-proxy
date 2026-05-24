package proxy

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/HaaapyDay/llm-proxy/internal/auth"
	"github.com/HaaapyDay/llm-proxy/internal/config"
	_ "modernc.org/sqlite"
)

// Doctor checks local configuration without making network requests.
func Doctor(cfg *config.Config) []string {
	var issues []string
	if cfg.DataDir == "" {
		return []string{"data directory not configured"}
	}
	if cfg.ListenHost != config.DefaultListenHost && cfg.ListenHost != "localhost" {
		issues = append(issues, "listen host is not localhost; do not expose this service to untrusted networks")
	}

	info, err := os.Stat(cfg.DataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return append(issues, "data directory does not exist yet (will be created on first login)")
		}
		return append(issues, "cannot stat data directory: "+err.Error())
	}
	if !info.IsDir() {
		return append(issues, "data path is not a directory")
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		issues = append(issues, fmt.Sprintf("data directory permissions are %03o; expected 700", perm))
	}

	keyCount, keys, keyIssues := inspectAPIKeys(filepath.Join(cfg.DataDir, "llm-proxy.db"))
	issues = append(issues, keyIssues...)

	accountCount := 0
	codexCount, codexAccounts, codexIssues := inspectCodexAuth(filepath.Join(cfg.DataDir, "codex_oauth_auth.json"))
	accountCount += codexCount
	issues = append(issues, codexIssues...)

	copilotCount, copilotAccounts, copilotIssues := inspectCopilotAuth(filepath.Join(cfg.DataDir, "copilot_auth.json"))
	accountCount += copilotCount
	issues = append(issues, copilotIssues...)
	issues = append(issues, inspectKeyAccountBindings(keys, codexAccounts, copilotAccounts)...)

	if keyCount == 0 {
		issues = append(issues, "no API keys found; run `llm-proxy login codex` or `llm-proxy login copilot`")
	}
	if accountCount == 0 {
		issues = append(issues, "no logged-in accounts found")
	}
	return issues
}

type keyBinding struct {
	ID        string
	Provider  string
	AccountID string
}

func inspectAPIKeys(path string) (int, []keyBinding, []string) {
	issues := inspectFilePerm(path, "llm-proxy.db")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil, []string{"llm-proxy.db does not exist"}
		}
		return 0, nil, []string{"cannot stat llm-proxy.db: " + err.Error()}
	}
	db, err := sql.Open("sqlite", path+"?_pragma=query_only(1)")
	if err != nil {
		return 0, nil, append(issues, "cannot open llm-proxy.db: "+err.Error())
	}
	defer db.Close()
	rows, err := db.Query(`
		select id, provider, account_id
		from api_keys
		where revoked_at is null
	`)
	if err != nil {
		return 0, nil, append(issues, "cannot inspect api_keys table: "+err.Error())
	}
	defer rows.Close()
	var keys []keyBinding
	for rows.Next() {
		var key keyBinding
		if err := rows.Scan(&key.ID, &key.Provider, &key.AccountID); err != nil {
			return 0, nil, append(issues, "cannot inspect api_keys row: "+err.Error())
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, append(issues, "cannot inspect api_keys rows: "+err.Error())
	}
	return len(keys), keys, issues
}

func inspectCodexAuth(path string) (int, map[string]bool, []string) {
	issues := inspectFilePerm(path, "codex_oauth_auth.json")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil, nil
		}
		return 0, nil, []string{"cannot stat codex_oauth_auth.json: " + err.Error()}
	}
	var store auth.CodexOAuthStore
	if err := readJSONFile(path, &store); err != nil {
		return 0, nil, append(issues, err.Error())
	}
	accounts := make(map[string]bool, len(store.Accounts))
	for id := range store.Accounts {
		accounts[id] = true
	}
	return len(store.Accounts), accounts, issues
}

func inspectCopilotAuth(path string) (int, map[string]bool, []string) {
	issues := inspectFilePerm(path, "copilot_auth.json")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil, nil
		}
		return 0, nil, []string{"cannot stat copilot_auth.json: " + err.Error()}
	}
	var store auth.CopilotAuthStore
	if err := readJSONFile(path, &store); err != nil {
		return 0, nil, append(issues, err.Error())
	}
	accounts := make(map[string]bool, len(store.Accounts))
	for id := range store.Accounts {
		accounts[id] = true
	}
	return len(store.Accounts), accounts, issues
}

func readJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", filepath.Base(path), err)
	}
	if len(data) == 0 {
		return fmt.Errorf("%s is empty", filepath.Base(path))
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("cannot parse %s: %w", filepath.Base(path), err)
	}
	return nil
}

func inspectFilePerm(path, name string) []string {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		return []string{fmt.Sprintf("%s permissions are %03o; expected 600", name, perm)}
	}
	return nil
}

func inspectKeyAccountBindings(keys []keyBinding, codexAccounts, copilotAccounts map[string]bool) []string {
	var issues []string
	for _, key := range keys {
		switch key.Provider {
		case auth.ProviderCodexOAuth:
			if key.AccountID != "" && !codexAccounts[key.AccountID] {
				issues = append(issues, fmt.Sprintf("API key %s references missing codex account %s", key.ID, key.AccountID))
			}
		case auth.ProviderGitHubCopilot:
			if key.AccountID != "" && !copilotAccounts[key.AccountID] {
				issues = append(issues, fmt.Sprintf("API key %s references missing copilot account %s", key.ID, key.AccountID))
			}
		default:
			issues = append(issues, fmt.Sprintf("API key %s uses unknown provider %s", key.ID, key.Provider))
		}
	}
	return issues
}
