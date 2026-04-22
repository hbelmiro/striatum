#!/usr/bin/env bash
# Demo: pack fictitious artifacts (example-skill + deps), push to a local registry,
# pull, install, and uninstall.  Also demonstrates a Git-hosted dependency.
#
# Requires Docker for the registry unless STRIATUM_DEMO_REGISTRY is set to an
# already-running registry base (e.g. localhost:5050/demo).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DEMO_DIR="$REPO_ROOT/demo"
STRIATUM="${STRIATUM_BIN:-striatum}"

# Registry port — macOS uses 5000 for AirPlay, so default to 5050.
REGISTRY_PORT="${STRIATUM_DEMO_PORT:-5050}"
REGISTRY_BASE="${STRIATUM_DEMO_REGISTRY:-localhost:${REGISTRY_PORT}/demo}"

# Extract host:port from the registry base for dependency declarations.
# "localhost:5050/demo" -> "localhost:5050"
REGISTRY_HOST="${REGISTRY_BASE%%/*}"

# Temp directories cleaned up on exit
TMPROOT="$(mktemp -d)"
trap 'rm -rf "$TMPROOT"' EXIT

# Ensure we have the binary (use absolute path when in repo so it works from any cwd)
if ! command -v "$STRIATUM" &>/dev/null; then
  if [ -x "$REPO_ROOT/striatum" ]; then
    STRIATUM="$(cd "$REPO_ROOT" && pwd)/striatum"
  else
    echo "Build striatum first: go build -o striatum ./cmd/striatum"
    exit 1
  fi
fi

# Start registry if not using external one
if [ -z "${STRIATUM_DEMO_REGISTRY:-}" ]; then
  if ! command -v docker &>/dev/null; then
    echo "Docker is required to run the demo (or set STRIATUM_DEMO_REGISTRY to an existing registry base)."
    exit 1
  fi
  CONTAINER_NAME="striatum-demo-registry"
  docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
  echo "Starting local registry (${CONTAINER_NAME}) on port ${REGISTRY_PORT}..."
  docker run -d -p "${REGISTRY_PORT}:5000" --name "$CONTAINER_NAME" registry:2 >/dev/null
  trap 'docker rm -f "$CONTAINER_NAME" 2>/dev/null || true; rm -rf "$TMPROOT"' EXIT
  sleep 2
fi

echo "Using registry base: $REGISTRY_BASE"
cd "$REPO_ROOT"

# ---------------------------------------------------------------------------
# 1) OCI artifacts: pack and push helpers first, then root
# ---------------------------------------------------------------------------
for dir in example-helper-a example-helper-b; do
  echo "Packing and pushing $dir..."
  (cd "$DEMO_DIR/$dir" && "$STRIATUM" pack && "$STRIATUM" push "$REGISTRY_BASE/$dir:1.0.0")
done

# example-skill has OCI dependencies — generate artifact.json with correct registry host
SKILL_DIR="$TMPROOT/example-skill"
mkdir -p "$SKILL_DIR"
cp "$DEMO_DIR/example-skill/SKILL.md" "$SKILL_DIR/"
cat > "$SKILL_DIR/artifact.json" <<EOF
{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": { "name": "example-skill", "version": "1.0.0" },
  "spec": { "entrypoint": "SKILL.md", "files": ["SKILL.md"] },
  "dependencies": [
    { "source": "oci", "registry": "$REGISTRY_HOST", "repository": "demo/example-helper-a", "tag": "1.0.0" },
    { "source": "oci", "registry": "$REGISTRY_HOST", "repository": "demo/example-helper-b", "tag": "1.0.0" }
  ]
}
EOF

echo "Packing and pushing example-skill..."
(cd "$SKILL_DIR" && "$STRIATUM" pack && "$STRIATUM" push "$REGISTRY_BASE/example-skill:1.0.0")

# Pull OCI root (with transitive OCI deps)
OUT_DIR="$REPO_ROOT/.demo-out"
rm -rf "$OUT_DIR"
echo "Pulling example-skill and OCI dependencies..."
"$STRIATUM" pull "$REGISTRY_BASE/example-skill:1.0.0" -o "$OUT_DIR"
echo "Pulled to $OUT_DIR"

# Skill install / uninstall (OCI only)
echo "Installing example-skill to Cursor target..."
"$STRIATUM" skill install --target cursor "$REGISTRY_BASE/example-skill:1.0.0"
echo "Uninstalling example-skill..."
"$STRIATUM" skill uninstall --target cursor example-skill

# ---------------------------------------------------------------------------
# 2) Git dependency demo
# ---------------------------------------------------------------------------
echo ""
echo "=== Git dependency demo ==="

GIT_DEP_URL="https://github.com/hbelmiro/striatum-demo-git-skill.git"

SKILL_GIT_DIR="$TMPROOT/example-skill-git"
mkdir -p "$SKILL_GIT_DIR"
cp "$DEMO_DIR/example-skill-git/SKILL.md" "$SKILL_GIT_DIR/"

cat > "$SKILL_GIT_DIR/artifact.json" <<EOF
{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": { "name": "example-skill-git", "version": "1.0.0" },
  "spec": { "entrypoint": "SKILL.md", "files": ["SKILL.md"] },
  "dependencies": [
    { "source": "oci", "registry": "$REGISTRY_HOST", "repository": "demo/example-helper-a", "tag": "1.0.0" },
    { "source": "git", "url": "$GIT_DEP_URL", "ref": "main" }
  ]
}
EOF

echo "Packing and pushing example-skill-git..."
(cd "$SKILL_GIT_DIR" && "$STRIATUM" pack && "$STRIATUM" push "$REGISTRY_BASE/example-skill-git:1.0.0")

# Pull — resolves both OCI and Git dependencies
GIT_OUT_DIR="$REPO_ROOT/.demo-out-git"
rm -rf "$GIT_OUT_DIR"
echo "Pulling example-skill-git (OCI + Git dependencies)..."
"$STRIATUM" pull "$REGISTRY_BASE/example-skill-git:1.0.0" -o "$GIT_OUT_DIR"
echo "Pulled to $GIT_OUT_DIR"

# Verify
for name in example-skill-git example-helper-a striatum-demo-git-skill; do
  if [ -f "$GIT_OUT_DIR/$name/artifact.json" ]; then
    echo "  ✓ $name"
  else
    echo "  ✗ $name MISSING" >&2
    exit 1
  fi
done

echo ""
echo "Demo complete: pack → push → pull → skill install → skill uninstall (OCI + Git)."
