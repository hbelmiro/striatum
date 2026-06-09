# Striatum

OCI-native CLI and library for packaging, versioning, and distributing AI artifacts (skills, prompts, workflows) using standard OCI-compliant registries.

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

- `striatum init` -- scaffold an `artifact.json` (requires `--name`, `--kind` (Skill, Prompt, Workflow), `--entrypoint`)
- `striatum validate` -- validate local artifact and optionally check dependencies
- `striatum pack` -- bundle artifact into a local OCI Image Layout at `<project>/build/` by default; optional `-o` / `--output` sets another layout directory. For Workflow artifacts with Prompt dependencies, pack resolves and inlines the prompt files as extra OCI layers under `deps/<prompt-name>/`; missing prompt dependencies are fetched automatically.
- `striatum push <reference>` -- push to an OCI registry
- `striatum pull <reference>` -- download artifact and dependencies to the output directory (default: current working directory; each artifact in `<output>/<name>/`) and into the Striatum cache; use `--no-cache` for output only
- `striatum inspect <reference>` -- show remote artifact metadata
- `striatum install <reference>` -- install an artifact into Cursor/Claude directories (kind auto-detected from manifest; Workflow installs to `~/.claude/workflows/`, Skill installs to skills directories)
- `striatum uninstall <name>` -- remove an installed artifact
- `striatum list` -- list artifacts in local cache (all kinds, with KIND column); use `--installed` to list installed artifacts (optional `--target cursor|claude`)

### Usage examples

```bash
striatum init --name my-skill --kind Skill --entrypoint SKILL.md
striatum init --name severity-rubric --kind Prompt --entrypoint severity-rubric.md
striatum init --name thorough-review --kind Workflow --entrypoint review.js
striatum validate
striatum validate -f packages/my-skill
striatum validate --check-deps --registry localhost:5000/skills
striatum pack
striatum pack --manifest path/to/artifact.json
striatum pack -o ./dist
striatum push localhost:5000/skills/my-skill:1.0.0
striatum push -f ./my-skill localhost:5000/skills/my-skill:1.0.0
striatum pull localhost:5000/skills/my-skill:1.0.0
striatum install --target cursor localhost:5000/skills/my-skill:1.0.0
striatum install --target claude localhost:5000/workflows/thorough-review:1.0.0
striatum uninstall --target cursor my-skill
striatum inspect localhost:5000/skills/my-skill:1.0.0
striatum list
striatum list --installed --target cursor
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
