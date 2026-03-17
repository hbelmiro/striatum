package oci

import (
	"context"
	"fmt"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
)

// Push packs the artifact and pushes it to the registry (or OCI layout) at reference.
// Reference format: "host/repo/name:tag" for remote, or "oci:/path/to/layout:tag" for local layout.
func Push(ctx context.Context, m *artifact.Manifest, baseDir string, reference string) error {
	repo, tag, err := parseReference(reference)
	if err != nil {
		return err
	}

	mem := memory.New()
	if err := packToTarget(ctx, m, baseDir, mem, tag); err != nil {
		return err
	}

	if strings.HasPrefix(reference, "oci:") {
		layoutPath := strings.TrimPrefix(repo, "oci:")
		layoutStore, err := oci.New(layoutPath)
		if err != nil {
			return fmt.Errorf("open OCI layout: %w", err)
		}
		_, err = oras.Copy(ctx, mem, tag, layoutStore, tag, oras.DefaultCopyOptions)
		return err
	}

	reg, err := remote.NewRepository(repo)
	if err != nil {
		return fmt.Errorf("create repository: %w", err)
	}
	_, err = oras.Copy(ctx, mem, tag, reg, tag, oras.DefaultCopyOptions)
	return err
}

// parseReference splits "host/repo/name:tag" into ("host/repo/name", "tag").
// For "oci:/path:tag" returns ("oci:/path", "tag"). Returns error if no colon.
func parseReference(reference string) (repo string, tag string, err error) {
	i := strings.LastIndex(reference, ":")
	if i < 0 {
		return "", "", fmt.Errorf("invalid reference %q: missing tag", reference)
	}
	return reference[:i], reference[i+1:], nil
}
