# Contributing to Striatum

## Build

```bash
go build ./cmd/striatum
```

## Test

```bash
go test -v ./...
```

To run integration tests (requires Docker for a local registry):

```bash
go test -tags=integration ./...
```

## Releasing

Releases are automated with [GoReleaser](https://goreleaser.com/) via GitHub Actions.

```bash
git tag v0.4.0
git push origin v0.4.0
```

This triggers the release workflow, which:

1. Runs tests
2. Builds binaries for macOS and Linux (amd64 + arm64)
3. Creates a GitHub Release with archives and checksums
4. Commits an updated Homebrew formula to `HomebrewFormula/`
