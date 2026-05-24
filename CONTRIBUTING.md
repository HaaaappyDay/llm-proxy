# Contributing

Thanks for helping improve `llm-proxy`.

## Development Setup

Requirements:

- Go 1.25 or newer
- Network access only when exercising OAuth flows or real upstream proxy calls

Build and test:

```bash
go test ./...
go vet ./...
go build -o bin/llm-proxy ./cmd/llm-proxy
```

Release-readiness checks used by maintainers:

```bash
govulncheck ./...
goreleaser check
goreleaser release --snapshot --clean
```

The test suite must not require real Codex, Copilot, or GitHub credentials.

## Pull Requests

- Keep changes focused on one behavior or maintenance concern.
- Add or update tests for user-visible behavior, auth/key handling, request transforms, and proxy error handling.
- Update README or `docs/` when changing commands, configuration, compatibility, security behavior, or release artifacts.
- Do not commit runtime data from `~/.llm-proxy/`, OAuth tokens, generated API keys, or local environment files.
- Call out any change that affects OAuth tokens, local API keys, request headers, logging, error bodies, or persisted storage.

## Documentation

Before changing non-trivial behavior, skim the relevant docs so the
change can land with matching documentation in the same PR:

- [docs/architecture.md](docs/architecture.md) for an orientation map
  of packages and the request lifecycle.
- [docs/oauth.md](docs/oauth.md) when changing anything under
  `internal/auth/`, on-disk auth files, or the `login` CLI surface.
- [docs/errors.md](docs/errors.md) when changing handler error shapes,
  HTTP status codes, or upstream-error envelopes.
- [docs/roadmap.md](docs/roadmap.md) for direction and non-goals; update
  it when you finish a planned item or discover a new known issue.

Update the matching doc in the same PR. Documentation drift is
treated as a bug.

## Compatibility

Provider APIs can change without notice. If a contribution changes protocol conversion behavior, include fixtures or unit tests that demonstrate the supported request and response shape.

## Security

Do not open public issues for vulnerabilities. Follow `SECURITY.md`.

Do not paste OAuth refresh tokens, generated `lpk_...` keys, account identifiers, private prompts, or full sensitive payloads into public issues or pull requests.
