# Example 04: Memory

This example packages a Memory artifact. Memory artifacts behave differently from Skills, Prompts, and Workflows: rather than copying a directory into place, the installer merges the artifact's markdown files into the memory index (`MEMORY.md`) of a Claude Code project, so their content is available across conversations.

The artifact lives in the `team-conventions/` directory. It contains an `artifact.json` manifest and a single `team-conventions.md` entrypoint that starts with YAML frontmatter, since each file becomes an indexed entry in the target project's `MEMORY.md`.

## Walkthrough

Build the CLI and start a local registry:

```bash
go build -o striatum ./cmd/striatum
docker run -d -p 5050:5000 --name striatum-demo registry:2
```

Validate the manifest before packing:

```bash
cd examples/04-memory/team-conventions
striatum validate
```

Pack the artifact into a local OCI image layout, then push it to the registry:

```bash
striatum pack
striatum push localhost:5050/examples/team-conventions:1.0.0
```

Pull the image back down into an output directory:

```bash
striatum pull localhost:5050/examples/team-conventions:1.0.0 -o out
```

Inspect the manifest of the pushed image:

```bash
striatum inspect localhost:5050/examples/team-conventions:1.0.0
```

Install the memory into a specific project. The install command requires either `--project <dir>` or `--all-existing-projects`, because a Memory artifact must be merged into a project's memory index rather than copied to a global directory:

```bash
striatum install --target claude --project /path/to/my-project localhost:5050/examples/team-conventions:1.0.0
```

To install the memory into every Claude Code project that already exists on your machine:

```bash
striatum install --target claude --all-existing-projects localhost:5050/examples/team-conventions:1.0.0
```

After installing, the project's `MEMORY.md` gains an index entry pointing at the merged `team-conventions.md` file under the project's memory directory. Uninstall removes the entry and the merged file. Memory artifacts are project-scoped, so the uninstall needs the same `--project` (or `--all-existing-projects`) the install used:

```bash
striatum uninstall --target claude --project /path/to/my-project --kind Memory team-conventions
```
