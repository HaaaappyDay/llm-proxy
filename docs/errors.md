# Error Responses

This document specifies the JSON shapes `llm-proxy` returns for HTTP errors
on its `/v1/...` surface, the conditions that trigger each shape, and which
fields clients can rely on.

The implementation lives in
[internal/proxy/middleware.go](../internal/proxy/middleware.go),
[internal/proxy/handlers.go](../internal/proxy/handlers.go), and
[internal/proxy/forward.go](../internal/proxy/forward.go).

## Current State

Pre-`v1.0`, `llm-proxy` returns a structured `{"error": {...}}` envelope
for proxy endpoint errors. The exact field set still varies by source while
the surface converges.

| Source | Status | Envelope |
| --- | --- | --- |
| API key middleware | `401` | `{"error": {"type": "invalid_api_key", "message": "...", "status": 401}}` |
| Handler local checks (auth context, body read, generic forwarder failure) | `401`, `400`, `413`, `502` | `{"error": {"type": "...", "message": "...", "status": N}}` |
| Transform validation (unsupported feature) | `400` | `{"error": {"type": "unsupported_feature", "message": "...", "source_format": "...", "target_format": "...", "unsupported_feature": "..."}}` |
| Upstream non-2xx response | upstream status (`>=400`) | `{"error": {"type": "upstream_error", "message": "...", "upstream_status": N, "retry_after"?: "...", "body_preview"?: "...", "body_truncated"?: true}}` |

Clients should branch on `body.error.type` when they need a specific
remediation, or on the HTTP status when only the status class matters.

## Canonical Shapes

### Auth Error

Returned by the API key middleware
([middleware.go:26](../internal/proxy/middleware.go)) and by handler
fallbacks when the API key record is missing from context
([handlers.go](../internal/proxy/handlers.go)).

```json
{
  "error": {
    "type": "invalid_api_key",
    "message": "invalid or missing api key",
    "status": 401
  }
}
```

### Request Body Error

Returned by `readRequestBody` in
[handlers.go](../internal/proxy/handlers.go) when the inbound body cannot be
read or exceeds the 32 MiB cap (`MaxRequestBodyBytes`):

```json
{
  "error": {
    "type": "request_too_large",
    "message": "http: request body too large",
    "status": 413
  }
}
```

Status is `413 Payload Too Large` when the body exceeds the cap, otherwise
`400 Bad Request` with type `invalid_request`.

### Unsupported Feature (object)

Returned by `writeProxyError` in
[handlers.go](../internal/proxy/handlers.go) when transformation rejects an
input that has no equivalent on the chosen upstream. The underlying type is
`transform.UnsupportedFeatureError`
([unified.go](../internal/transform/unified.go)).

```json
{
  "error": {
    "type": "unsupported_feature",
    "message": "unsupported feature \"audio_content\" when converting from openai_chat_completions to openai_responses",
    "source_format": "openai_chat_completions",
    "target_format": "openai_responses",
    "unsupported_feature": "audio_content"
  }
}
```

`source_format` and `target_format` are drawn from this enum:

- `anthropic_messages`
- `openai_chat_completions`
- `openai_responses`

Known `unsupported_feature` values include `audio_content`, `file_content`,
`reasoning_or_thinking_content`, `previous_response_id`,
`structured_response_format`, `custom_tool_call`,
`hosted_or_custom_tool:<type>`, `tool_choice:<mode>`, and
`unknown_content:<raw_type>`. The set may grow.

### Upstream Error (object)

Returned by `writeUpstreamStatusError` in
[forward.go](../internal/proxy/forward.go) whenever an upstream HTTP call
returns `>= 400`. The local HTTP status mirrors the upstream status.

```json
{
  "error": {
    "type": "upstream_error",
    "message": "upstream returned status 429",
    "upstream_status": 429,
    "retry_after": "12",
    "body_preview": "{\"error\":\"rate limited\"}"
  }
}
```

When the upstream response includes `Retry-After`, the proxy forwards it as
an HTTP response header and also mirrors the raw string in `retry_after`.
The value can be either seconds or an HTTP date; clients should parse it
according to the standard header semantics.

`body_preview` is included when the upstream returned a non-empty error
body. It is trimmed, capped at 4 KiB, and redacts common token, API key,
authorization, and secret fields before returning the preview to the
client.

If the upstream body exceeded the preview cap, the envelope also includes:

```json
{
  "error": {
    "type": "upstream_error",
    "message": "upstream returned status 500",
    "upstream_status": 500,
    "body_preview": "...",
    "body_truncated": true
  }
}
```

Full upstream error bodies are **not** forwarded to the client. The preview
is only a bounded diagnostic summary. A matching truncated preview is also
available in stderr when `LLM_PROXY_DEBUG=1` is set (see below).

### Generic Forwarder Error

Returned by the final fallback in `writeProxyError`
([handlers.go:130](../internal/proxy/handlers.go)) for forwarder errors that
are neither `UnsupportedFeatureError` nor `UpstreamStatusError`. Status is
`502 Bad Gateway`.

```json
{
  "error": {
    "type": "proxy_error",
    "message": "post upstream: ...",
    "status": 502
  }
}
```

Common triggers: network failure reaching the upstream, JSON decode failure
on a 2xx upstream response, or an OAuth refresh error such as
`ErrRefreshInvalid` surfaced from the auth layer. See
[OAuth and Token Lifecycle](oauth.md) for refresh-related remediation.

Malformed client JSON is returned earlier as `400 invalid_request`.

## Status Code Reference

| Status | Cause | Envelope |
| --- | --- | --- |
| `401` | Missing or invalid `Authorization` / `x-api-key` (no matching active `lpk_...`). | object `invalid_api_key` |
| `400` | Body could not be read; malformed JSON; client request fails transform validation. | object `invalid_request` or `unsupported_feature` |
| `413` | Body exceeds 32 MiB. | object `request_too_large` |
| `400-499` (from upstream) | Upstream rejected the proxied request. | object `upstream_error` |
| `500-599` (from upstream) | Upstream server error. | object `upstream_error` |
| `502` | Forwarder could not complete the request (network, decode, refresh, etc.). | object `proxy_error` |

`GET /health` is not part of this surface: it always returns
`200 {"status":"ok"}` and is unauthenticated.

## Streaming Errors

For streaming requests, errors that occur **before** the upstream stream
opens follow the table above and are returned as a normal JSON response.

Errors that occur **after** the response status and headers have been
written (mid-stream upstream failure, conversion failure on an unsupported
event type) cause the stream to terminate without an additional JSON
envelope. The handler checks `c.Writer.Written()` before writing an error
to avoid corrupting an already-flushed response. Clients should treat an
abrupt stream termination as a recoverable error and not assume a final
JSON object.

The conversion layer's policy is conservative: rather than synthesize an
event with no target-side equivalent, the stream is cut. This matches the
guidance in [docs/compatibility.md](compatibility.md).

## Worked Examples

### Missing API key

```http
POST /v1/messages HTTP/1.1
Host: 127.0.0.1:15721
Content-Type: application/json

{...}
```

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{"error":{"type":"invalid_api_key","message":"invalid or missing api key","status":401}}
```

### Audio content sent to a Codex account

`audio` content blocks have no representation in Codex Responses, so
validation fails before any upstream call.

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "error": {
    "type": "unsupported_feature",
    "message": "unsupported feature \"audio_content\" when converting from openai_chat_completions to openai_responses",
    "source_format": "openai_chat_completions",
    "target_format": "openai_responses",
    "unsupported_feature": "audio_content"
  }
}
```

### Upstream rate limit (429)

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 12

{
  "error": {
    "type": "upstream_error",
    "message": "upstream returned status 429",
    "upstream_status": 429,
    "retry_after": "12",
    "body_preview": "{\"error\":\"rate limited\"}"
  }
}
```

Clients implementing retries should honor `Retry-After` when present. On
the error path, the proxy forwards the standard `Retry-After` header but
does not forward provider-specific rate-limit headers.

### Body exceeds 32 MiB

```http
HTTP/1.1 413 Request Entity Too Large
Content-Type: application/json

{"error":{"type":"request_too_large","message":"http: request body too large","status":413}}
```

### Refresh token revoked mid-request

The upstream call fails before it leaves the proxy because
`auth.GetValidToken` returns `ErrRefreshInvalid`. The forwarder surfaces
this as a generic 502.

```http
HTTP/1.1 502 Bad Gateway
Content-Type: application/json

{"error":{"type":"proxy_error","message":"refresh token invalid","status":502}}
```

Remediation: re-run `llm-proxy login <provider>`. See
[OAuth and Token Lifecycle](oauth.md#failure-modes-and-remediation).

## Debug Logging

Setting `LLM_PROXY_DEBUG=1` before starting the proxy enables structured
key-value logging to stderr. Relevant lines for error investigation include:

```
upstream method=POST label=codex.chat.completions model=gpt-5.2 stream=false url=https://chatgpt.com/... status=429 duration=120ms
upstream_error label=codex.chat.completions model=gpt-5.2 status=429 truncated=false preview="<first 4 KiB of upstream body>"
```

Debug logs deliberately omit:

- `lpk_...` API keys
- OAuth access tokens, refresh tokens, GitHub tokens, Copilot tokens
- Full request and response bodies

The upstream body preview is truncated to 4 KiB
(`maxUpstreamErrorBodyPreviewBytes` in
[forward.go](../internal/proxy/forward.go)) and common token-like values
are redacted before the preview is written to the client response. For a
richer view, reproduce the failure against the upstream directly with
credentials you own.

## Stability Promise (Pre-1.0)

Stable across pre-`v1.0` minor versions:

- HTTP status code for each cause listed in the
  [status code reference](#status-code-reference).
- The `upstream_error` envelope: presence of `type`, `message`,
  `upstream_status`, and the optional `retry_after`, `body_preview`, and
  `body_truncated` fields.
- The `unsupported_feature` envelope: presence of `type`, `message`,
  `source_format`, `target_format`, and `unsupported_feature`. The set of
  enumerated `unsupported_feature` values may grow.

Subject to change pre-`v1.0`:

- The exact string of `message` fields.
- Whether provider-specific rate-limit headers are surfaced on the error
  path.

Breaking changes to error envelopes will be called out in
[CHANGELOG.md](../CHANGELOG.md).
