# Tutorial: packaging and distributing AI skills with Striatum

This walkthrough takes you through the full Striatum lifecycle — from creating an artifact, to pushing it to a registry, pulling it with dependency resolution, and installing it as a Cursor skill. By the end you will have:

- Packed and pushed artifacts to an OCI registry
- Pulled a skill with transitive **OCI** and **Git** dependencies
- Installed and uninstalled a skill in Cursor

## Prerequisites

- **Go** (version in `go.mod`)
- **Docker** (for a local OCI registry)
- Internet access (the Git dependency demo clones from GitHub)

Build the CLI from the repository root:

```bash
go build -o striatum ./cmd/striatum
```

## Demo artifacts

The `demo/` directory contains sample artifacts you can experiment with:

| Artifact                                                                         | Dependencies                                              | Stored in      |
|----------------------------------------------------------------------------------|-----------------------------------------------------------|----------------|
| `example-skill`                                                                  | `example-helper-a` (OCI), `example-helper-b` (OCI)        | OCI registry   |
| `example-helper-a`                                                               | none                                                      | OCI registry   |
| `example-helper-b`                                                               | none                                                      | OCI registry   |
| `example-skill-git`                                                              | `example-helper-a` (OCI), `striatum-demo-git-skill` (Git) | OCI registry   |
| [`striatum-demo-git-skill`](https://github.com/hbelmiro/striatum-demo-git-skill) | none                                                      | Git repository |

> **Note:** The checked-in `artifact.json` files for `example-skill` and `example-skill-git` do not include dependencies. The demo script (`scripts/demo.sh`) generates the full manifests at runtime, injecting the correct registry host and dependency declarations.

## Quick run (automated)

To run every step at once:

```bash
./scripts/demo.sh
```

The script starts a local registry, packs/pushes all artifacts, pulls them with dependency resolution, and exercises skill install/uninstall. To point at an existing registry:

```bash
export STRIATUM_DEMO_REGISTRY=localhost:5050/demo
./scripts/demo.sh
```

The rest of this document walks through each step manually so you can see what each command does.

## Step 1 — Start a local OCI registry

> **macOS note:** Port 5000 is used by AirPlay Receiver. The examples below
> use port **5050** instead. On Linux you can use any free port — just keep it
> consistent across all subsequent commands.

```bash
docker run -d -p 5050:5000 --name striatum-demo registry:2
sleep 2
```

## Step 2 — Pack and push leaf artifacts

Leaf artifacts have no dependencies. Pack bundles the files listed in `artifact.json` into a local OCI Image Layout, and push uploads that layout to the registry.

```bash
(cd demo/example-helper-a && ../../striatum pack && ../../striatum push localhost:5050/demo/example-helper-a:1.0.0)
(cd demo/example-helper-b && ../../striatum pack && ../../striatum push localhost:5050/demo/example-helper-b:1.0.0)
```

After this, both helpers are available in the registry at `localhost:5050/demo/example-helper-{a,b}:1.0.0`.

## Step 3 — Pack and push a root artifact with OCI dependencies

`example-skill` depends on both helpers. The demo script generates an `artifact.json` with the correct registry host at runtime, but you can create one yourself:

```json
{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": { "name": "example-skill", "version": "1.0.0" },
  "spec": { "entrypoint": "SKILL.md", "files": ["SKILL.md"] },
  "dependencies": [
    { "source": "oci", "registry": "localhost:5050", "repository": "demo/example-helper-a", "tag": "1.0.0" },
    { "source": "oci", "registry": "localhost:5050", "repository": "demo/example-helper-b", "tag": "1.0.0" }
  ]
}
```

Each OCI dependency declares its `registry`, `repository`, and `tag`. Striatum resolves them transitively at pull time.

Save that file as `artifact.json` alongside `demo/example-skill/SKILL.md`, then:

```bash
(cd demo/example-skill && ../../striatum pack && ../../striatum push localhost:5050/demo/example-skill:1.0.0)
```

## Step 4 — Pull with dependency resolution

```bash
./striatum pull localhost:5050/demo/example-skill:1.0.0 -o ./demo-out
```

Striatum reads the root manifest, discovers the two OCI dependencies, fetches their manifests, and downloads everything into the output directory:

```text
demo-out/
  example-skill/        # root artifact
  example-helper-a/     # OCI dependency
  example-helper-b/     # OCI dependency
```

## Step 5 — Install and uninstall a skill

Install copies the pulled artifacts into the Cursor skills directory (`~/.cursor/skills`):

```bash
./striatum skill install --target cursor localhost:5050/demo/example-skill:1.0.0
```

To remove it:

```bash
./striatum skill uninstall --target cursor example-skill
```

You can list cached and installed skills with:

```bash
./striatum skill list
./striatum skill list --installed --target cursor
```

## Step 6 — Git dependencies

Not every dependency lives in an OCI registry. Striatum can also resolve dependencies hosted in Git repositories.

`example-skill-git` demonstrates this. A Git dependency declares a `url` and a `ref` (branch, tag, or commit SHA). Striatum clones the repository at that ref and reads the `artifact.json` inside it.

The checked-in `demo/example-skill-git/artifact.json` has no dependencies (the demo script generates them at runtime). To try it manually, replace the file with a full manifest that includes both OCI and Git dependencies:

```json
{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": { "name": "example-skill-git", "version": "1.0.0" },
  "spec": { "entrypoint": "SKILL.md", "files": ["SKILL.md"] },
  "dependencies": [
    { "source": "oci", "registry": "localhost:5050", "repository": "demo/example-helper-a", "tag": "1.0.0" },
    { "source": "git", "url": "https://github.com/hbelmiro/striatum-demo-git-skill.git", "ref": "main" }
  ]
}
```

Then pack, push, and pull:

```bash
(cd demo/example-skill-git && ../../striatum pack && ../../striatum push localhost:5050/demo/example-skill-git:1.0.0)
./striatum pull localhost:5050/demo/example-skill-git:1.0.0 -o ./demo-out-git
```

The output directory now contains artifacts from both backends:

```text
demo-out-git/
  example-skill-git/          # root artifact (from OCI)
  example-helper-a/            # OCI dependency
  striatum-demo-git-skill/     # Git dependency (cloned from GitHub)
```

## Cleanup

```bash
docker rm -f striatum-demo
rm -rf demo-out demo-out-git
rm -rf demo/example-helper-a/build demo/example-helper-b/build demo/example-skill/build demo/example-skill-git/build
```
