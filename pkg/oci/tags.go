package oci

import (
	"context"
	"fmt"
	"strings"

	orasoci "oras.land/oras-go/v2/content/oci"
)

// ListTags returns all tags for the repository identified by reference.
// For oci: layout references, pass "oci:/path/to/layout".
// For remote references, pass "host/repo/name" (no tag).
func ListTags(ctx context.Context, reference string) ([]string, error) {
	if strings.HasPrefix(reference, "oci:") {
		return listTagsOCILayout(ctx, reference[len("oci:"):])
	}
	return listTagsRemote(ctx, reference)
}

func listTagsOCILayout(ctx context.Context, path string) ([]string, error) {
	store, err := orasoci.New(path)
	if err != nil {
		return nil, fmt.Errorf("open OCI layout %q: %w", path, err)
	}
	var tags []string
	err = store.Tags(ctx, "", func(t []string) error {
		tags = append(tags, t...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list tags from OCI layout %q: %w", path, err)
	}
	return tags, nil
}

func listTagsRemote(ctx context.Context, reference string) ([]string, error) {
	repo, err := NewRepository(reference)
	if err != nil {
		return nil, fmt.Errorf("create repository %q: %w", reference, err)
	}
	var tags []string
	err = repo.Tags(ctx, "", func(t []string) error {
		tags = append(tags, t...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list tags from %q: %w", reference, err)
	}
	return tags, nil
}
