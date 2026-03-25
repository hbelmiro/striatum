# Agent instructions for Striatum

Use this file as context when working on this repository.

## Project

**Striatum** is an OCI-native CLI and library (Go) for packaging, versioning, and distributing AI artifacts (skills, prompts, RAG configs) using OCI-compliant registries. See [README.md](README.md) for the full spec.

## Layout

- **`cmd/striatum/`** — CLI entrypoint.
- **`internal/cli/`** — Cobra commands and CLI-only logic (init, validate, pack, push, pull, install, uninstall, inspect, skill list).
- **`pkg/`** — Reusable packages: `oci` (pack/push/pull/inspect), `artifact` (manifest), `resolver` (dependencies), `installer` (cache and install targets).

## Build and test

```bash
go build ./cmd/striatum
go test -v ./...
```

Integration tests (need Docker for a local registry):

```bash
go test -tags=integration ./...
```

## Conventions

- **Go**: Prefer standard library and existing deps (`oras.land/oras-go/v2`, `github.com/spf13/cobra`, `github.com/opencontainers/image-spec`).
- **CLI**: Subcommands and flags live in `internal/cli/`; registry and artifact logic in `pkg/`. For validate, pack, and push, `-f` / `--manifest` selects a manifest **file** only when the basename is `artifact.json` (case-insensitive); any other basename is treated as a project directory and `artifact.json` is appended.
- **Docs**: [README.md](README.md) is the source of truth for behavior; [docs/demo.md](docs/demo.md) for end-to-end flows.

## Commands (reference)

| Command | Purpose |
| ------- | ------- |
| `striatum init` | Scaffold `artifact.json` (requires `--name`, `--kind`, `--entrypoint`) |
| `striatum validate` | Validate artifact (optional `--check-deps`; `-f` / `--manifest` for non-CWD `artifact.json`) |
| `striatum pack` | Bundle into local OCI Image Layout at `<project>/build/` by default; `-o` / `--output` for another path (`-f` / `--manifest` supported) |
| `striatum push <ref>` | Push to OCI registry (`-f` / `--manifest` supported) |
| `striatum pull <ref>` | Pull artifact and deps |
| `striatum inspect <ref>` | Show remote artifact metadata |
| `striatum skill install <ref>` | Install skill into Cursor/Claude skills dirs |
| `striatum skill uninstall <name>` | Remove installed skill |
| `striatum skill list` | List cached skills; use `--installed --target cursor` or `--target claude` for installed |
