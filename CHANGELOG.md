# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

## v0.1.2 - 2026-05-26

### Added

- Added Codex Browser OAuth login for `llm-proxy login codex`.
- Added an interactive Codex login method prompt with Browser OAuth as the
  default choice and device-code login as the headless-friendly alternative.
- Added explicit non-interactive Codex login flags:
  - `llm-proxy login codex --browser`
  - `llm-proxy login codex --device-code`
- Added a localhost OAuth callback listener for Codex Browser OAuth. The CLI
  prints the OpenAI authorization URL, waits for the local callback, exchanges
  the authorization code with PKCE, persists the Codex OAuth account, and creates
  the usual local `lpk_...` API key.
- Added tests for Codex login method selection, Codex login flag validation,
  Browser OAuth callback success, state mismatch, OAuth error responses, missing
  callback codes, and token persistence.

### Changed

- `llm-proxy login codex` now asks whether to use Browser OAuth or device-code
  login when stdin is interactive.
- Browser OAuth is now the recommended interactive Codex login path.
- Codex device-code login remains available and continues to print the OpenAI
  device verification URL and user code.
- Non-interactive Codex login now requires `--browser` or `--device-code` so
  scripts do not block waiting for a prompt.
- Copilot login behavior is unchanged and continues to use the GitHub device
  flow. `llm-proxy login copilot --no-browser` remains supported.
- Updated README and OAuth documentation to describe the Codex Browser OAuth
  flow, explicit Codex login flags, callback behavior, and headless usage.

### Security

- Codex Browser OAuth uses PKCE and validates callback state before exchanging
  authorization codes.
- Browser callback responses and CLI output do not include OAuth access tokens,
  refresh tokens, authorization codes, account identifiers, or local API keys.
- The Codex browser callback listener is local-only and exists only for the
  duration of the login attempt.

### Compatibility

- This release fixes the `v0.1.1` release gap where the changelog mentioned
  Browser OAuth work, but the published release binary still contained only the
  device-code Codex login path.

## v0.1.1 - 2026-05-26

## v0.1.0 - 2026-05-24

### Added

- CLI-based OAuth device login for OpenAI Codex and GitHub Copilot accounts.
- Local `lpk_...` API keys backed by OAuth sessions.
- API key list, create, and delete commands.
- Anthropic Messages-compatible endpoint at `POST /v1/messages`.
- OpenAI Chat Completions-compatible endpoint at `POST /v1/chat/completions`.
- OpenAI Responses-compatible endpoint at `POST /v1/responses`.
- OpenAI-compatible model list endpoint at `GET /v1/models`.
- Request and response conversion across supported Anthropic, OpenAI Chat, and OpenAI Responses shapes.
- Streaming text and tool-call delta conversion where the selected upstream supports it.
- `llm-proxy doctor` for local configuration and storage checks.
- Version metadata for release builds.
- GitHub Actions CI and GoReleaser release automation.
- Open source governance files for contribution, security, conduct, and licensing.

### Security

- The server listens on `127.0.0.1:15721` by default.
- OAuth login and local API key management are CLI-only.
- Plaintext local API keys are printed once and are not persisted.
- Persisted local API keys are stored as SHA-256 hashes with metadata.
- Runtime data is stored under `~/.llm-proxy/` with restrictive file permissions.
- Debug logs avoid API keys, OAuth tokens, and full request bodies.

### Known Limitations

- Intended for local development use only.
- GitHub Enterprise Server is not currently supported.
- Provider availability and model access depend on the authenticated account.
- Provider API changes may require project updates.
