package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const APIKeyPrefix = "lpk_"

type APIKeyRecord struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Provider   string `json:"provider"`
	AccountID  string `json:"account_id"`
	CreatedAt  int64  `json:"created_at"`
	KeyPreview string `json:"key_preview,omitempty"`
	RevokedAt  *int64 `json:"revoked_at,omitempty"`
}

type apiKeyStore struct {
	Keys map[string]APIKeyRecord `json:"keys"` // key = sha256 hex
}

type APIKeyManager struct {
	db         *sql.DB
	dbPath     string
	legacyPath string
	initErr    error
}

func NewAPIKeyManager(dataDir string) *APIKeyManager {
	m := &APIKeyManager{
		dbPath:     filepath.Join(dataDir, "llm-proxy.db"),
		legacyPath: filepath.Join(dataDir, "api_keys.json"),
	}
	m.initErr = m.init(dataDir)
	return m
}

func (m *APIKeyManager) init(dataDir string) error {
	if err := ensureDataDir(dataDir); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", m.dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	m.db = db
	if err := m.ensureSchema(); err != nil {
		return err
	}
	if err := m.migrateLegacyJSON(); err != nil {
		return err
	}
	return chmodIfExists(m.dbPath, 0o600)
}

func hashAPIKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func randomSuffix(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
	if err := m.ready(); err != nil {
		return nil, err
	}
	if in.Provider != ProviderCodexOAuth && in.Provider != ProviderGitHubCopilot {
		return nil, fmt.Errorf("unsupported provider: %s", in.Provider)
	}
	suffix, err := randomSuffix(4)
	if err != nil {
		return nil, err
	}
	plain := APIKeyPrefix + uuid.New().String() + suffix
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
	if _, err := m.db.Exec(`
		insert into api_keys (id, key_hash, key_preview, label, provider, account_id, created_at, revoked_at)
		values (?, ?, ?, ?, ?, ?, ?, null)
	`, rec.ID, hash, rec.KeyPreview, rec.Label, rec.Provider, rec.AccountID, rec.CreatedAt); err != nil {
		return nil, err
	}
	return &CreateKeyResult{Plaintext: plain, Record: rec}, nil
}

func (m *APIKeyManager) Resolve(plain string) (*APIKeyRecord, error) {
	if err := m.ready(); err != nil {
		return nil, err
	}
	if plain == "" {
		return nil, errors.New("missing api key")
	}
	hash := hashAPIKey(plain)
	rec, err := m.scanRecord(m.db.QueryRow(`
		select id, key_preview, label, provider, account_id, created_at, revoked_at
		from api_keys
		where key_hash = ? and revoked_at is null
	`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("invalid api key")
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (m *APIKeyManager) List() []APIKeyRecord {
	out, _ := m.ListActive()
	return out
}

func (m *APIKeyManager) ListActive() ([]APIKeyRecord, error) {
	if err := m.ready(); err != nil {
		return nil, err
	}
	rows, err := m.db.Query(`
		select id, key_preview, label, provider, account_id, created_at, revoked_at
		from api_keys
		where revoked_at is null
		order by created_at desc, id asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKeyRecord
	for rows.Next() {
		rec, err := scanAPIKeyRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (m *APIKeyManager) Delete(id string) error {
	if err := m.ready(); err != nil {
		return err
	}
	res, err := m.db.Exec(`update api_keys set revoked_at = ? where id = ? and revoked_at is null`, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("api key not found")
	}
	return nil
}

func (m *APIKeyManager) MarshalList() ([]byte, error) {
	list, err := m.ListActive()
	if err != nil {
		return nil, err
	}
	return json.Marshal(list)
}

func (m *APIKeyManager) ready() error {
	if m.initErr != nil {
		return m.initErr
	}
	if m.db == nil {
		return errors.New("api key database is not initialized")
	}
	return nil
}

func (m *APIKeyManager) ensureSchema() error {
	_, err := m.db.Exec(`
		create table if not exists api_keys (
			id text primary key,
			key_hash text not null unique,
			key_preview text not null,
			label text not null,
			provider text not null,
			account_id text not null,
			created_at integer not null,
			revoked_at integer null
		);
		create index if not exists idx_api_keys_active on api_keys (revoked_at, created_at);
	`)
	return err
}

func (m *APIKeyManager) migrateLegacyJSON() error {
	var store apiKeyStore
	if err := loadJSON(m.legacyPath, &store); err != nil {
		return err
	}
	if len(store.Keys) == 0 {
		return nil
	}
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		insert or ignore into api_keys (id, key_hash, key_preview, label, provider, account_id, created_at, revoked_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for hash, rec := range store.Keys {
		if rec.ID == "" {
			rec.ID = uuid.New().String()
		}
		if rec.Label == "" {
			rec.Label = "default"
		}
		_, err := stmt.Exec(rec.ID, hash, rec.KeyPreview, rec.Label, rec.Provider, rec.AccountID, rec.CreatedAt, rec.RevokedAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

type recordScanner interface {
	Scan(dest ...any) error
}

func (m *APIKeyManager) scanRecord(row recordScanner) (APIKeyRecord, error) {
	return scanAPIKeyRecord(row)
}

func scanAPIKeyRecord(row recordScanner) (APIKeyRecord, error) {
	var rec APIKeyRecord
	var revokedAt sql.NullInt64
	err := row.Scan(&rec.ID, &rec.KeyPreview, &rec.Label, &rec.Provider, &rec.AccountID, &rec.CreatedAt, &revokedAt)
	if err != nil {
		return rec, err
	}
	if revokedAt.Valid {
		rec.RevokedAt = &revokedAt.Int64
	}
	return rec, nil
}
