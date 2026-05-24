# Client Configuration Examples

`llm-proxy` exposes Anthropic-compatible and OpenAI-compatible local endpoints.
All proxy endpoints require a local `lpk_...` key created by `llm-proxy login`
or `llm-proxy keys create`.

The default local server is:

```text
http://127.0.0.1:15721
```

## Environment Variables

For OpenAI-compatible clients:

```bash
export OPENAI_BASE_URL=http://127.0.0.1:15721/v1
export OPENAI_API_KEY=lpk_xxxx
```

For Anthropic-compatible clients:

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:15721
export ANTHROPIC_AUTH_TOKEN=lpk_xxxx
```

Some clients use `ANTHROPIC_API_KEY` instead of `ANTHROPIC_AUTH_TOKEN`:

```bash
export ANTHROPIC_API_KEY=lpk_xxxx
```

## curl

OpenAI Chat Completions:

```bash
curl http://127.0.0.1:15721/v1/chat/completions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.2",
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

Anthropic Messages:

```bash
curl http://127.0.0.1:15721/v1/messages \
  -H "Authorization: Bearer $ANTHROPIC_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.2",
    "max_tokens": 128,
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

## OpenAI-Compatible SDKs

Use the local `/v1` base URL and the generated local API key:

```text
base_url: http://127.0.0.1:15721/v1
api_key: lpk_xxxx
```

Clients that support `OPENAI_BASE_URL` and `OPENAI_API_KEY` usually work without
code changes.

## Anthropic-Compatible Clients

Use the root local URL and the generated local API key:

```text
base_url: http://127.0.0.1:15721
api_key: lpk_xxxx
```

The Anthropic-compatible endpoint is:

```text
POST /v1/messages
```

## LangChain

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

## Compatibility Reports

If a client fails, open a compatibility report with:

- client name and version
- endpoint
- provider
- streaming mode
- minimized sanitized request body
- sanitized response or error

Do not include OAuth tokens, generated `lpk_...` keys, account identifiers,
private prompts, file IDs, or full sensitive payloads.

