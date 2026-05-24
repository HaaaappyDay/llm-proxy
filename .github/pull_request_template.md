## Summary

Describe the change and why it is needed.

## Testing

- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `go build -o bin/llm-proxy ./cmd/llm-proxy`
- [ ] `govulncheck ./...` if the change affects dependencies, auth, storage, networking, or release readiness.
- [ ] `goreleaser check` if the change affects release configuration.

## Checklist

- [ ] Documentation updated if commands, behavior, compatibility, or security posture changed.
- [ ] No OAuth tokens, local API keys, runtime data, or `.env` files are included.
- [ ] New behavior is covered by tests where practical.
- [ ] Compatibility changes update `docs/compatibility.md` or explain why no doc change is needed.
- [ ] Changes touching OAuth, local keys, headers, logs, storage, or errors have been reviewed for sensitive data exposure.
