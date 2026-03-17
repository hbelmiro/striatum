package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/resolver"
)

// remoteFetcher fetches manifests from a remote registry by reference (host/repo/name:version).
type remoteFetcher struct{}

// NewRemoteFetcher returns a ManifestFetcher that fetches from remote registries.
func NewRemoteFetcher() resolver.ManifestFetcher {
	return &remoteFetcher{}
}

// FetchManifest parses reference into repo and tag, then inspects the remote repository.
func (f *remoteFetcher) FetchManifest(ctx context.Context, reference string) (*artifact.Manifest, error) {
	i := strings.LastIndex(reference, ":")
	if i < 0 {
		return nil, fmt.Errorf("invalid reference %q: expected host/repo/name:version", reference)
	}
	repo, tag := strings.TrimSpace(reference[:i]), strings.TrimSpace(reference[i+1:])
	if repo == "" || tag == "" {
		return nil, fmt.Errorf("invalid reference %q: expected host/repo/name:version", reference)
	}
	reg, err := oci.NewRepository(repo)
	if err != nil {
		return nil, fmt.Errorf("create repository for %q: %w", reference, err)
	}
	return oci.Inspect(ctx, reg, tag)
}
