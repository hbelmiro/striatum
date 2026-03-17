package oci

import (
	"context"
	"fmt"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/content/oci"
)

// Push packs the artifact and pushes it to the registry (or OCI layout) at reference.
// Reference format: "host/repo/name:tag" for remote, or "oci:/path/to/layout:tag" for local layout.
// For oci: references, the path is passed to the OCI layout store as-is (use absolute paths
// for predictable behavior across platforms).
func Push(ctx context.Context, m *artifact.Manifest, baseDir string, reference string) error {
	repo, tag, err := SplitReference(reference)
	if err != nil {
		return fmt.Errorf("parse reference: %w", err)
	}

	mem := memory.New()
	if err := packToTarget(ctx, m, baseDir, mem, tag); err != nil {
		return fmt.Errorf("pack artifact: %w", err)
	}

	if strings.HasPrefix(reference, "oci:") {
		layoutStore, err := oci.New(repo)
		if err != nil {
			return fmt.Errorf("open OCI layout: %w", err)
		}
		if _, err = oras.Copy(ctx, mem, tag, layoutStore, tag, oras.DefaultCopyOptions); err != nil {
			return fmt.Errorf("copy to OCI layout: %w", err)
		}
		return nil
	}

	reg, err := NewRepository(repo)
	if err != nil {
		return fmt.Errorf("create repository: %w", err)
	}
	if _, err = oras.Copy(ctx, mem, tag, reg, tag, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("push to registry: %w", err)
	}
	return nil
}
