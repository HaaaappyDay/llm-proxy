# Release Process

This project publishes release binaries through GitHub Releases and GoReleaser.

The target repository is:

```text
https://github.com/HaaapyDay/llm-proxy
```

## Versioning

Use semver-style tags with a `v` prefix.

The first public release is expected to be:

```text
v0.1.0
```

## Checklist

Before tagging a release:

- [ ] Confirm all repository links use `github.com/HaaapyDay/llm-proxy`.
- [ ] Confirm `README.md` and `README.zh-CN.md` describe the current commands, endpoints, and limitations.
- [ ] Confirm `CHANGELOG.md` has an entry for the target version.
- [ ] Confirm `SECURITY.md` has a valid private reporting path or repository-profile contact path.
- [ ] Confirm no runtime data, OAuth tokens, generated `lpk_...` keys, `.env` files, or local auth files are staged.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run `govulncheck ./...`.
- [ ] Run `goreleaser check`.
- [ ] Run `goreleaser release --snapshot --clean`.

## Commands

```bash
go test ./...
go vet ./...
govulncheck ./...
goreleaser check
goreleaser release --snapshot --clean
```

If `govulncheck` is not installed:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
```

## Tag And Publish

After the checklist passes:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Pushing a `v*` tag triggers the release workflow. GoReleaser builds platform
archives and publishes `checksums.txt` with the release.

## Post-Release

After GitHub Actions completes:

- Confirm the release page exists.
- Confirm archives are attached for Linux, macOS, and Windows.
- Confirm `checksums.txt` is attached.
- Download one archive and verify the binary prints version metadata:

```bash
llm-proxy version
```

