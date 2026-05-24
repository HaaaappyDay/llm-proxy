# GitHub Readiness Design

## Goal

Prepare `llm-proxy` for public GitHub publication as a credible open-source project.
The work should improve repository identity, installation guidance, release process,
security posture, compatibility reporting, and maintainer workflow without changing
the core proxy behavior.

The target repository is:

```text
git@github.com:HaaapyDay/llm-proxy.git
https://github.com/HaaapyDay/llm-proxy
```

## Recommended Approach

Use a release-readiness pass rather than a minimal cleanup or full productization.

This keeps the scope focused on the public repository surface:

- documentation
- release process
- GitHub templates
- CI checks
- compatibility and maintenance guidance
- repository path consistency

The first public release should be treated as `v0.1.0`.

## Repository Identity

All public package, clone, release, and install references should use the final
repository path:

```text
github.com/HaaapyDay/llm-proxy
```

Affected areas:

- `go.mod`
- Go import paths under `cmd/` and `internal/`
- `README.md`
- `README.zh-CN.md`
- release download links
- source build examples
- `go install` examples

The SSH remote should be configured as `origin`:

```text
git@github.com:HaaapyDay/llm-proxy.git
```

## README Design

The README files should make the project understandable and safe to evaluate
within the first screen.

Both English and Chinese READMEs should include:

- concise project positioning
- prominent safety and compliance notice
- feature list
- installation options
- quick start
- client configuration examples
- API reference
- compatibility summary with a link to detailed docs
- data directory and local security model
- development commands
- limitations

The safety notice should make these boundaries explicit:

- `llm-proxy` is a local development gateway.
- It does not bypass provider limits, access controls, account restrictions, or terms.
- It should not be exposed to the public internet.
- Local `lpk_...` keys should be treated as access to the underlying OAuth session.
- The project is not intended for resale, account pooling, or public shared access.

Installation should include:

```bash
go install github.com/HaaapyDay/llm-proxy/cmd/llm-proxy@latest
```

Release download links should point to:

```text
https://github.com/HaaapyDay/llm-proxy/releases
```

## Release Process

Add `docs/release.md` with a concrete release checklist and commands.

The release checklist should require:

- final repository path check
- README and Chinese README link check
- `go test ./...`
- `go vet ./...`
- `govulncheck ./...`
- `goreleaser check`
- `goreleaser release --snapshot --clean`
- target version entry in `CHANGELOG.md`
- no committed runtime data, OAuth tokens, local API keys, or `.env` files
- tag push only after the checks pass

The first release should use `v0.1.0`.

`CHANGELOG.md` should use a conventional structure:

- Added
- Security
- Known Limitations

## CI Design

Keep the existing cross-platform Go CI matrix and add focused release-readiness
checks.

Expected CI behavior:

- run `go test ./...`
- run `go vet ./...`
- run `go build -o bin/llm-proxy ./cmd/llm-proxy`
- run `govulncheck ./...`
- run `go test -race ./...` on Ubuntu only
- run `goreleaser check`

The release workflow should continue to publish on `v*` tags through GoReleaser.

## Security Design

Keep the current local-only security model and document it more prominently.

Update `SECURITY.md`, `CONTRIBUTING.md`, and README content so contributors and
users understand:

- OAuth and local API key data are sensitive.
- Issues must not contain tokens, generated `lpk_...` keys, account identifiers,
  private prompts, or full sensitive payloads.
- Changes to logging, storage, auth, headers, errors, or proxying must consider
  sensitive data exposure.
- Public or untrusted network binding is outside the intended safe operating mode.

## Compatibility Design

Treat protocol compatibility as a first-class support surface.

Strengthen `docs/compatibility.md` around:

- supported endpoints
- supported request features
- supported streaming behavior
- known unsupported features
- provider-specific behavior
- how to report compatibility issues

The README should summarize compatibility rather than duplicate every detail.

## Client Examples

Add `docs/clients.md` for client setup examples that are useful but too detailed
for the README.

Initial examples should cover:

- curl
- generic OpenAI-compatible clients
- generic Anthropic-compatible clients
- LangChain

Specific clients such as Continue, Cline, LiteLLM, or editor integrations can be
added later based on user demand and verified behavior.

## Maintenance Design

Add `docs/maintenance.md` to define maintainer expectations.

It should document:

- provider APIs can change without notice
- compatibility changes require tests or fixtures
- auth, storage, logging, and error-handling changes require explicit security review
- GitHub Enterprise Server is not currently supported
- the project is intended for local development, not public service deployment
- new compatibility claims should be backed by reproducible examples

## GitHub Templates

Keep the existing templates and refine them.

Updates should include:

- PR template: add `govulncheck`, `goreleaser check`, and compatibility/security
  documentation reminders.
- Bug report: ask for `llm-proxy doctor` output when relevant, with sanitization.
- Compatibility report: ask for client version, endpoint, provider, streaming mode,
  minimized sanitized request, and sanitized response/error.
- Feature request: ask whether the proposal changes the security model or public
  exposure surface.

## Out Of Scope

Do not add these in the first readiness pass:

- Docker images
- Homebrew or Scoop packaging
- SBOM generation
- artifact signing
- docs website
- broad client compatibility claims
- core proxy behavior changes

These can be considered after the first public release has real users and issue
feedback.

## Verification

Before considering the readiness pass complete, run:

```bash
go test ./...
go vet ./...
govulncheck ./...
goreleaser check
goreleaser release --snapshot --clean
```

If network restrictions prevent dependency or vulnerability checks locally, record
that limitation and rely on GitHub Actions for final verification.

## Implementation Scope

Expected changed files:

```text
README.md
README.zh-CN.md
CHANGELOG.md
CONTRIBUTING.md
SECURITY.md
.github/pull_request_template.md
.github/ISSUE_TEMPLATE/bug_report.md
.github/ISSUE_TEMPLATE/compatibility_report.md
.github/ISSUE_TEMPLATE/feature_request.md
.github/workflows/ci.yml
docs/compatibility.md
go.mod
cmd/llm-proxy/main.go
internal/**/*.go
```

Expected new files:

```text
docs/release.md
docs/clients.md
docs/maintenance.md
```

The implementation should keep edits scoped to publication readiness and avoid
unrelated refactors.
