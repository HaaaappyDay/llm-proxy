# Roadmap

This document sets expectations about where `llm-proxy` is going, what is
stable, and what is known to be incomplete. It is intentionally undated:
this is a hobby-friendly project and nothing here is a delivery commitment.

For the user-facing change log see [CHANGELOG.md](../CHANGELOG.md). For
contribution flow see [CONTRIBUTING.md](../CONTRIBUTING.md).

## Versioning Policy

`llm-proxy` follows [Semantic Versioning](https://semver.org/) with the
usual pre-`v1.0` caveat:

- While the version is `v0.y.z`, minor (`y`) bumps may include breaking
  changes. Patch (`z`) bumps are reserved for fixes and additive changes.
- Once `v1.0.0` is published, breaking changes will only land in major
  bumps.
- Breaking changes will always be called out in [CHANGELOG.md](../CHANGELOG.md)
  with a migration note.

## Stability Tiers

Different surfaces evolve at different speeds. Pre-`v1.0` the matrix is:

| Surface | Stability | Notes |
| --- | --- | --- |
| `llm-proxy` CLI commands (`serve`, `login`, `keys`, `doctor`, `version`) | Stable in spirit; flags may evolve | Removing or renaming a command will be called out in the changelog. |
| `GET /health` | Stable | `{"status":"ok"}` shape will not change. |
| Authentication via `Authorization: Bearer lpk_...` / `x-api-key` | Stable | Both header forms will remain accepted. |
| `POST /v1/messages`, `/v1/chat/completions`, `/v1/responses` paths and methods | Stable | Path strings and HTTP methods are frozen. |
| Conversion semantics across protocols | Evolving | Best-effort; tracked against real client traffic. May add fields, expand supported features, or tighten validation. |
| Error response envelopes | Evolving | See [Error Responses](errors.md). Convergence to a single object envelope is planned. |
| Debug log line format | Evolving | Stderr-only, intended for humans. Do not parse. |
| On-disk file formats under `~/.llm-proxy/` | Evolving with migration | Auth stores carry a `version` field; SQLite uses migrations. Existing data will be upgraded in place. |

## Planned

In rough priority order. Status is `idea` (still being scoped) or `planned`
(scoped, accepting PRs).

- **planned** Unified structured error envelope across all handlers
  ([docs/errors.md](errors.md)) so clients can rely on `{"error": {"type":
  "...", "message": "..."}}` everywhere instead of the current
  string/object mix.
- **planned** Architecture, OAuth, and error specifications kept
  current with the code (this document set landed; PRs that touch the
  matching code should update these files).
- **idea** Additional client recipes in [docs/clients.md](clients.md)
  (Claude Code, Cursor, Continue, Aider, Cline, official OpenAI and
  Anthropic SDKs).
- **idea** Optional structured (JSON) debug logging, gated behind an
  env var, so log lines can be ingested by external tools without
  changing the default human-readable format.
- **idea** GitHub Enterprise Server support for Copilot, including
  the `domain` plumbing already reserved in
  [auth/copilot.go](../internal/auth/copilot.go) and
  [auth/types.go](../internal/auth/types.go).
- **idea** Per-key account selection (`keys create --account`) so a
  user with multiple logged-in accounts on the same provider can bind a
  key to a non-default account without editing files on disk.
- **idea** Documented model name mapping (what the client sends vs.
  what the upstream sees).
- **idea** Optional Prometheus-style metrics endpoint bound to the
  local interface for long-running serves.
- **idea** FAQ doc, screenshots/demo asset for the README, and a
  third-party license inventory for the dependency tree.

## Known Issues

Documented here so users do not have to rediscover them; track them in
GitHub Issues if you hit them.

- **Inconsistent error response shapes.** Some failure paths return
  `{"error": "<string>"}` and others return `{"error": {"type": "..."}}`.
  See [Error Responses](errors.md) for the current matrix.
- **Upstream error headers are dropped.** Headers like `Retry-After` are
  not forwarded on the error path because only `Content-Type` is set
  before writing the envelope. Clients that retry on 429 cannot read
  `Retry-After` today.
- **No documented model name mapping.** Clients pass `model` strings
  through verbatim; the actual upstream model selection depends on the
  provider and the request path. This will be documented once stabilized.
- **Streaming tool-call delta conversion is conservative.** Some edge
  cases (interleaved tool calls and reasoning blocks across providers)
  may cut the stream rather than translate. See
  [docs/compatibility.md](compatibility.md).
- **No CLI flag to choose a non-default account when minting a key.**
  Switch via `llm-proxy login` to change the default account first.
- **No graceful handling of clock skew on token expiry math.** The 60 s
  refresh buffer absorbs small skew; large skew can produce spurious
  refresh attempts.
- **`api_keys.json` legacy import has no version bump.** It is imported
  into SQLite on startup and left in place forever; there is no cleanup
  command.

## Non-Goals

These are out of scope and will not be accepted as feature work. They are
listed here so contributors do not invest time on them and so users do not
expect them.

- Public-internet deployment as a hosted service. `llm-proxy` is a local
  development tool. See [SECURITY.md](../SECURITY.md).
- Shared multi-tenant API access, account pooling, or reselling provider
  access through the proxy.
- Bypassing provider rate limits, account restrictions, access controls,
  or terms of service.
- Storing plaintext `lpk_...` keys on disk. Only SHA-256 hashes plus
  metadata are persisted; lost keys are re-issued, not recovered.
- Logging full request or response bodies, OAuth tokens, or API keys, even
  in debug mode.
- Web UI for login or key management. CLI-only is a deliberate security
  boundary.

## How to Propose Changes

- For small, well-scoped changes (a bug fix, a documented feature,
  a new client recipe), open a PR directly. Follow
  [CONTRIBUTING.md](../CONTRIBUTING.md).
- For larger changes (new transport, new provider, behavior changes to
  the auth or error surfaces, anything in the "Non-Goals" gray area),
  open an issue first to discuss before writing code.
- For protocol conversion changes, include fixtures or table tests that
  demonstrate the shapes you support.
- For security-sensitive reports, follow [SECURITY.md](../SECURITY.md)
  and do not open a public issue.
