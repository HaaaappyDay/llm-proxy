# API Key SQLite CLI Design

## Scope

Move local API key metadata from `api_keys.json` to SQLite and expose local CLI commands for listing, creating, and revoking keys. This does not add an HTTP key management surface.

## Storage

Runtime data remains under `~/.llm-proxy` by default. API key records are stored in `llm-proxy.db` with restrictive filesystem permissions inherited from the existing data directory setup.

The `api_keys` table stores:

- `id`: stable key identifier.
- `key_hash`: SHA-256 hash of the plaintext key.
- `key_preview`: shortened display value.
- `label`: user-facing label.
- `provider`: `codex_oauth` or `github_copilot`.
- `account_id`: provider account identifier.
- `created_at`: Unix timestamp.
- `revoked_at`: nullable Unix timestamp for soft deletion.

Plaintext API keys are never stored.

## Migration

On startup, if `api_keys.json` exists, records are imported into SQLite. Existing hashed keys are preserved. The JSON file is left in place after migration so migration is non-destructive.

## CLI

Add:

```bash
llm-proxy keys list
llm-proxy keys create codex|copilot [--label NAME]
llm-proxy keys delete KEY_ID
```

`login codex|copilot` continues to create a default API key. `keys create` requires an existing logged-in account for the selected provider and prints the plaintext key once. `keys list` never prints plaintext keys. `keys delete` sets `revoked_at`; authentication ignores revoked records.

## Error Handling

SQLite initialization and migration failures are returned to the caller. Invalid providers, missing accounts, and unknown key IDs are reported as CLI errors. `doctor` checks the SQLite database and reports missing keys or unreadable storage.

## Tests

Unit tests should cover SQLite create, resolve, list, revoke, JSON migration, and route authentication using revoked keys.
