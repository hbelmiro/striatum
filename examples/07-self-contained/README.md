# Example 07: Self-contained bundles

This example shows `--self-contained`, which bundles the full dependency closure into a single OCI image. The `bundle-skill` skill depends on `helper-a` and `helper-b`, and its self-contained image carries all three artifacts.

## Layout

| Directory | Kind | Role |
| --- | --- | --- |
| `helper-a/` | Skill | Standalone helper pushed as an image |
| `helper-b/` | Skill | Standalone helper pushed as an image |
| `bundle-skill/` | Skill | Root skill with two OCI dependencies |

## Push the helpers first

Self-contained packing resolves the dependency closure at pack time, so the helpers must be in the registry:

```bash
cd helper-a
striatum pack
striatum push localhost:5000/examples/helper-a:1.0.0

cd ../helper-b
striatum pack
striatum push localhost:5000/examples/helper-b:1.0.0
```

The `bundle-skill/artifact.json` committed here declares both dependencies against registry host `localhost:5000`. Adjust the `registry` fields if you push elsewhere.

## Pack and push the bundle

From the root skill directory, add `--self-contained` to both commands:

```bash
cd bundle-skill
striatum pack --self-contained
striatum push --self-contained --print-digest localhost:5000/examples/bundle-skill:1.0.0
```

The `--print-digest` flag prints one canonical reference line in the form `localhost:5000/examples/bundle-skill@sha256:<digest>`, which you can copy verbatim into scripts, documentation, or a dependency declaration.

## Why self-contained

A normal push stores only the root artifact; consumers resolve dependencies themselves from their sources at pull or install time. A self-contained image bundles the full dependency closure, so consumers such as MLflow or `striatum install` need no access to the dependency sources at all. Whatever registry state changes later, the image still contains everything it needs.

Installing by digest reference installs the bundle as one unit:

```bash
striatum install localhost:5000/examples/bundle-skill@sha256:<digest>
```

Every artifact in the closure lands under `~/.claude/skills/` from that single image, with no further resolution steps.
