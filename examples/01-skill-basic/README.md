# Example 01: Skill basic

This example covers the core striatum lifecycle with a single Skill artifact and no dependencies. You will validate the manifest, pack it into an OCI image layout, push it to a local registry, pull it back, inspect it, and install it as a Claude skill.

The artifact lives in the `my-skill/` directory. It contains an `artifact.json` manifest and a single `SKILL.md` entrypoint.

## Walkthrough

Build the CLI and start a local registry. On macOS, port 5000 conflicts with AirPlay Receiver, so the examples use port 5050:

```bash
go build -o striatum ./cmd/striatum
docker run -d -p 5050:5000 --name striatum-demo registry:2
```

The `init` command scaffolds a new artifact directory. It is informational only; this example already contains the generated files:

```bash
cd examples/01-skill-basic/my-skill
striatum init --kind Skill my-skill
```

Validate the manifest before packing. This checks the schema, the kind, the version format, and that the entrypoint is listed in `files`:

```bash
striatum validate
```

Pack the artifact into a local OCI image layout, then push it to the registry:

```bash
striatum pack
striatum push localhost:5050/examples/my-skill:1.0.0
```

Pull the image back down into an output directory:

```bash
striatum pull localhost:5050/examples/my-skill:1.0.0 -o out
```

Inspect the manifest of the pushed image without downloading its files:

```bash
striatum inspect localhost:5050/examples/my-skill:1.0.0
```

Install the skill into Claude's skill directory:

```bash
striatum install --target claude localhost:5050/examples/my-skill:1.0.0
```

Uninstall it again when you no longer need it:

```bash
striatum uninstall --target claude my-skill
```
