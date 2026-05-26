# llm-proxy

[English](README.md)

> **把你已有的 ChatGPT Plus / GitHub Copilot 订阅，变成任何「需要 OpenAI 或 Anthropic API Key」的工具都能直接用的本地服务 —— 完全本地，不收额外费用。**

`llm-proxy` 是一个本地 OAuth 到 API Key 的网关。它把你的 Codex（ChatGPT）或 GitHub Copilot 订阅，转换成本地的 `lpk_...` API Key 和 `http://127.0.0.1:15721` Base URL，任何兼容 OpenAI 或 Anthropic 协议的客户端都能直接调用。

它同时支持业界主流的三种 LLM API 协议，并在它们之间透明互转：

- **Anthropic Messages** — `POST /v1/messages`
- **OpenAI Chat Completions** — `POST /v1/chat/completions`
- **OpenAI Responses** — `POST /v1/responses`
- **OpenAI Models** — `GET /v1/models`

也就是说，一个只会说 Anthropic Messages 的客户端（例如 Claude Code）也能调用一个 Codex（OpenAI Responses）账号；反过来，一个 OpenAI SDK 也能调用 Copilot 账号 —— 客户端完全不用改。

## 适用场景 / Why

如果你已经付费订阅了 **ChatGPT Plus / Pro** 或 **GitHub Copilot**，你通常 **没法在任意「只接受 API Key」的开发者工具里用上这份订阅**。`llm-proxy` 在本地解决这件事：

- **在只支持 Anthropic 的工具里用上 Codex（ChatGPT）订阅** —— 把 Claude Code、Cline（Anthropic 模式）、任何 Anthropic SDK 指向 `http://127.0.0.1:15721`，背后实际是你的 Codex 在回复。
- **在 OpenAI 系工具里用上 GitHub Copilot 订阅** —— Cursor 自定义 OpenAI Base URL、Aider、Continue、Open WebUI、LangChain、LlamaIndex 等都可以无缝接入。
- **协议自由互转** —— 客户端发 Anthropic Messages，命中 Codex 账号会被转成 OpenAI Responses 上游，流式响应再转回 Anthropic SSE。Chat Completions ↔ Responses 同理。
- **本机一份本地 Key** —— `lpk_...` 仅以 SHA‑256 散列形式落盘，不会离开本机；轮换/吊销不影响 OAuth 会话本身。
- **没有第三方服务器、不做账号池** —— 默认只监听 `127.0.0.1`，只用你自己的账号，只在你自己的机器上。

典型组合：

| 你已有的订阅 | 客户端工具 | 走的路径 |
| --- | --- | --- |
| ChatGPT Plus / Pro（Codex） | Claude Code、Cline、Anthropic SDK | Anthropic Messages → Codex Responses |
| ChatGPT Plus / Pro（Codex） | Cursor、Aider、Continue、OpenAI SDK | OpenAI Chat / Responses → Codex Responses |
| GitHub Copilot | Cursor、Aider、OpenAI SDK、LangChain | OpenAI Chat / Responses → Copilot |
| GitHub Copilot | Claude Code、Anthropic SDK | Anthropic Messages → Copilot Chat / Responses |

**安全与合规提示：** 本项目不会绕过 provider 的额度、账号限制、访问控制或服务条款。不要将它作为公网服务暴露，不要通过它转售访问能力，也不要用于账号池化。使用 Codex、GitHub Copilot、OpenAI 和 GitHub 时，你需要自行遵守对应条款和政策。

## 与同类项目的对比

| | `llm-proxy` | 仅 Copilot 的代理 | 仅 Codex 的代理 |
| --- | --- | --- | --- |
| Codex（ChatGPT）账号 | ✅ | ❌ | ✅ |
| GitHub Copilot 账号 | ✅ | ✅ | ❌ |
| OpenAI Chat Completions 端点 | ✅ | ✅ | 部分 |
| OpenAI Responses 端点 | ✅ | ❌ | ✅ |
| Anthropic Messages 端点 | ✅ | 罕见 | 罕见 |
| 协议互转（Anthropic ↔ Chat ↔ Responses） | ✅ | ❌ | ❌ |
| 本地 `lpk_...` API Key（落盘只存散列） | ✅ | ❌ | ❌ |
| 纯 Go 构建，无 CGO，单静态二进制 | ✅ | 视情况 | 视情况 |
| 默认仅本机，零公网面 | ✅ | 视情况 | 视情况 |

## 功能

- 通过 CLI 执行 Codex 和 GitHub Copilot 的 OAuth 登录。Codex 支持 Browser OAuth 和 device-code 登录；Copilot 使用 device-code 登录。
- 使用本地 `lpk_...` API Key 绑定 OAuth 会话。
- 提供 Anthropic-compatible 和 OpenAI-compatible HTTP 端点。
- 在 Anthropic Messages、OpenAI Chat Completions、OpenAI Responses 之间转换请求和响应。
- 支持兼容上游的流式请求。
- 本地 token 使用受限文件权限保存。
- API Key 只保存 SHA-256 哈希；明文 key 仅在创建时显示一次。
- 不提供 HTTP 登录或 HTTP Key 管理接口。

## 安全模型

`llm-proxy` 设计为本地网关，不是公网服务。

- 默认监听 `127.0.0.1:15721`。
- OAuth 登录和 API Key 创建只通过 `llm-proxy login` 或 `llm-proxy keys` CLI 完成。
- HTTP 服务只暴露健康检查和模型代理端点。
- 除非完全控制网络环境，否则不要绑定到公网或不可信网卡。
- 本地 `lpk_...` API Key 应视为等同于访问底层 OAuth 会话。
- 明文 API Key 不会保存。如果丢失，重新运行 `llm-proxy login` 或 `llm-proxy keys create` 创建新的 key。
- 绑定到非 localhost 地址时，服务会打印警告。除非网络完全可信，否则不要使用这种模式。
- 不要使用本项目转售、池化或公开共享 provider 账号访问能力。

## 环境要求

- Go 1.25+
- 能访问相关上游服务：
  - `auth.openai.com`
  - `chatgpt.com`
  - `github.com`
  - `api.github.com`

如果已安装 Go 但不在 `PATH` 中，可先设置：

```bash
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
```

## 安装

### 使用 curl 安装

```bash
curl -fsSL https://raw.githubusercontent.com/HaaapyDay/llm-proxy/main/install.sh | sh
```

安装脚本会从 GitHub Releases 下载预编译二进制，使用 `checksums.txt` 校验，
并安装到 `~/.local/bin`。这种方式不依赖 Go。如果 `~/.local/bin` 不在
`PATH` 中，脚本会在交互式终端里先询问，再决定是否更新 shell 配置。

### 使用 Go 安装

```bash
go install github.com/HaaapyDay/llm-proxy/cmd/llm-proxy@latest
```

### 从 GitHub Releases 安装

从以下地址下载对应平台的压缩包：

```text
https://github.com/HaaapyDay/llm-proxy/releases
```

使用发布的 `checksums.txt` 校验压缩包，解压后将 `llm-proxy` 放到 `PATH` 中。

### 从源码构建

```bash
git clone https://github.com/HaaapyDay/llm-proxy.git
cd llm-proxy
go build -o bin/llm-proxy ./cmd/llm-proxy
```

`llm-proxy` 使用纯 Go SQLite driver，普通源码构建不需要 C 编译器。

## 快速开始

登录一个 provider。命令会完成 OAuth，并创建本地 API Key。

```bash
./bin/llm-proxy login codex
# 或
./bin/llm-proxy login copilot
```

`login codex` 会让你选择 Browser OAuth 或 Device code。Browser OAuth 是默认选项，会打印授权 URL，并等待 localhost 回调。脚本或无头环境可以显式选择：

```bash
./bin/llm-proxy login codex --browser
./bin/llm-proxy login codex --device-code
```

Copilot 登录保持 device-code flow。在无图形界面或远程机器上，可以跳过 Copilot 的浏览器自动打开：

```bash
./bin/llm-proxy login copilot --no-browser
```

授权完成后，命令会输出类似环境变量：

```bash
export LLM_PROXY_API_KEY=lpk_...
export ANTHROPIC_BASE_URL=http://127.0.0.1:15721
export ANTHROPIC_AUTH_TOKEN=lpk_...
export OPENAI_BASE_URL=http://127.0.0.1:15721/v1
export OPENAI_API_KEY=lpk_...
```

启动网关：

```bash
./bin/llm-proxy serve
```

仅在可信本地网络中使用自定义地址：

```bash
./bin/llm-proxy serve --host 127.0.0.1 --port 15721
```

## 客户端配置

更多 SDK 和工具配置方式见 [客户端配置示例](docs/clients.md)。

### Anthropic-Compatible 客户端

对使用 Anthropic Messages API 的工具，将 Anthropic base URL 指向本地网关，并使用生成的 `lpk_...` key。

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:15721
export ANTHROPIC_AUTH_TOKEN=lpk_xxxx
```

请求示例：

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

### OpenAI-Compatible 客户端

对 OpenAI SDK 或兼容工具：

```bash
export OPENAI_BASE_URL=http://127.0.0.1:15721/v1
export OPENAI_API_KEY=lpk_xxxx
```

请求示例：

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

使用 `langchain-openai` 的 Python 示例：

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

## API 参考

### 公开端点

| Method | Path | 说明 |
| --- | --- | --- |
| `GET` | `/health` | 健康检查，不需要 API Key。 |

### 代理端点

所有代理端点都需要 API Key：

```http
Authorization: Bearer lpk_...
```

对于无法设置 Bearer token 的客户端，也支持：

```http
x-api-key: lpk_...
```

| Method | Path | 说明 |
| --- | --- | --- |
| `GET` | `/v1/models` | OpenAI-compatible 模型列表端点。 |
| `POST` | `/v1/messages` | Anthropic Messages-compatible 端点。 |
| `POST` | `/v1/chat/completions` | OpenAI Chat Completions-compatible 端点。 |
| `POST` | `/v1/responses` | OpenAI Responses-compatible 端点。 |

## 兼容性

当需要在 Anthropic Messages、OpenAI Chat Completions、OpenAI Responses 之间转换时，代理会使用共享的中间表示。

已支持的跨格式能力包括：

- 支持客户端通过 `/v1/models` 获取 OpenAI-compatible 模型列表。
- 文本消息和 system/developer instructions。
- 目标格式支持图片内容时的图片输入。
- 请求保持在 Responses-compatible 路径时的 OpenAI Responses 文件输入。
- Function tools、常见 `tool_choice` 模式、tool calls 和 tool results。
- 上游接受时的基础采样参数，例如 `temperature`、`top_p` 和 max token 字段。
- 流式文本 delta 和 function tool call arguments delta。

不支持的能力会返回结构化 `400` 错误，而不是静默丢弃。包括从 OpenAI Chat 转出时的音频、转换到 Anthropic 或 Chat 时的文件输入、非 Responses 目标上的 hosted/built-in Responses tools、非 Responses 目标上的 `previous_response_id` 等 response state，以及目标协议无法表达的 reasoning/thinking blocks。

端点行为、provider 差异和兼容性问题提交方式见 [Compatibility Matrix](docs/compatibility.md)。

## CLI 参考

```bash
llm-proxy serve [--host 127.0.0.1] [--port 15721] [--data-dir ~/.llm-proxy]
llm-proxy login codex [--browser|--device-code]
llm-proxy login copilot [--no-browser]
llm-proxy keys list
llm-proxy keys create codex|copilot [--label NAME]
llm-proxy keys delete KEY_ID
llm-proxy doctor
llm-proxy version
```

### `serve`

启动本地 HTTP 网关。

排查上游错误时可以开启 debug 日志：

```bash
LLM_PROXY_DEBUG=1 llm-proxy serve
```

debug 日志会写到 stderr，包含上游 URL、provider 路径、模型、状态码、耗时，以及截断后的上游错误预览。不会记录 API Key、OAuth token 或完整请求体。

### `login`

为 `codex` 或 `copilot` 执行 OAuth 登录，保存本地 OAuth 会话，并创建本地 API Key。`codex` 可选择 Browser OAuth 或 device-code；`copilot` 保持 device-code。

明文 API Key 只打印一次。持久化 key store 只保存 SHA-256 哈希和元数据。

### `keys`

管理本地 API Key，不暴露 HTTP Key 管理接口。

```bash
llm-proxy keys list
llm-proxy keys create codex --label work
llm-proxy keys delete KEY_ID
```

`keys list` 只显示 active key 的元数据和预览。`keys create` 要求对应 provider 已登录，并且只打印一次明文 API Key。`keys delete` 会在本地吊销 key。

### `doctor`

检查本地配置、数据目录权限、API Key 元数据和本地 auth 文件可解析性。该命令不会发起网络请求。

### `version`

输出版本号、commit 和构建时间。GitHub Releases 发布的二进制会在构建时写入这些字段。

## 数据目录

运行时数据默认保存在 `~/.llm-proxy/`。目录使用 `0700` 权限创建，文件使用 `0600` 权限写入。

| 文件 | 说明 |
| --- | --- |
| `codex_oauth_auth.json` | Codex OAuth refresh token 存储。 |
| `copilot_auth.json` | GitHub 和 Copilot token 存储。 |
| `llm-proxy.db` | SQLite 数据库，保存本地 `lpk_...` API Key 的 SHA-256 哈希和元数据。 |
| `api_keys.json` | 旧版 API Key 存储。如果存在，会自动导入并保留原文件。 |

不要提交或共享该目录。

## 开发

```bash
go test ./...
go vet ./...
go build -o bin/llm-proxy ./cmd/llm-proxy
```

维护者约定见 [Maintenance Notes](docs/maintenance.md)。

推送 `v*` tag 时由 GoReleaser 生成发布产物。可以用以下命令在本地检查 snapshot 发布配置：

```bash
goreleaser release --snapshot --clean
```

完整发布检查清单见 [Release Process](docs/release.md)。

## 限制

- 仅面向本地使用。
- GitHub Copilot 支持目前面向 `github.com`，不支持 GitHub Enterprise Server。
- 上游可用性和模型访问权限取决于已认证账号。
- provider API 变化时，代理可能需要同步更新。
- 支持的协议转换和已知不支持能力见 [Compatibility Matrix](docs/compatibility.md)。
