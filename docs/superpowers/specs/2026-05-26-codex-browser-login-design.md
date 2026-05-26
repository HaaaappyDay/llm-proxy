# Codex Browser OAuth Login Design

## Summary

`llm-proxy login codex` should let users choose between two Codex login
methods:

- Browser OAuth, the default interactive choice.
- Device code, the existing headless-friendly flow.

This change applies only to Codex. GitHub Copilot login keeps its current
device-code behavior.

The browser flow must print the authorization URL instead of automatically
opening a browser. Users can click or copy the URL themselves. This removes the
need for a Codex-specific `--no-browser` mode and keeps the login method choice
separate from browser-launch behavior.

## Goals

- Add a Codex login method prompt for interactive `llm-proxy login codex`.
- Make Browser OAuth the default prompt choice.
- Keep the existing Codex device-code flow available from the prompt.
- Add non-interactive Codex flags for scripts and tests: `--browser` and
  `--device-code`.
- Keep Copilot login behavior unchanged.
- Preserve the existing Codex account storage shape and local API key minting
  behavior.

## Non-Goals

- Do not add an HTTP login or key-management endpoint.
- Do not change Copilot OAuth behavior.
- Do not automatically open the browser for Codex Browser OAuth.
- Do not expose OAuth tokens, authorization codes, or callback query strings in
  logs, CLI output, or browser success pages.

## CLI Behavior

### Interactive Codex Login

Running:

```bash
llm-proxy login codex
```

prints:

```text
Choose Codex login method:
  1. Browser OAuth
  2. Device code

Selection [1]:
```

Input handling:

- Empty input selects Browser OAuth.
- `1` or `browser` selects Browser OAuth.
- `2`, `device`, or `device-code` selects device code.
- Unknown input returns a clear error and does not start either flow.

If stdin is not interactive and no flow flag was provided, the command fails
with a message telling the user to pass `--browser` or `--device-code`.

### Non-Interactive Codex Flags

Codex supports:

```bash
llm-proxy login codex --browser
llm-proxy login codex --device-code
```

Rules:

- `--browser` selects Browser OAuth and skips the prompt.
- `--device-code` selects the existing device-code flow and skips the prompt.
- Passing both flags is an error.
- `llm-proxy login codex --no-browser` is obsolete and should fail with a
  message telling users to choose `--browser` or `--device-code`.

### Copilot Compatibility

`llm-proxy login copilot` keeps the current device flow.

`llm-proxy login copilot --no-browser` remains valid and continues to print the
verification URL instead of trying to open it. The new Codex-specific
`--browser` and `--device-code` flags do not apply to Copilot.

## Browser OAuth Flow

When Browser OAuth is selected, the CLI:

1. Generates a PKCE verifier/challenge pair and a random `state`.
2. Starts a short-lived callback server bound to `127.0.0.1`.
3. Uses port `1455` by default. If unavailable, tries port `1457`.
4. Constructs an authorization URL against `https://auth.openai.com/oauth/authorize`.
5. Prints the URL and waits for the callback.

Example output:

```text
Open this URL in your browser:
https://auth.openai.com/oauth/authorize?...

Waiting for browser authorization on http://localhost:1455/auth/callback ...
```

The command does not call the existing browser launcher for this flow.

After the user authorizes in the browser, OpenAI redirects to:

```text
http://localhost:<port>/auth/callback?code=...&state=...
```

The callback handler:

- Validates `state`.
- Rejects missing authorization codes.
- Converts OAuth callback errors into clear CLI errors.
- Exchanges the authorization code with the original PKCE verifier.
- Reuses the existing Codex token parsing and account persistence path.
- Reuses the existing local `lpk_...` API key creation and environment output.

The browser response should be a minimal success or failure page. It must not
include tokens, authorization codes, account identifiers, or local API keys.

## Device-Code Flow

When Device code is selected, Codex reuses the existing device-code
implementation:

1. Request the device/user code from `auth.openai.com`.
2. Print the verification URL and user code.
3. Poll until authorization succeeds, expires, or fails.
4. Exchange the returned authorization code and verifier for tokens.
5. Persist the account and mint a local API key as today.

The Codex device-code path should print the verification URL instead of trying
to open a browser. Browser launching is no longer part of Codex login behavior.

Example output:

```text
Open: https://auth.openai.com/codex/device
Code: XXXX-XXXX
Waiting for authorization...
```

## Code Structure

### `cmd/llm-proxy/main.go`

The command layer should:

- Parse Codex-only `--browser` and `--device-code` flags.
- Keep `--no-browser` for Copilot.
- Reject invalid flag/provider combinations with clear messages.
- Prompt for the Codex flow only when no Codex flow flag is supplied and stdin
  is interactive.
- Dispatch to separate device-code and browser-login runner functions.

The current generic `runLogin` helper can become a device-login helper, or it
can be split into provider-neutral key minting plus flow-specific login
functions. The split should keep account persistence in `internal/auth` and API
key creation in the command/application layer, matching the current ownership.

### `internal/auth/codex.go`

Codex auth should gain browser OAuth support while reusing existing token and
store logic.

Expected additions:

- A browser flow starter that returns the printed authorization URL and owns the
  callback server lifecycle.
- Callback handling for success, OAuth error, state mismatch, cancellation, and
  timeout.
- A token exchange path that accepts a flow-specific `redirect_uri`.

The existing `exchangeCodeForTokens` helper currently uses the device redirect
URI. It should be changed to accept `redirectURI` as an argument so both browser
OAuth and device code use the same token exchange logic without duplicating HTTP
request handling.

Existing helpers such as token identity extraction, token caching, and
`addAccount` should be reused.

### `internal/auth/constants.go`

Add constants for Browser OAuth:

- OpenAI authorize URL or issuer base.
- Default callback host and ports.
- Browser OAuth scopes, if kept centralized.

Keep existing device-code constants unchanged.

## Security Requirements

- Bind the callback server to `127.0.0.1` only.
- Use `http://localhost:<port>/auth/callback` as the OAuth `redirect_uri`,
  matching the official Codex callback shape while still binding locally.
- Generate high-entropy `state` and validate it exactly.
- Use PKCE S256 for browser OAuth.
- Do not log or print authorization codes, access tokens, refresh tokens, ID
  tokens, full callback URLs, or full callback query strings.
- Redact sensitive token exchange errors if they contain request URLs.
- Shut down the callback server after success, cancellation, timeout, or fatal
  error.
- Treat refresh tokens persisted in `codex_oauth_auth.json` with the same file
  permission expectations as today.

## Error Handling

The command should produce actionable errors for:

- Non-interactive `login codex` without `--browser` or `--device-code`.
- Conflicting `--browser` and `--device-code` flags.
- Obsolete Codex `--no-browser` usage.
- Browser callback port conflicts.
- Browser callback timeout.
- Browser callback cancellation.
- OAuth callback `error` responses.
- Missing callback code.
- State mismatch.
- Token exchange failure.

The browser page can show short success or failure text, but the CLI error
message is the primary diagnostic surface.

## Documentation Updates

Update:

- `README.md`
- `README.zh-CN.md`
- `docs/oauth.md`
- CLI help text and CLI help tests, if present

Docs should describe:

- Codex has Browser OAuth and Device code choices.
- Browser OAuth prints a URL and waits for a localhost callback.
- Device code remains the right choice for headless or remote environments.
- Copilot remains device-code based.
- OAuth login and API key creation remain CLI-only.

## Test Plan

Unit and command tests should cover:

- Codex prompt default selection.
- Codex prompt aliases for browser and device code.
- Unknown Codex prompt input.
- Non-interactive Codex login without a flow flag.
- `--browser` and `--device-code` conflict.
- Codex `--no-browser` obsolete error.
- Copilot `--no-browser` still accepted.
- Browser callback state mismatch.
- Browser callback missing code.
- Browser callback OAuth error response.
- Browser callback success path exchanges tokens and persists the account.
- Device-code path still uses the existing polling behavior.

Tests must not require real Codex, GitHub, Copilot, or OpenAI credentials.
Network-facing OAuth interactions should use fake HTTP clients or local test
servers, consistent with the existing auth tests.

## Implementation Decisions

- The callback success page should use a minimal "login complete; return to the
  terminal" message and no dynamic account or key data.
- The first implementation should support only port `1455` plus fallback port
  `1457`. A custom callback port flag can be considered later if users need it.
