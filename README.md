# Striatum

OCI-native CLI for packaging, versioning, and distributing AI artifacts with dependency management, using standard container registries.

Striatum gives skills, prompts, and workflows the same publish/pull/install lifecycle that container images already have, with full dependency resolution across the artifact tree. You author an `artifact.json` manifest, declare dependencies (OCI or Git), pack the files into an OCI image, push to any compliant registry, and install into Claude Code or Cursor with a single command. Striatum resolves transitive dependencies, detects version conflicts, and fetches anything missing from the local cache.

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

## Quickstart

```bash
# Scaffold a new skill
striatum init --name my-skill --kind Skill --entrypoint SKILL.md

# Edit SKILL.md, then pack and push
striatum pack
striatum push quay.io/my-org/my-skill:1.0.0

# Install into Claude Code
striatum install --target claude quay.io/my-org/my-skill:1.0.0
```

See [docs/demo.md](docs/demo.md) for a full walkthrough with dependency resolution, Git dependencies, and install/uninstall flows.

## Artifact manifest

Every artifact has an `artifact.json` at its root. A minimal manifest looks like this:

```json
{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {
    "name": "my-skill",
    "version": "1.0.0"
  },
  "spec": {
    "entrypoint": "SKILL.md",
    "files": ["SKILL.md"]
  }
}
```

Artifacts can declare OCI and Git dependencies. During `pack` and `pull`, Striatum resolves the full dependency tree, detects version conflicts, and fetches anything missing from the local cache.

```json
{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {
    "name": "my-skill",
    "version": "1.0.0",
    "description": "A skill with dependencies",
    "authors": ["alice"],
    "license": "Apache-2.0",
    "tags": ["example"]
  },
  "spec": {
    "entrypoint": "SKILL.md",
    "files": ["SKILL.md"]
  },
  "dependencies": [
    {
      "source": "oci",
      "registry": "quay.io",
      "repository": "my-org/helper-skill",
      "tag": "1.0.0",
      "digest": "sha256:abc123..."
    },
    {
      "source": "git",
      "url": "https://github.com/example/skill.git",
      "ref": "v1.0.0",
      "path": "skills/helper",
      "commit": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
    }
  ]
}
```

| Field                   | Description                                        |
| ----------------------- | -------------------------------------------------- |
| `apiVersion`            | Schema version (currently `striatum.dev/v1alpha2`) |
| `kind`                  | `Skill`, `Prompt`, or `Workflow`                   |
| `metadata.name`         | Artifact name (required)                           |
| `metadata.version`      | Semver version (required)                          |
| `metadata.description`  | Short description                                  |
| `metadata.authors`      | List of author names                               |
| `metadata.license`      | SPDX license identifier                            |
| `metadata.tags`         | Free-form tags                                     |
| `spec.entrypoint`       | Primary file (must appear in `files`)              |
| `spec.files`            | All files to bundle                                |
| `dependencies[].source` | `oci` or `git`                                     |

OCI dependencies use `registry`, `repository`, `tag`, and an optional `digest` for pinning. Git dependencies use `url`, `ref`, an optional `path` (subdirectory), and an optional `commit` SHA for pinning.

## Commands

For `validate`, `pack`, and `push`, the `-f` / `--manifest` flag accepts a path to `artifact.json` or to the project directory that contains it (defaults to `./artifact.json` in the current working directory). Paths in `spec.files` are resolved relative to the directory that contains the manifest.

### init

Scaffold an `artifact.json` in the current directory.

```bash
striatum init --name my-skill --kind Skill --entrypoint SKILL.md
striatum init --name severity-rubric --kind Prompt --entrypoint severity-rubric.md
striatum init --name thorough-review --kind Workflow --entrypoint review.js
```

| Flag           | Description                                  |
| -------------- | -------------------------------------------- |
| `--name`       | Artifact name (required)                     |
| `--kind`       | `Skill`, `Prompt`, or `Workflow` (required)  |
| `--entrypoint` | Entrypoint file (required)                   |
| `--version`    | Artifact version (default `0.1.0`)           |

### validate

Validate the manifest schema and check that all `spec.files` exist. Use `--check-deps` to verify that dependencies resolve from their declared sources.

```bash
striatum validate
striatum validate -f packages/my-skill
striatum validate --check-deps
```

| Flag               | Description                                                |
| ------------------ | ---------------------------------------------------------- |
| `-f`, `--manifest` | Path to manifest file or project directory                 |
| `--check-deps`     | Verify all dependencies exist in their declared registries |

### pack

Bundle the artifact into a local OCI Image Layout directory. The default output is `<project>/build/`; override with `-o`.

For Workflow artifacts with Prompt dependencies, `pack` resolves and inlines the prompt files as extra OCI layers under `deps/<prompt-name>/`.

```bash
striatum pack
striatum pack -f packages/my-skill
striatum pack -o ./dist
```

| Flag               | Description                                |
| ------------------ | ------------------------------------------ |
| `-f`, `--manifest` | Path to manifest file or project directory |
| `-o`, `--output`   | OCI layout output directory                |

### push

Pack and push the artifact to an OCI registry.

```bash
striatum push quay.io/my-org/my-skill:1.0.0
striatum push -f ./my-skill quay.io/my-org/my-skill:1.0.0
```

| Flag               | Description                                |
| ------------------ | ------------------------------------------ |
| `-f`, `--manifest` | Path to manifest file or project directory |

### pull

Download an artifact and its transitive dependencies to the output directory (default: current working directory). Each artifact is placed in `<output>/<name>/`. Git dependencies declared in `artifact.json` are resolved automatically.

By default, pulled artifacts are also stored in the Striatum cache (`~/.striatum/cache`). Use `--no-cache` to write only to the output directory.

```bash
striatum pull quay.io/my-org/my-skill:1.0.0
striatum pull -o ./out oci:./build:my-skill:1.0.0
```

| Flag             | Description                        |
| ---------------- | ---------------------------------- |
| `-o`, `--output` | Output directory                   |
| `--no-cache`     | Skip writing to the Striatum cache |

### inspect

Display the artifact manifest and metadata of a remote artifact without downloading layers.

```bash
striatum inspect quay.io/my-org/my-skill:1.0.0
striatum inspect git:https://github.com/example/skill.git@v1.0.0
```

### install

Pull and install an artifact into the appropriate target directory. The artifact kind is auto-detected from the manifest. Accepts a registry reference, `oci:/path:tag`, or a local directory path.

Workflows install to `~/.claude/workflows/<name>/`, Skills and Prompts to the target's skills or prompts directory.

```bash
striatum install --target claude quay.io/my-org/my-skill:1.0.0
striatum install --target cursor quay.io/my-org/my-skill:1.0.0
striatum install --target claude --project . quay.io/my-org/my-workflow:1.0.0
```

| Flag              | Description                                     |
| ----------------- | ----------------------------------------------- |
| `-t`, `--target`  | Install target: `cursor` or `claude` (required) |
| `--project`       | Project path for project-level install          |
| `--force`         | Overwrite conflicting versions                  |
| `--reinstall-all` | Replay all entries from the install database    |

### uninstall

Remove an installed artifact and any dependencies that are no longer required by other installed artifacts.

```bash
striatum uninstall --target claude my-skill
striatum uninstall --target claude --kind Workflow my-workflow
```

| Flag             | Description                                                            |
| ---------------- | ---------------------------------------------------------------------- |
| `-t`, `--target` | Uninstall from target: `cursor` or `claude` (required)                 |
| `-k`, `--kind`   | Artifact kind filter; required when multiple kinds share the same name |
| `--project`      | Project path (match project-level install)                             |

### list

List artifacts in the local cache. Use `--installed` to list installed artifacts instead.

```bash
striatum list
striatum list --installed --target claude
striatum list --installed --project .
```

| Flag             | Description                                                      |
| ---------------- | ---------------------------------------------------------------- |
| `--installed`    | List installed artifacts instead of cache                        |
| `-t`, `--target` | Filter by target (`cursor` or `claude`); only with `--installed` |
| `--project`      | Filter by project path; only with `--installed`                  |

### update

Check for newer versions of installed artifacts and upgrade them in place. Without arguments, updates all installed artifacts. With arguments, updates only the named artifacts.

```bash
striatum update
striatum update my-skill
striatum update --check
striatum update --target claude
striatum update --yes
```

| Flag             | Description                                |
|------------------|--------------------------------------------|
| `--check`        | List outdated artifacts without installing |
| `-t`, `--target` | Filter by target (`cursor` or `claude`)    |
| `--project`      | Filter by project path                     |
| `-y`, `--yes`    | Skip confirmation prompt                   |

## Supported registries

Striatum works with any OCI-compliant container registry. Tested with:

- [Quay.io](https://quay.io)
- [GitHub Container Registry (ghcr.io)](https://ghcr.io)
- [Docker Hub](https://hub.docker.com)

Authentication uses your Docker config (`~/.docker/config.json` or `$DOCKER_CONFIG`). Localhost registries (`localhost`, `127.0.0.1`, `::1`) are automatically accessed over plain HTTP.

## Workflow artifacts

Workflow artifacts are [Claude Code workflow](https://docs.anthropic.com/en/docs/claude-code/workflows) scripts that orchestrate multiple agents. When a Workflow declares Prompt dependencies, `striatum pack` resolves them and bundles the prompt files as extra OCI layers under `deps/<prompt-name>/`. After install, the layout on disk looks like this:

```text
~/.claude/workflows/thorough-review/
  review.js
  deps/
    severity-rubric/
      severity-rubric.md
```

Example flow:

```bash
striatum init --name thorough-review --kind Workflow --entrypoint review.js
# add a Prompt dependency to artifact.json, then:
striatum pack
striatum push quay.io/my-org/thorough-review:1.0.0
striatum install --target claude quay.io/my-org/thorough-review:1.0.0
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for build, test, and release instructions.

## License

Apache-2.0. See [LICENSE](LICENSE).
