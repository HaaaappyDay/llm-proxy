package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

const APIKeyPrefix = "lpk_"

type APIKeyRecord struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Provider   string `json:"provider"`
	AccountID  string `json:"account_id"`
	CreatedAt  int64  `json:"created_at"`
	KeyPreview string `json:"key_preview,omitempty"`
}

type apiKeyStore struct {
	Keys map[string]APIKeyRecord `json:"keys"` // key = sha256 hex
}

type APIKeyManager struct {
	path string
	mu   sync.RWMutex
	keys map[string]APIKeyRecord
}

func NewAPIKeyManager(dataDir string) *APIKeyManager {
	m := &APIKeyManager{
		path: filepath.Join(dataDir, "api_keys.json"),
		keys: make(map[string]APIKeyRecord),
	}
	_ = ensureDataDir(dataDir)
	_ = m.load()
	return m
}

func (m *APIKeyManager) load() error {
	var store apiKeyStore
	if err := loadJSON(m.path, &store); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if store.Keys != nil {
		m.keys = store.Keys
	}
	return nil
}

func (m *APIKeyManager) save() error {
	m.mu.RLock()
	store := apiKeyStore{Keys: m.keys}
	m.mu.RUnlock()
	return atomicWriteJSON(m.path, store)
}

func hashAPIKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func randomSuffix(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type CreateKeyInput struct {
	Label     string
	Provider  string
	AccountID string
}

type CreateKeyResult struct {
	Plaintext string
	Record    APIKeyRecord
}

func (m *APIKeyManager) Create(in CreateKeyInput) (*CreateKeyResult, error) {
	if in.Provider != ProviderCodexOAuth && in.Provider != ProviderGitHubCopilot {
		return nil, fmt.Errorf("unsupported provider: %s", in.Provider)
	}
	plain := APIKeyPrefix + uuid.New().String() + randomSuffix(4)
	hash := hashAPIKey(plain)
	preview := plain[:12] + "..." + plain[len(plain)-4:]
	rec := APIKeyRecord{
		ID:         uuid.New().String(),
		Label:      in.Label,
		Provider:   in.Provider,
		AccountID:  in.AccountID,
		CreatedAt:  time.Now().Unix(),
		KeyPreview: preview,
	}
	m.mu.Lock()
	m.keys[hash] = rec
	m.mu.Unlock()
	if err := m.save(); err != nil {
		return nil, err
	}
	return &CreateKeyResult{Plaintext: plain, Record: rec}, nil
}

func (m *APIKeyManager) Resolve(plain string) (*APIKeyRecord, error) {
	if plain == "" {
		return nil, errors.New("missing api key")
	}
	hash := hashAPIKey(plain)
	m.mu.RLock()
	rec, ok := m.keys[hash]
	m.mu.RUnlock()
	if !ok {
		return nil, errors.New("invalid api key")
	}
	return &rec, nil
}

func (m *APIKeyManager) List() []APIKeyRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]APIKeyRecord, 0, len(m.keys))
	for _, rec := range m.keys {
		out = append(out, rec)
	}
	return out
}

func (m *APIKeyManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for hash, rec := range m.keys {
		if rec.ID == id {
			delete(m.keys, hash)
			return m.save()
		}
	}
	return errors.New("api key not found")
}

func (m *APIKeyManager) MarshalList() ([]byte, error) {
	list := m.List()
	return json.Marshal(list)
}
