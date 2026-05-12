# Striatum

OCI-native CLI and library for packaging, versioning, and distributing AI artifacts (skills, prompts, RAG configs) using standard OCI-compliant registries.

## Install

### Homebrew (macOS / Linux)

```bash
brew tap hbelmiro/striatum https://github.com/hbelmiro/striatum
brew install striatum
```

### Binary releases

Download the latest binary from [GitHub Releases](https://github.com/hbelmiro/striatum/releases).

### From source

```bash
go install github.com/hbelmiro/striatum/cmd/striatum@latest
```

## Build

```bash
go build ./cmd/striatum
```

## Test

```bash
go test -v ./...
```

To run integration tests (requires Docker for the local registry):

```bash
go test -tags=integration ./...
```

## Commands

For `validate`, `pack`, and `push`, use `-f` / `--manifest` with a path to `artifact.json` or to the project directory that contains it (defaults to `./artifact.json` in the current working directory). Paths in `spec.files` are always resolved relative to the directory that contains the manifest, not the shell’s current directory.

The flag treats a path as a manifest **file** only when its final component is named `artifact.json` (case-insensitive, e.g. `Artifact.json` on disk). Any other final component is treated as a **project directory** and `artifact.json` is appended—even if that name is missing—so typos get errors pointing at the expected manifest path.

- `striatum init` — scaffold an `artifact.json` (requires `--name`, `--kind`, `--entrypoint`)
- `striatum validate` — validate local artifact and optionally check dependencies
- `striatum pack` — bundle artifact into a local OCI Image Layout at `<project>/build/` by default (the manifest’s project directory, not necessarily the shell’s cwd—see `-f` / `--manifest`); optional `-o` / `--output` sets another layout directory (paths relative to the shell’s cwd, like `pull --output`)
- `striatum push <reference>` — push to an OCI registry
- `striatum pull <reference>` — download artifact and dependencies to the output directory (default: current working directory; each artifact in `<output>/<name>/`) and, by default, into the Striatum cache (`STRIATUM_HOME` or `~/.striatum/cache`) so `skill list` can see them; use `--no-cache` for output only
- `striatum inspect <reference>` — show remote artifact metadata
- `striatum skill install <reference>` — install a skill into Cursor/Claude skills directories
- `striatum skill uninstall <name>` — remove an installed skill
- `striatum skill list` — list skills in local cache; use `--installed` to list installed skills (optional `--target cursor|claude`)

### Usage examples

```bash
striatum init --name my-skill --kind Skill --entrypoint SKILL.md
striatum validate
striatum validate -f packages/my-skill
striatum validate --check-deps --registry localhost:5000/skills
striatum pack
striatum pack --manifest path/to/artifact.json
striatum pack -o ./dist
striatum push localhost:5000/skills/my-skill:1.0.0
striatum push -f ./my-skill localhost:5000/skills/my-skill:1.0.0
striatum pull localhost:5000/skills/my-skill:1.0.0
striatum skill install --target cursor localhost:5000/skills/my-skill:1.0.0
striatum skill uninstall --target cursor my-skill
striatum inspect localhost:5000/skills/my-skill:1.0.0
striatum skill list
striatum skill list --installed --target cursor
```

See [docs/demo.md](docs/demo.md) for a full-flow demo (pack, push, pull, install, uninstall).

## Releasing

Releases are automated with [GoReleaser](https://goreleaser.com/) via GitHub Actions.

```bash
git tag v0.2.0
git push origin v0.2.0
```

This triggers the release workflow, which:

1. Runs tests
2. Builds binaries for macOS and Linux (amd64 + arm64)
3. Creates a GitHub Release with archives and checksums
4. Commits an updated Homebrew formula to `HomebrewFormula/`

## License

Apache-2.0. See [LICENSE](LICENSE).
