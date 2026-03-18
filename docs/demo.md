# Demo: Full flow with dependencies

This demo runs the full Striatum flow using **fictitious** artifacts with dependencies:

1. **Pack** — bundle each artifact into an OCI layout
2. **Push** — push to a local OCI registry
3. **Pull** — download the root artifact and its transitive dependencies
4. **Skill Install** — copy into the Cursor skills directory
5. **Skill Uninstall** — remove the skill and orphaned dependencies

The demo artifacts live in the `demo/` directory:

- `example-skill` — root artifact (version 1.0.0) that depends on the two helpers
- `example-helper-a` — dependency (1.0.0)
- `example-helper-b` — dependency (1.0.0)

## Quick run

From the repository root, with `striatum` built and Docker available:

```bash
./scripts/demo.sh
```

The script will start a local registry (registry:2) on port 5000 if needed, then run pack → push → pull → install → uninstall. To use an already-running registry instead, set:

```bash
export STRIATUM_DEMO_REGISTRY=localhost:5000/demo
./scripts/demo.sh
```

## Manual steps

If you prefer to run each step yourself (from the repository root, with `striatum` built as `./striatum`):

```bash
# Build and start registry (if not already running)
go build -o striatum ./cmd/striatum
docker run -d -p 5000:5000 --name striatum-demo registry:2
sleep 2

# Pack and push (order: helpers first, then root)
(cd demo/example-helper-a && ../../striatum pack && ../../striatum push localhost:5000/demo/example-helper-a:1.0.0)
(cd demo/example-helper-b && ../../striatum pack && ../../striatum push localhost:5000/demo/example-helper-b:1.0.0)
(cd demo/example-skill  && ../../striatum pack && ../../striatum push localhost:5000/demo/example-skill:1.0.0)

# Pull, skill install, skill uninstall
./striatum pull localhost:5000/demo/example-skill:1.0.0 -o ./demo-out
./striatum skill install --target cursor localhost:5000/demo/example-skill:1.0.0
./striatum skill uninstall --target cursor example-skill

# Stop registry
docker rm -f striatum-demo
```
