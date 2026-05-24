# llm-proxy

[中文文档](README.zh-CN.md)

> **Use your ChatGPT Plus / GitHub Copilot subscription in any tool that expects an OpenAI or Anthropic API key — locally, no extra fees.**

`llm-proxy` is a local OAuth-to-API-key gateway for OpenAI Codex and GitHub Copilot accounts. It turns your existing subscription into a local `lpk_...` API key and a `http://127.0.0.1:15721` base URL that any OpenAI- or Anthropic-compatible client can use.

It speaks all three mainstream LLM API shapes and converts between them transparently:

- **Anthropic Messages** — `POST /v1/messages`
- **OpenAI Chat Completions** — `POST /v1/chat/completions`
- **OpenAI Responses** — `POST /v1/responses`
- **OpenAI Models** — `GET /v1/models`

So a Claude-Code-style client speaking Anthropic Messages can talk to a Codex (OpenAI Responses) account, and an OpenAI SDK can talk to a Copilot account — without changing the client.

## Why / Use Cases

If you already pay for **ChatGPT Plus / Pro** or **GitHub Copilot**, you usually cannot use that subscription from arbitrary developer tools that expect a raw API key. `llm-proxy` bridges that gap, locally:

- **Use Codex (ChatGPT) in Anthropic-only tools** — point Claude Code, Cline (Anthropic mode), or any Anthropic SDK at `http://127.0.0.1:15721` and your Codex subscription answers the call.
- **Use GitHub Copilot in OpenAI-SDK tools** — drop your Copilot subscription into Cursor's custom OpenAI base URL, Aider, Continue, Open WebUI, LangChain, LlamaIndex, or any `OPENAI_BASE_URL`-aware tool.
- **Mix protocols freely** — an Anthropic Messages request hitting a Codex account is converted to OpenAI Responses upstream and the streaming reply is converted back to Anthropic SSE. Same for Chat Completions ↔ Responses.
- **Keep one local key per machine** — `lpk_...` keys are stored as SHA‑256 hashes and never leave the box; revoke or rotate without touching the OAuth session.
- **No third-party server, no account pooling** — only `127.0.0.1` by default, only your own account, only your own machine.

Typical setups:

| Your subscription | Client tool | Works via |
| --- | --- | --- |
| ChatGPT Plus / Pro (Codex) | Claude Code, Cline, Anthropic SDK | Anthropic Messages → Codex Responses |
| ChatGPT Plus / Pro (Codex) | Cursor, Aider, Continue, OpenAI SDK | OpenAI Chat / Responses → Codex Responses |
| GitHub Copilot | Cursor, Aider, OpenAI SDK, LangChain | OpenAI Chat / Responses → Copilot |
| GitHub Copilot | Claude Code, Anthropic SDK | Anthropic Messages → Copilot Chat / Responses |

**Safety and compliance:** this project does not bypass provider limits, account restrictions, access controls, or terms. Do not expose it as a public service, resell access through it, or use it for account pooling. You are responsible for using Codex, GitHub Copilot, OpenAI, and GitHub in compliance with their applicable terms and policies.

## How It Compares

| | `llm-proxy` | Typical Copilot-only proxy | Typical Codex-only proxy |
| --- | --- | --- | --- |
| Codex (ChatGPT) account support | ✅ | ❌ | ✅ |
| GitHub Copilot account support | ✅ | ✅ | ❌ |
| OpenAI Chat Completions endpoint | ✅ | ✅ | partial |
| OpenAI Responses endpoint | ✅ | ❌ | ✅ |
| Anthropic Messages endpoint | ✅ | rare | rare |
| Cross-protocol conversion (Anthropic ↔ Chat ↔ Responses) | ✅ | ❌ | ❌ |
| Local `lpk_...` API keys (hashed at rest) | ✅ | ❌ | ❌ |
| Pure-Go build, no CGO, single static binary | ✅ | varies | varies |
| Localhost-only by default, no public surface | ✅ | varies | varies |

## Features

- CLI-based OAuth device login for Codex and GitHub Copilot.
- Local `lpk_...` API keys backed by OAuth sessions.
- Anthropic-compatible and OpenAI-compatible HTTP endpoints.
- Request and response conversion between Anthropic Messages, OpenAI Chat Completions, and OpenAI Responses.
- Streaming support for compatible upstream requests.
- Local token storage protected by restrictive file permissions.
- API key hashes are persisted; plaintext API keys are shown only when created.
- No HTTP login or HTTP key management surface.

## Security Model

`llm-proxy` is designed as a local gateway, not a public service.

- It listens on `127.0.0.1:15721` by default.
- OAuth login and API key creation are only available through `llm-proxy login`.
- The HTTP server only exposes health and model proxy endpoints.
- Do not bind it to a public interface unless you fully control the network.
- A local `lpk_...` API key should be treated like access to the underlying OAuth session.
- Plaintext API keys are not stored. If you lose one, create a new key by running `llm-proxy login` again.
- When binding to any non-localhost interface, the server prints a warning. Treat that mode as unsafe unless the network is fully trusted.
- Do not use this project to resell, pool, or publicly share access to provider accounts.

## Requirements

- Go 1.25+
- Network access to the relevant upstream services:
  - `auth.openai.com`
  - `chatgpt.com`
  - `github.com`
  - `api.github.com`

If Go is installed but not on `PATH`, add it before building:

```bash
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
```

## Installation

### With curl

```bash
curl -fsSL https://raw.githubusercontent.com/HaaapyDay/llm-proxy/main/install.sh | sh
```

The installer downloads the prebuilt binary from GitHub Releases, verifies it
with `checksums.txt`, and installs it to `~/.local/bin`. It does not require Go.
If `~/.local/bin` is not on `PATH`, the installer will ask before updating your
shell configuration in interactive terminals.

### With Go

```bash
go install github.com/HaaapyDay/llm-proxy/cmd/llm-proxy@latest
```

### From GitHub Releases

Download the archive for your platform from:

```text
https://github.com/HaaapyDay/llm-proxy/releases
```

Verify the archive with the published `checksums.txt`, extract it, and put `llm-proxy` on your `PATH`.

### From Source

```bash
git clone https://github.com/HaaapyDay/llm-proxy.git
cd llm-proxy
go build -o bin/llm-proxy ./cmd/llm-proxy
```

`llm-proxy` uses a pure Go SQLite driver, so normal source builds do not require a C compiler.

## Quick Start

Log in to one provider. The command starts an OAuth device flow, opens the browser when possible, and creates a local API key.

```bash
./bin/llm-proxy login codex
# or
./bin/llm-proxy login copilot
```

On headless machines, skip browser launch and open the printed URL manually:

```bash
./bin/llm-proxy login codex --no-browser
```

After authorization, the command prints environment variables similar to:

```bash
export LLM_PROXY_API_KEY=lpk_...
export ANTHROPIC_BASE_URL=http://127.0.0.1:15721
export ANTHROPIC_AUTH_TOKEN=lpk_...
export OPENAI_BASE_URL=http://127.0.0.1:15721/v1
export OPENAI_API_KEY=lpk_...
```

Start the gateway:

```bash
./bin/llm-proxy serve
```

Use a custom address only for trusted local networks:

```bash
./bin/llm-proxy serve --host 127.0.0.1 --port 15721
```

## Client Configuration

See [Client Configuration Examples](docs/clients.md) for more SDK and tool setup patterns.

### Anthropic-Compatible Clients

For tools that speak Anthropic Messages, point the Anthropic base URL at the local gateway and use the generated `lpk_...` key.

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:15721
export ANTHROPIC_AUTH_TOKEN=lpk_xxxx
```

Example request:

```bash
curl http://127.0.0.1:15721/v1/messages \
  -H "Authorization: Bearer $LLM_PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.2",
    "max_tokens": 128,
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

### OpenAI-Compatible Clients

For OpenAI SDKs and compatible tools:

```bash
export OPENAI_BASE_URL=http://127.0.0.1:15721/v1
export OPENAI_API_KEY=lpk_xxxx
```

Example request:

```bash
curl http://127.0.0.1:15721/v1/chat/completions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.2",
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

### LangChain

Python example using `langchain-openai`:

```bash
pip install langchain-openai
export OPENAI_BASE_URL=http://127.0.0.1:15721/v1
export OPENAI_API_KEY=lpk_xxxx
```

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    model="gpt-5.2",
    base_url="http://127.0.0.1:15721/v1",
    api_key="lpk_xxxx",
)

print(llm.invoke("hello").content)
```

## API Reference

### Public

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/health` | Health check. Does not require an API key. |

### Proxy

All proxy endpoints require an API key:

```http
Authorization: Bearer lpk_...
```

`x-api-key: lpk_...` is also accepted for clients that cannot set bearer tokens.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/v1/models` | OpenAI-compatible model list endpoint. |
| `POST` | `/v1/messages` | Anthropic Messages-compatible endpoint. |
| `POST` | `/v1/chat/completions` | OpenAI Chat Completions-compatible endpoint. |
| `POST` | `/v1/responses` | OpenAI Responses-compatible endpoint. |

## Compatibility

The proxy uses a shared intermediate representation when it has to translate between Anthropic Messages, OpenAI Chat Completions, and OpenAI Responses.

Supported cross-format features include:

- OpenAI-compatible model listing for clients that discover models from `/v1/models`.
- Text messages and system/developer instructions.
- Image inputs where the target format supports image content.
- OpenAI Responses file inputs when the request stays on a Responses-compatible path.
- Function tools, common `tool_choice` modes, tool calls, and tool results.
- Basic sampling controls such as `temperature`, `top_p`, and max token fields where the selected upstream accepts them.
- Streaming text deltas and function tool call argument deltas.

Unsupported features return a structured `400` error instead of being silently dropped. This includes audio when translating away from OpenAI Chat, file inputs when translating to Anthropic or Chat, hosted/built-in Responses tools on non-Responses targets, response state such as `previous_response_id` on non-Responses targets, and reasoning/thinking blocks when the target protocol cannot represent them.

See [Compatibility Matrix](docs/compatibility.md) for endpoint behavior, provider-specific notes, and compatibility report guidance.

## CLI Reference

```bash
llm-proxy serve [--host 127.0.0.1] [--port 15721] [--data-dir ~/.llm-proxy]
llm-proxy login codex|copilot [--no-browser]
llm-proxy keys list
llm-proxy keys create codex|copilot [--label NAME]
llm-proxy keys delete KEY_ID
llm-proxy doctor
llm-proxy version
```

### `serve`

Starts the local HTTP gateway.

Enable debug logs for upstream troubleshooting:

```bash
LLM_PROXY_DEBUG=1 llm-proxy serve
```

Debug logs are written to stderr and include upstream URL, provider path, model, status, duration, and a truncated upstream error preview. They do not log API keys, OAuth tokens, or full request bodies.

### `login`

Runs an OAuth device login for `codex` or `copilot`, stores the OAuth session locally, and creates a local API key.

The plaintext API key is printed once. The persisted key store contains only a SHA-256 hash and metadata.

### `keys`

Manages local API keys without exposing an HTTP key management surface.

```bash
llm-proxy keys list
llm-proxy keys create codex --label work
llm-proxy keys delete KEY_ID
```

`keys list` shows active key metadata and previews only. `keys create` requires an existing logged-in provider account and prints the plaintext API key once. `keys delete` revokes the key locally.

### `doctor`

Checks local configuration, data directory permissions, stored API key metadata, and local auth file parseability. It does not make network requests.

### `version`

Prints the version, commit, and build date. Release binaries populate these fields during the GitHub Releases build.

## Data Directory

Runtime data is stored in `~/.llm-proxy/` by default. The directory is created with `0700` permissions, and files are written with `0600` permissions.

| File | Description |
| --- | --- |
| `codex_oauth_auth.json` | Codex OAuth refresh token store. |
| `copilot_auth.json` | GitHub and Copilot token store. |
| `llm-proxy.db` | SQLite database containing SHA-256 hashes and metadata for local `lpk_...` API keys. |
| `api_keys.json` | Legacy API key store. Imported automatically if present and left in place. |

Do not commit or share this directory.

## Development

```bash
go test ./...
go vet ./...
go build -o bin/llm-proxy ./cmd/llm-proxy
```

Maintainer guidance is documented in [Maintenance Notes](docs/maintenance.md).

Release builds are produced by GoReleaser when a `v*` tag is pushed. Snapshot release configuration can be checked locally with:

```bash
goreleaser release --snapshot --clean
```

See [Release Process](docs/release.md) for the full release checklist.

## Limitations

- Intended for local use only.
- GitHub Copilot support currently targets `github.com`; GitHub Enterprise Server is not supported.
- Upstream availability and model access depend on the authenticated account.
- The proxy follows upstream behavior and may need updates when provider APIs change.
- See [Compatibility Matrix](docs/compatibility.md) for supported protocol conversions and known unsupported features.

## Further Reading

- [Architecture](docs/architecture.md) - package layout and request lifecycle.
- [OAuth and Token Lifecycle](docs/oauth.md) - Codex and Copilot device flows, refresh behavior, and on-disk file fields.
- [Error Responses](docs/errors.md) - HTTP status codes, JSON envelopes, and stability guarantees.
- [Roadmap](docs/roadmap.md) - versioning policy, planned items, known issues, and non-goals.
- [Compatibility Matrix](docs/compatibility.md) - endpoint behavior and provider-specific notes.
- [Client Configuration Examples](docs/clients.md) - SDK and tool setup patterns.
