# Maintenance Notes

These notes define the expected maintenance posture for `llm-proxy`.

## Project Boundary

`llm-proxy` is a local development gateway. It is not intended to be a public
service, shared hosted API, account pool, or resale layer.

The default safe operating mode is binding to:

```text
127.0.0.1:15721
```

## Provider Drift

Codex, GitHub Copilot, OpenAI-compatible APIs, and model behavior can change
without notice. Compatibility claims should stay narrow and be backed by tests,
fixtures, or a minimized reproducible request.

When provider behavior changes:

- preserve existing tests where behavior is still valid
- add regression coverage for new request or response shapes
- document unsupported conversions instead of silently dropping fields
- prefer structured `400` responses for unsupported local conversions

## Compatibility Changes

Changes to request or response conversion should include tests for:

- source request parsing
- target request generation
- response conversion
- streaming behavior when applicable
- unsupported feature errors when data cannot be represented safely

Update `docs/compatibility.md` when user-visible compatibility changes.

## Security-Sensitive Changes

Changes in these areas require explicit review for sensitive data exposure:

- OAuth token handling
- local API key creation, hashing, lookup, and revocation
- runtime data storage
- request and response logging
- upstream error handling
- HTTP headers
- debug output
- network binding behavior

Public issue and PR content must not include OAuth tokens, generated `lpk_...`
keys, account identifiers, private prompts, file IDs, or full sensitive payloads.

## Release Discipline

Follow `docs/release.md` before publishing a tag. A release should not be tagged
until tests, vet, vulnerability checks, and GoReleaser validation have passed in
at least one trusted environment.

## Current Non-Goals

- GitHub Enterprise Server support.
- Public internet deployment.
- Docker images.
- Homebrew or Scoop packages.
- Artifact signing or SBOM generation.
- Broad compatibility claims for unverified clients.
