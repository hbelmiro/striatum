# Example 05: OCI dependencies

This example shows a Skill that depends on two other Skills published as OCI images. The `greeter` skill declares `greeting-styles` and `greeting-tone` as OCI dependencies, so installing or pulling it resolves the whole closure from the registry.

## Layout

| Directory | Kind | Role |
| --- | --- | --- |
| `greeting-styles/` | Skill | Standalone helper pushed as an image |
| `greeting-tone/` | Skill | Standalone helper pushed as an image |
| `greeter/` | Skill | Root skill with two OCI dependencies |

## Push the dependencies first

Dependencies must exist in the registry before the root artifact can resolve them. Pack and push each helper:

```bash
cd greeting-styles
striatum pack
striatum push localhost:5000/examples/greeting-styles:1.0.0

cd ../greeting-tone
striatum pack
striatum push localhost:5000/examples/greeting-tone:1.0.0
```

The committed `greeter/artifact.json` declares the dependencies against the registry host `localhost:5000`:

```json
"dependencies": [
  {
    "source": "oci",
    "registry": "localhost:5000",
    "repository": "examples/greeting-styles",
    "tag": "1.0.0"
  },
  {
    "source": "oci",
    "registry": "localhost:5000",
    "repository": "examples/greeting-tone",
    "tag": "1.0.0"
  }
]
```

If you push to a different registry (a different port, or a remote host), edit the `registry` fields in `greeter/artifact.json` to match wherever you actually pushed the helpers. The registry host in the manifest must match the registry the images live in, or resolution fails.

## Validate and push the root

With the helpers in place, check that the dependency closure resolves, then push the root skill:

```bash
cd greeter
striatum pack
striatum validate --check-deps
striatum push localhost:5000/examples/greeter:1.0.0
```

`striatum validate --check-deps` fails with a clear error if any dependency is missing from the registry or the manifest is malformed.

## Install resolves the whole closure

Installing the root skill resolves the dependency closure and materializes every artifact under `~/.claude/skills/`, with each dependency landing as its own directory:

```bash
striatum install --target claude localhost:5000/examples/greeter:1.0.0
```

After this you will find `~/.claude/skills/greeter/`, `~/.claude/skills/greeting-styles/`, and `~/.claude/skills/greeting-tone/` on disk. You can also use `striatum pull` with an output directory to fetch the closure without installing it.
