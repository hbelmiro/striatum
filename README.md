# Striatum

OCI-native CLI and library for packaging, versioning, and distributing AI artifacts (skills, prompts, RAG configs) using standard OCI-compliant registries.

## Build

```bash
go build ./cmd/striatum
```

## Test

```bash
go test -v -race ./...
```

## Commands

- `striatum init` — scaffold an `artifact.json`
- `striatum validate` — validate local artifact and optionally check dependencies
- `striatum pack` — bundle artifact into a local OCI Image Layout
- `striatum push <reference>` — push to an OCI registry
- `striatum pull <reference>` — download artifact and dependencies
- `striatum install <reference>` — install into Cursor/Claude skills directories
- `striatum uninstall <name>` — remove an installed skill
- `striatum inspect <reference>` — show remote artifact metadata

See [docs/MVP.md](docs/MVP.md) for the full specification.

## License

Apache-2.0. See [LICENSE](LICENSE).
