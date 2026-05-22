# llm-proxy

本地 OAuth 网关：通过 **API Key** 代理访问 **OpenAI Codex** 与 **GitHub Copilot**，并在 Anthropic Messages、OpenAI Chat Completions、OpenAI Responses 之间转换请求/响应格式。

默认监听 `http://127.0.0.1:15721`（与 [cc-switch](https://github.com) 本地代理端口一致）。

## 要求

- Go 1.22+
- 可访问 `auth.openai.com`、`github.com`、`api.github.com`、`chatgpt.com`

确保 `PATH` 包含 Go（若已写入 `~/.bashrc`）：

```bash
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
```

## 安装

```bash
cd /home/lotus/projects/llm-proxy
go build -o bin/llm-proxy ./cmd/llm-proxy
```

## 快速开始

### 1. OAuth 登录并获取 API Key

```bash
./bin/llm-proxy login codex
# 或
./bin/llm-proxy login copilot
```

按提示在浏览器完成授权后，终端会输出：

```bash
export LLM_PROXY_API_KEY=lpk_...
export ANTHROPIC_BASE_URL=http://127.0.0.1:15721
export ANTHROPIC_AUTH_TOKEN=lpk_...
```

### 2. 启动网关

```bash
./bin/llm-proxy serve
# 自定义端口: ./bin/llm-proxy serve --port 8080
```

### 3. 配置 Claude Code / Codex CLI

将客户端指向本地网关，并使用上一步的 `lpk_` 作为 Bearer Token（**不是** `PROXY_MANAGED`）：

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:15721
export ANTHROPIC_AUTH_TOKEN=lpk_xxxx
```

## API 概览

### 健康检查（无需 API Key）

- `GET /health`

### 认证（无需 API Key）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/auth/codex/device` | 启动 Codex Device Code |
| POST | `/api/v1/auth/codex/poll` | 轮询 Codex（body: `device_code`） |
| POST | `/api/v1/auth/copilot/device` | 启动 Copilot Device Code |
| POST | `/api/v1/auth/copilot/poll` | 轮询 Copilot |
| POST | `/api/v1/keys` | 为已登录账号创建 Key |
| GET | `/api/v1/keys` | 列出 Key（掩码） |
| DELETE | `/api/v1/keys/:id` | 吊销 Key |

### 代理（需要 `Authorization: Bearer lpk_...`）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/v1/messages` | Anthropic Messages（按 Key 绑定 provider 转发） |
| POST | `/v1/chat/completions` | OpenAI Chat Completions |
| POST | `/v1/responses` | OpenAI Responses |

### 示例：创建 API Key

```bash
curl -s http://127.0.0.1:15721/api/v1/keys \
  -H 'Content-Type: application/json' \
  -d '{"label":"dev","provider":"codex_oauth","account_id":"<account_id>"}'
```

## 数据目录

运行时数据保存在 `~/.llm-proxy/`（权限 `0700`/`0600`）：

- `codex_oauth_auth.json` — Codex refresh token（schema 与 cc-switch 兼容）
- `copilot_auth.json` — GitHub / Copilot token
- `api_keys.json` — API Key 的 SHA-256 哈希与元数据（**不存明文**）

**切勿**将上述文件提交到 Git 或暴露到公网。

## CLI

```bash
llm-proxy serve [--host 127.0.0.1] [--port 15721] [--data-dir ~/.llm-proxy]
llm-proxy login codex|copilot
llm-proxy doctor
```

## 安全说明

- 网关默认只绑定 `127.0.0.1`，依赖本机信任模型。
- 不要将服务暴露到公网；API Key 等同于本地 OAuth 会话能力。
- MVP 仅支持 `github.com` Copilot，不含 GHES。

## 架构

```
Client (Bearer lpk_*) → Gin → API Key 解析 → OAuth Token 刷新
  → 格式转换 (Anthropic ↔ OpenAI Chat/Responses)
  → Codex / Copilot 上游 API
```

实现参考同级项目 cc-switch（Rust）与 cc-switch-tui（Python transforms）。

## 开发

```bash
go test ./...
go build -o bin/llm-proxy ./cmd/llm-proxy
```
