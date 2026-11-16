# Agent instructions for Striatum

Use this file as context when working on this repository.

## Project

**Striatum** is an OCI-native CLI and library (Go) for packaging, versioning, and distributing AI artifacts (skills, prompts, RAG configs) using OCI-compliant registries. See [README.md](README.md) for the full spec.

## Layout

- **`cmd/striatum/`** — CLI entrypoint.
- **`internal/cli/`** — Cobra commands and CLI-only logic (init, validate, pack, push, pull, install, uninstall, inspect, list).
- **`pkg/`** — Reusable packages: `oci` (pack/push/pull/inspect), `artifact` (manifest), `resolver` (dependencies), `installer` (cache and install targets), `selfcontained` (self-contained image preparation: safe materialization, Markdown link rewriting, provenance, consumer-constraint checks).

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
- **Cache**: Artifacts are cached at `~/.striatum/cache/<kind>/<name>@<version>/` (e.g. `cache/Skill/my-skill@1.0.0/`). `FindCacheDir(name, version)` probes all kind subdirectories when the caller does not know the kind; returns an error if multiple kinds match. Self-contained image payloads are cached separately under `~/.striatum/cache/self-contained/<name>@<version>/` and never mix with ordinary kind caches.
- **Docs**: [README.md](README.md) is the source of truth for behavior; [docs/demo.md](docs/demo.md) for end-to-end flows.

## Commands (reference)

| Command                     | Purpose                                                                                                                                                                                                                               |
|-----------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `striatum init`             | Scaffold `artifact.json` (requires `--name`, `--kind`, `--entrypoint`)                                                                                                                                                                |
| `striatum validate`         | Validate artifact (optional `--check-deps`; `-f` / `--manifest` for non-CWD `artifact.json`)                                                                                                                                          |
| `striatum pack`             | Bundle into local OCI Image Layout at `<project>/build/` by default; `-o` / `--output` for another path; `--self-contained` bundles the Skill with its full dependency closure (`-f` / `--manifest` supported)                        |
| `striatum push <ref>`       | Push to OCI registry; `--self-contained` publishes the closure-bundled image; `--print-digest` prints one canonical `repo@sha256:...` line for automation (`-f` / `--manifest` supported)                                             |
| `striatum pull <ref>`       | Pull artifact and deps                                                                                                                                                                                                                |
| `striatum inspect <ref>`    | Show remote artifact metadata                                                                                                                                                                                                         |
| `striatum install <ref>`    | Install artifact into Cursor/Claude directories (kind auto-detected; Workflow to `~/.claude/workflows/`, Skill to skills dirs). Accepts digest-only refs; self-contained images install as one unit without re-resolving dependencies |
| `striatum uninstall <name>` | Remove installed artifact; `--kind` to disambiguate when multiple kinds share the same name                                                                                                                                           |
| `striatum list`             | List cached artifacts (all kinds); use `--installed --target cursor` or `--target claude` for installed                                                                                                                               |
