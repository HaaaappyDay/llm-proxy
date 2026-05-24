package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAPIKeyManagerSQLiteLifecycle(t *testing.T) {
	m := NewAPIKeyManager(t.TempDir())

	result, err := m.Create(CreateKeyInput{
		Label:     "test",
		Provider:  ProviderCodexOAuth,
		AccountID: "acct_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	rec, err := m.Resolve(result.Plaintext)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rec.ID != result.Record.ID {
		t.Fatalf("resolved id = %s, want %s", rec.ID, result.Record.ID)
	}

	keys, err := m.ListActive()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("len(keys) = %d, want 1", len(keys))
	}

	if err := m.Delete(result.Record.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := m.Resolve(result.Plaintext); err == nil {
		t.Fatal("revoked key resolved successfully")
	}
	keys, err = m.ListActive()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("len(keys) after delete = %d, want 0", len(keys))
	}
}

func TestAPIKeyManagerMigratesLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	plain := APIKeyPrefix + "legacy"
	hash := hashAPIKey(plain)
	legacy := apiKeyStore{
		Keys: map[string]APIKeyRecord{
			hash: {
				ID:         "key_legacy",
				Label:      "legacy",
				Provider:   ProviderGitHubCopilot,
				AccountID:  "acct_legacy",
				CreatedAt:  123,
				KeyPreview: "lpk_legacy",
			},
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api_keys.json"), data, 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	m := NewAPIKeyManager(dir)
	rec, err := m.Resolve(plain)
	if err != nil {
		t.Fatalf("resolve migrated key: %v", err)
	}
	if rec.ID != "key_legacy" {
		t.Fatalf("migrated id = %s", rec.ID)
	}
	if _, err := os.Stat(filepath.Join(dir, "api_keys.json")); err != nil {
		t.Fatalf("legacy file should be preserved: %v", err)
	}
}
