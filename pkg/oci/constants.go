package oci

import "strings"

// Striatum OCI media types (see docs/demo.md).
const (
	ConfigMediaType = "application/vnd.striatum.artifact.config.v1+json"
	LayerMediaType  = "application/vnd.striatum.artifact.layer.v1.file"
)

// ArtifactTypeForKind returns the OCI artifactType for the given artifact kind.
// The kind must be a non-empty, validated kind (e.g. "Skill"); callers should
// run artifact.Validate before calling this.
// Example: "Skill" -> "application/vnd.striatum.skill.v1"
func ArtifactTypeForKind(kind string) string {
	return "application/vnd.striatum." + strings.ToLower(kind) + ".v1"
}
