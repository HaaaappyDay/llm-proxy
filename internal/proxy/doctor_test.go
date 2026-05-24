package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HaaapyDay/llm-proxy/internal/auth"
	"github.com/HaaapyDay/llm-proxy/internal/config"
)

func TestDoctorReportsFilePermissionsAndMissingAccountBindings(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}

	apiKeys := auth.NewAPIKeyManager(dir)
	_, err := apiKeys.Create(auth.CreateKeyInput{
		Label:     "orphaned",
		Provider:  auth.ProviderCodexOAuth,
		AccountID: "acct_missing",
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	authPath := filepath.Join(dir, "codex_oauth_auth.json")
	data, err := json.Marshal(auth.CodexOAuthStore{
		Version:  1,
		Accounts: map[string]auth.CodexAccountData{},
	})
	if err != nil {
		t.Fatalf("marshal auth store: %v", err)
	}
	if err := os.WriteFile(authPath, data, 0o644); err != nil {
		t.Fatalf("write auth store: %v", err)
	}
	if err := os.Chmod(authPath, 0o644); err != nil {
		t.Fatalf("chmod auth store: %v", err)
	}

	issues := Doctor(&config.Config{
		ListenHost: config.DefaultListenHost,
		ListenPort: config.DefaultListenPort,
		DataDir:    dir,
	})
	joined := strings.Join(issues, "\n")
	if !strings.Contains(joined, "codex_oauth_auth.json permissions are 644; expected 600") {
		t.Fatalf("missing permission issue: %v", issues)
	}
	if !strings.Contains(joined, "references missing codex account acct_missing") {
		t.Fatalf("missing account binding issue: %v", issues)
	}
}
