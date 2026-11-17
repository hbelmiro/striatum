# Striatum examples

This directory contains worked examples for striatum. Each example is a small, self-contained artifact plus a README that walks through the full lifecycle: validate, pack, push to a registry, pull, inspect, install, and uninstall.

Start with `01-skill-basic` if you are new to striatum. It covers the core workflow with a single Skill artifact and no dependencies. The later examples build on it, adding dependencies, digest pinning, and version updates.

| Example                                              | Demonstrates                                                   |
|------------------------------------------------------|----------------------------------------------------------------|
| [01-skill-basic](01-skill-basic)                     | The core lifecycle with a single Skill artifact                |
| [02-prompt-basic](02-prompt-basic)                   | Packaging and distributing a Prompt artifact                   |
| [03-workflow-claude](03-workflow-claude)             | A Workflow artifact that installs only with the Claude target  |
| [04-memory](04-memory)                               | A Memory artifact that merges into project memory indexes      |
| [05-oci-dependencies](05-oci-dependencies)           | Resolving dependencies stored in an OCI registry               |
| [06-git-dependency](06-git-dependency)               | Mixing a Git-hosted dependency into the dependency graph       |
| [07-self-contained](07-self-contained)               | Self-contained images that bundle the dependency closure       |
| [08-digest-pinning](08-digest-pinning)               | Pinning dependency references to immutable digests             |
| [09-update-versions](09-update-versions)             | Publishing new versions and updating installed artifacts       |
| [10-mlflow-skill-registry](10-mlflow-skill-registry) | A Skill that wraps MLflow model registry operations            |

## Running the example tests locally

The examples are covered by integration tests. To run them on your machine:

```bash
go test -tags=integration ./examples/...
```

The tests require Docker because they spin up a local OCI registry. When Docker is not available, the tests fail rather than skip, so a broken environment is never mistaken for a passing run. To reuse a registry you already have running, set `STRIATUM_EXAMPLES_REGISTRY` to its repository prefix (for example `localhost:5000/examples`).

The MLflow example additionally needs the `mlflow` CLI on PATH. Install the pinned version used by CI into a local venv:

```bash
uv sync --frozen --project examples/docker
PATH="examples/docker/.venv/bin:$PATH" go test -tags=integration ./examples/...
```

For a scripted end-to-end tour of the CLI, including dependency resolution across OCI and Git sources, see [the demo walkthrough](../docs/demo.md).
