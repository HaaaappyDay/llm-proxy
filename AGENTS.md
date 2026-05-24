# Repository Guidelines

## Project Structure & Module Organization

`llm-proxy` is a Go module for a local OAuth-to-API-key gateway. The CLI entry point lives in `cmd/llm-proxy/`. Core packages are under `internal/`: `app` wires the application, `auth` handles OAuth sessions and local API keys, `config` manages local configuration, `proxy` exposes HTTP handlers and forwarding, `server` defines routing, and `transform` converts request shapes. Documentation is in `docs/`; release and compatibility notes are in `CHANGELOG.md`, `README.md`, and `CONTRIBUTING.md`. Build outputs such as `bin/` and `dist/` are not source.

## Build, Test, and Development Commands

- `go test ./...` runs the full unit test suite. Tests must not require real Codex, Copilot, GitHub, or OpenAI credentials.
- `go vet ./...` runs Go static analysis.
- `go build -o bin/llm-proxy ./cmd/llm-proxy` builds the local CLI binary.
- `./bin/llm-proxy serve` starts the gateway on the default local address after login setup.
- `govulncheck ./...` and `goreleaser check` are maintainer release-readiness checks.

## Coding Style & Naming Conventions

Use standard Go formatting: run `gofmt` on edited Go files and keep imports organized by `goimports` or the Go toolchain. Package names are short lowercase words such as `auth`, `proxy`, and `transform`. Test files use `_test.go` suffixes and `Test...` functions. Prefer small package-local helpers, and keep security-sensitive behavior explicit.

## Testing Guidelines

Place tests next to the package they exercise, as in `internal/auth/apikey_test.go` or `internal/transform/transform_test.go`. Add focused unit tests for changes to auth/key handling, transforms, proxy forwarding, routing, and CLI-visible behavior. When provider protocol conversion changes, include fixtures or table tests showing supported shapes. Avoid network-dependent tests in the default suite.

## Commit & Pull Request Guidelines

Recent history uses concise conventional-style subjects such as `feat(proxy): add gin gateway...`, `docs: add readme...`, and `ci: run tests...`. Keep commits focused and use a clear prefix like `feat`, `fix`, `docs`, `test`, or `ci` when appropriate.

Pull requests should describe the behavior changed, list validation commands run, and link related issues when available. Update `README.md` or `docs/` for command, configuration, compatibility, security, or release-artifact changes. Call out any change affecting OAuth tokens, local `lpk_...` API keys, request headers, logging, error bodies, or persisted storage.

## Security & Configuration Tips

Do not commit runtime data from `~/.llm-proxy/`, OAuth refresh tokens, generated API keys, account identifiers, private prompts, or local environment files. Treat non-localhost serving as unsafe unless the network is fully trusted. Report vulnerabilities through `SECURITY.md`, not public issues.
