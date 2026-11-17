# Example 03: Workflow with the Claude target

This example packages a Workflow artifact. Workflows are multi-step scripts that Claude executes, and unlike Skills and Prompts they install only with the `claude` target: a Workflow lands in `~/.claude/workflows/` with a `name.js` symlink pointing at the entrypoint.

The artifact lives in the `pr-review/` directory. It contains an `artifact.json` manifest and a single `review.js` entrypoint that drives a two-phase code review.

## Walkthrough

Build the CLI and start a local registry:

```bash
go build -o striatum ./cmd/striatum
docker run -d -p 5050:5000 --name striatum-demo registry:2
```

Validate the manifest before packing:

```bash
cd examples/03-workflow-claude/pr-review
striatum validate
```

Pack the artifact into a local OCI image layout, then push it to the registry:

```bash
striatum pack
striatum push localhost:5050/examples/pr-review:1.0.0
```

Pull the image back down into an output directory:

```bash
striatum pull localhost:5050/examples/pr-review:1.0.0 -o out
```

Inspect the manifest of the pushed image:

```bash
striatum inspect localhost:5050/examples/pr-review:1.0.0
```

Install the workflow with the Claude target. This copies the artifact into `~/.claude/workflows/pr-review/` and creates a `~/.claude/workflows/pr-review.js` symlink to the entrypoint so the workflow runner can find it:

```bash
striatum install --target claude localhost:5050/examples/pr-review:1.0.0
```

Uninstall it again when you no longer need it:

```bash
striatum uninstall --target claude pr-review
```
