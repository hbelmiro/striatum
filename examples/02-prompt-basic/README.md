# Example 02: Prompt basic

This example covers the same lifecycle as [01-skill-basic](../01-skill-basic), but for a Prompt artifact. Prompts follow the same validate, pack, push, pull, inspect, install, and uninstall flow as skills; the difference is where they land: a Prompt installs into the target's prompts directory (for example `~/.claude/prompts/`) instead of its skills directory.

The artifact lives in the `severity-rubric/` directory. It contains an `artifact.json` manifest and a single `severity-rubric.md` entrypoint that defines a bug severity scale.

## Walkthrough

Build the CLI and start a local registry:

```bash
go build -o striatum ./cmd/striatum
docker run -d -p 5050:5000 --name striatum-demo registry:2
```

Validate the manifest before packing:

```bash
cd examples/02-prompt-basic/severity-rubric
striatum validate
```

Pack the artifact into a local OCI image layout, then push it to the registry:

```bash
striatum pack
striatum push localhost:5050/examples/severity-rubric:1.0.0
```

Pull the image back down into an output directory:

```bash
striatum pull localhost:5050/examples/severity-rubric:1.0.0 -o out
```

Inspect the manifest of the pushed image:

```bash
striatum inspect localhost:5050/examples/severity-rubric:1.0.0
```

Install the prompt into the target's prompts directory:

```bash
striatum install --target claude localhost:5050/examples/severity-rubric:1.0.0
```

Uninstall it again when you no longer need it:

```bash
striatum uninstall --target claude severity-rubric
```

Prompts can also be consumed as dependencies by other artifacts; see [05-oci-dependencies](../05-oci-dependencies).
