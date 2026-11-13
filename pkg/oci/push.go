package oci

import (
	"context"
	"fmt"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
)

// Push packs the artifact and pushes it to the OCI registry at reference.
// Reference format: "host/repo/name:tag" (optionally with an @digest suffix,
// which is ignored for push).
// OCI layout references ("oci:/path/to/layout:tag") are rejected: push is
// registry-only. Use Pack (or "striatum pack -o") to write a local layout.
// Returns the manifest descriptor of the pushed artifact.
func Push(ctx context.Context, m *artifact.Manifest, baseDir string, reference string) (ocispec.Descriptor, error) {
	if strings.HasPrefix(reference, "oci:") {
		return ocispec.Descriptor{}, fmt.Errorf("push is registry-only: %q is an OCI layout reference; use \"striatum pack -o <dir>\" to write a local layout", reference)
	}

	repo, tag, _, err := SplitReference(reference)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parse reference: %w", err)
	}

	mem := memory.New()
	if err := packToTarget(ctx, m, baseDir, mem, tag); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack artifact: %w", err)
	}

	reg, err := NewRepository(repo)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create repository: %w", err)
	}
	desc, err := oras.Copy(ctx, mem, tag, reg, tag, oras.DefaultCopyOptions)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push to registry: %w", err)
	}
	return desc, nil
}
