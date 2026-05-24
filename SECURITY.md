# Security Policy

`llm-proxy` is designed as a local development gateway. It is not intended to be exposed as a public service.

## Supported Versions

Security fixes target the latest released version and the `main` branch.

## Reporting a Vulnerability

Please report suspected vulnerabilities privately to the repository maintainers. If GitHub private vulnerability reporting is enabled, use that channel. Otherwise, contact the maintainers through the email or security contact listed on the repository profile.

Do not include OAuth refresh tokens, local `lpk_...` API keys, or full request payloads unless a maintainer explicitly asks for sanitized details.

## Security Model

- The server listens on `127.0.0.1:15721` by default.
- OAuth login and local API key creation are CLI-only.
- The HTTP server exposes only health and model proxy endpoints.
- Local `lpk_...` keys grant access equivalent to the underlying OAuth session.
- Plaintext local API keys are printed only once and are not persisted.
- Runtime data is stored under `~/.llm-proxy/` by default with restrictive file permissions.
- Debug logs are intended for local troubleshooting and must not include API keys, OAuth tokens, or full request bodies.

## Not Intended For

- Public internet deployment.
- Shared hosted API access.
- Account pooling or resale.
- Bypassing provider limits, access controls, account restrictions, or terms.

## User Responsibilities

- Do not bind the proxy to a public or untrusted network interface.
- Do not share local API keys or files from `~/.llm-proxy/`.
- Revoke local keys with `llm-proxy keys delete KEY_ID` if a key is exposed.
- Follow the terms and policies of OpenAI Codex, GitHub Copilot, and GitHub when using this project.
- Sanitize issue reports and remove tokens, local keys, account identifiers, private prompts, file IDs, and full sensitive payloads.

This project does not attempt to bypass account limits, access controls, or provider policies.
