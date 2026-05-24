# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

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
