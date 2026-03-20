# Striatum

OCI-native CLI and library for packaging, versioning, and distributing AI artifacts (skills, prompts, RAG configs) using standard OCI-compliant registries.

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

- `striatum init` — scaffold an `artifact.json` (requires `--name`, `--kind`, `--entrypoint`)
- `striatum validate` — validate local artifact and optionally check dependencies
- `striatum pack` — bundle artifact into a local OCI Image Layout
- `striatum push <reference>` — push to an OCI registry
- `striatum pull <reference>` — download artifact and dependencies to the output directory (default `./<name>/`) and, by default, into the Striatum cache (`STRIATUM_HOME` or `~/.striatum/cache`) so `skill list` can see them; use `--no-cache` for output only
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
striatum push localhost:5000/skills/my-skill:1.0.0
striatum push -f ./my-skill localhost:5000/skills/my-skill:1.0.0
striatum pull localhost:5000/skills/my-skill:1.0.0
striatum skill install --target cursor localhost:5000/skills/my-skill:1.0.0
striatum skill uninstall --target cursor my-skill
striatum inspect localhost:5000/skills/my-skill:1.0.0
striatum skill list
striatum skill list --installed --target cursor
```

See [docs/MVP.md](docs/MVP.md) for the full specification and [docs/demo.md](docs/demo.md) for a full-flow demo (pack, push, pull, install, uninstall).

## License

Apache-2.0. See [LICENSE](LICENSE).
