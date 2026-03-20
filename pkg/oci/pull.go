package oci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
)

// safeLayerPath sanitizes a layer filename for extraction: rejects absolute paths
// and ".." segments so the path cannot escape the artifact directory.
func safeLayerPath(name string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(strings.TrimSpace(name)))
	if cleaned == "" {
		return "", fmt.Errorf("empty layer name")
	}
	if filepath.IsAbs(cleaned) || strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("disallowed path (absolute or contains ..)")
	}
	return cleaned, nil
}

// Pull fetches the artifact from the target for the given reference and
// extracts it to outputDir/<artifact-name>/ (artifact.json plus all layer files).
func Pull(ctx context.Context, target oras.ReadOnlyTarget, ref string, outputDir string) error {
	manifest, m, configBytes, err := loadArtifactFromOCI(ctx, target, ref)
	if err != nil {
		return fmt.Errorf("read artifact manifest: %w", err)
	}

	artifactPath := filepath.Join(outputDir, m.Metadata.Name)
	if err := os.MkdirAll(artifactPath, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(artifactPath, "artifact.json"), configBytes, 0o600); err != nil {
		return fmt.Errorf("write artifact.json: %w", err)
	}

	for _, layer := range manifest.Layers {
		name := layer.Annotations[annotationTitle]
		if name == "" {
			name = layer.Digest.Encoded() // fallback
		}
		safeName, err := safeLayerPath(name)
		if err != nil {
			return fmt.Errorf("layer name %q: %w", name, err)
		}
		data, err := content.FetchAll(ctx, target, layer)
		if err != nil {
			return fmt.Errorf("fetch layer %q: %w", name, err)
		}
		p := filepath.Join(artifactPath, filepath.FromSlash(safeName))
		clean := filepath.Clean(p)
		base := filepath.Clean(artifactPath)
		rel, relErr := filepath.Rel(base, clean)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("layer name %q escapes artifact path", name)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("create layer dir: %w", err)
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			return fmt.Errorf("write %q: %w", name, err)
		}
	}

	return nil
}
