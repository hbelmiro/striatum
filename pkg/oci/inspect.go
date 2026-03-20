package oci

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hbelmiro/striatum/pkg/artifact"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
)

// loadArtifactFromOCI resolves ref, fetches the OCI image manifest and config blob
// once, and returns the OCI manifest (for layers), the parsed artifact manifest,
// and the raw config JSON (for writing artifact.json without re-fetching).
func loadArtifactFromOCI(ctx context.Context, target oras.ReadOnlyTarget, ref string) (ocispec.Manifest, *artifact.Manifest, []byte, error) {
	var zero ocispec.Manifest
	desc, err := target.Resolve(ctx, ref)
	if err != nil {
		return zero, nil, nil, fmt.Errorf("resolve %q: %w", ref, err)
	}

	manifestBytes, err := content.FetchAll(ctx, target, desc)
	if err != nil {
		return zero, nil, nil, fmt.Errorf("fetch manifest: %w", err)
	}

	var ociManifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &ociManifest); err != nil {
		return zero, nil, nil, fmt.Errorf("parse manifest: %w", err)
	}

	configBytes, err := content.FetchAll(ctx, target, ociManifest.Config)
	if err != nil {
		return zero, nil, nil, fmt.Errorf("fetch config: %w", err)
	}

	var m artifact.Manifest
	if err := json.Unmarshal(configBytes, &m); err != nil {
		return zero, nil, nil, fmt.Errorf("parse artifact config: %w", err)
	}
	return ociManifest, &m, configBytes, nil
}

// Inspect fetches the artifact manifest from the target for the given reference
// (e.g. "name:1.0.0" or a full registry reference). It resolves the reference,
// fetches the OCI manifest, then fetches the config blob and unmarshals it as
// artifact manifest.
func Inspect(ctx context.Context, target oras.ReadOnlyTarget, ref string) (*artifact.Manifest, error) {
	_, m, _, err := loadArtifactFromOCI(ctx, target, ref)
	if err != nil {
		return nil, err
	}
	return m, nil
}
