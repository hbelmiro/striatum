#!/usr/bin/env bash
# Demo: pack fictitious artifacts (example-skill + deps), push to a local registry,
# pull, install, and uninstall. Requires Docker for the registry unless
# STRIATUM_DEMO_REGISTRY is set to an already-running registry base (e.g. localhost:5000/demo).
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DEMO_DIR="$REPO_ROOT/demo"
REGISTRY_NAME="${STRIATUM_DEMO_REGISTRY:-localhost:5000/demo}"
STRiatUM="${STRIATUM_BIN:-striatum}"

# Ensure we have the binary (use absolute path when in repo so it works from any cwd)
if ! command -v "$STRiatUM" &>/dev/null; then
  if [ -x "$REPO_ROOT/striatum" ]; then
    STRiatUM="$(cd "$REPO_ROOT" && pwd)/striatum"
  else
    echo "Build striatum first: go build -o striatum ./cmd/striatum"
    exit 1
  fi
fi

# Start registry if not using external one
if [ -z "$STRIATUM_DEMO_REGISTRY" ]; then
  if ! command -v docker &>/dev/null; then
    echo "Docker is required to run the demo (or set STRIATUM_DEMO_REGISTRY to an existing registry base)."
    exit 1
  fi
  CONTAINER_NAME="striatum-demo-registry"
  if ! docker ps -q -f name="^${CONTAINER_NAME}$" | grep -q .; then
    echo "Starting local registry (${CONTAINER_NAME})..."
    docker run -d -p 5000:5000 --name "$CONTAINER_NAME" registry:2 >/dev/null
    trap 'docker rm -f "$CONTAINER_NAME" 2>/dev/null || true' EXIT
    sleep 2
  fi
fi

echo "Using registry base: $REGISTRY_NAME"
cd "$REPO_ROOT"

# Pack and push helpers first, then root
for dir in example-helper-a example-helper-b example-skill; do
  echo "Packing and pushing $dir..."
  (cd "$DEMO_DIR/$dir" && "$STRiatUM" pack && "$STRiatUM" push "$REGISTRY_NAME/$dir:1.0.0")
done

# Pull root (with transitive deps)
OUT_DIR="$REPO_ROOT/.demo-out"
rm -rf "$OUT_DIR"
echo "Pulling example-skill and dependencies..."
"$STRiatUM" pull "$REGISTRY_NAME/example-skill:1.0.0" -o "$OUT_DIR"
echo "Pulled to $OUT_DIR"

# Install (requires --target)
echo "Installing to Cursor target..."
"$STRiatUM" install --target cursor "$REGISTRY_NAME/example-skill:1.0.0"

# Uninstall
echo "Uninstalling example-skill..."
"$STRiatUM" uninstall --target cursor example-skill

echo "Demo complete: pack → push → pull → install → uninstall."
