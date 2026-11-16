package oci

import (
	"context"
	"encoding/json/v2"
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
		return zero, nil, nil, fmt.Errorf("fetch OCI manifest: %w", err)
	}

	var ociManifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &ociManifest); err != nil {
		return zero, nil, nil, fmt.Errorf("parse OCI manifest: %w", err)
	}

	configBytes, err := content.FetchAll(ctx, target, ociManifest.Config)
	if err != nil {
		return zero, nil, nil, fmt.Errorf("fetch OCI config blob: %w", err)
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
	m, _, err := InspectWithConfig(ctx, target, ref)
	return m, err
}

// InspectWithConfig is like Inspect but also returns the raw config blob
// bytes, so callers that need both (e.g. to check the self-contained image marker) avoid a
// second round trip.
func InspectWithConfig(ctx context.Context, target oras.ReadOnlyTarget, ref string) (*artifact.Manifest, []byte, error) {
	_, m, configBytes, err := loadArtifactFromOCI(ctx, target, ref)
	if err != nil {
		return nil, nil, err
	}
	return m, configBytes, nil
}

// ResolveDigest resolves the OCI manifest digest for the given reference without
// fetching the full manifest or config. Returns the digest string (e.g. "sha256:abc...").
func ResolveDigest(ctx context.Context, target oras.ReadOnlyTarget, ref string) (string, error) {
	desc, err := target.Resolve(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", ref, err)
	}
	return desc.Digest.String(), nil
}

// LoadImage resolves ref and fetches the OCI image manifest, the parsed
// artifact manifest, and the raw config blob in a single pass, so callers
// that need several of them avoid redundant round trips.
func LoadImage(ctx context.Context, target oras.ReadOnlyTarget, ref string) (ocispec.Manifest, *artifact.Manifest, []byte, error) {
	return loadArtifactFromOCI(ctx, target, ref)
}

// FetchLayerFromManifest returns the raw bytes of the layer whose title
// annotation (org.opencontainers.image.title) equals title, using an
// already-fetched OCI image manifest to avoid a re-fetch.
func FetchLayerFromManifest(ctx context.Context, target oras.ReadOnlyTarget, ociManifest ocispec.Manifest, title string) ([]byte, error) {
	for _, l := range ociManifest.Layers {
		if l.Annotations != nil && l.Annotations["org.opencontainers.image.title"] == title {
			return content.FetchAll(ctx, target, l)
		}
	}
	return nil, fmt.Errorf("no layer titled %q", title)
}
