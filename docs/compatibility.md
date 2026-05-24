# Compatibility Matrix

`llm-proxy` accepts Anthropic-compatible and OpenAI-compatible request shapes, then forwards to the selected account provider. Provider availability and model access still depend on the authenticated account.

## Endpoints

| Client endpoint | Codex account | GitHub Copilot account | Notes |
| --- | --- | --- | --- |
| `GET /v1/models` | Returns an OpenAI-compatible local Codex model list | Forwarded to Copilot `/models` | Intended for clients such as `cc-switch-cli` that discover model IDs. |
| `POST /v1/messages` | Converted to OpenAI Responses upstream | Converted to Chat Completions or Responses upstream | Streaming is converted to Anthropic SSE events when supported. |
| `POST /v1/chat/completions` | Converted to OpenAI Responses upstream | Forwarded to Copilot Chat Completions | Non-streaming Codex responses are converted back to Chat Completions. |
| `POST /v1/responses` | Forwarded to Codex Responses upstream | Forwarded to Copilot Responses when available | This path preserves Responses-specific fields best. |

## Supported Cross-Format Features

- Text messages and system/developer instructions.
- Image inputs where the target protocol supports image content.
- OpenAI Responses file inputs when the request stays on a Responses-compatible path.
- Function tools, common `tool_choice` modes, tool calls, and tool results.
- Basic sampling controls such as `temperature`, `top_p`, and max token fields where the selected upstream accepts them.
- Streaming text deltas and function tool call argument deltas.

## Streaming

Streaming requests are supported when the selected upstream endpoint supports
streaming. The proxy converts common text delta and function tool call argument
delta events between supported protocol shapes.

Streaming compatibility is intentionally conservative. If a stream contains an
event type that cannot be represented in the target protocol, the proxy should
return an explicit error or stop conversion rather than inventing incompatible
local semantics.

## Unsupported or Limited Features

Unsupported features return a structured `400` response instead of being silently dropped. Known unsupported conversions include:

- Audio content when translating away from OpenAI Chat Completions.
- File inputs when translating to Anthropic Messages or Chat Completions.
- Hosted or built-in Responses tools on non-Responses targets.
- Response state such as `previous_response_id` on non-Responses targets.
- Reasoning or thinking blocks when the target protocol cannot represent them.

The Responses endpoint preserves Responses-specific fields best. Prefer
`POST /v1/responses` when a client relies on Responses-only features.

## Error Handling

Upstream HTTP errors are returned as structured `upstream_error` responses with the upstream status code. Full upstream error bodies are not forwarded to avoid exposing account or provider details through the local API surface.

Set `LLM_PROXY_DEBUG=1` when starting `llm-proxy serve` to log upstream URLs, model names, status codes, durations, and truncated upstream error previews to stderr. Debug logs do not include API keys, OAuth tokens, or full request bodies.

## Provider Drift

Codex, GitHub Copilot, and model APIs may change without notice. Compatibility reports should include the endpoint, provider, streaming mode, model family, and a minimized sanitized request body.

## Reporting Compatibility Issues

Use the compatibility report issue template and include:

- client or SDK name and version
- endpoint path
- provider: Codex or Copilot
- model name
- streaming mode
- minimized sanitized request body
- sanitized response or error
- what behavior you expected

Do not include OAuth tokens, generated `lpk_...` keys, account identifiers,
private prompts, file IDs, or full sensitive payloads.
