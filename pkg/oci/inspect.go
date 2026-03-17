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

// Inspect fetches the artifact manifest from the target for the given reference
// (e.g. "name:1.0.0" or a full registry reference). It resolves the reference,
// fetches the OCI manifest, then fetches the config blob and unmarshals it as
// artifact manifest.
func Inspect(ctx context.Context, target oras.ReadOnlyTarget, ref string) (*artifact.Manifest, error) {
	desc, err := target.Resolve(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", ref, err)
	}

	manifestBytes, err := content.FetchAll(ctx, target, desc)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	configBytes, err := content.FetchAll(ctx, target, manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("fetch config: %w", err)
	}

	var m artifact.Manifest
	if err := json.Unmarshal(configBytes, &m); err != nil {
		return nil, fmt.Errorf("parse artifact config: %w", err)
	}
	return &m, nil
}
