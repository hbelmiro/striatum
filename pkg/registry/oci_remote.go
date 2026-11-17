package registry

import (
	"context"
	"fmt"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
)

// OCIRemoteBackend implements OCIBackend using the pkg/oci functions
// for remote OCI registries.
type OCIRemoteBackend struct{}

var _ OCIBackend = (*OCIRemoteBackend)(nil)

// resolveRef returns the reference to use for the dependency: the pinned
// digest when one is declared (so a moved or deleted tag cannot change what
// resolves), and the tag otherwise.
func (b *OCIRemoteBackend) resolveRef(dep *artifact.OCIDependency) string {
	if dep.Digest != "" {
		return dep.Digest
	}
	return dep.Tag
}

func (b *OCIRemoteBackend) Inspect(ctx context.Context, dep *artifact.OCIDependency) (*artifact.Manifest, error) {
	repoPath := dep.RegistryHost + "/" + dep.Repository
	reg, err := oci.NewRepository(repoPath)
	if err != nil {
		return nil, fmt.Errorf("create repository for %s: %w", dep.CanonicalRef(), err)
	}
	return oci.Inspect(ctx, reg, b.resolveRef(dep))
}

func (b *OCIRemoteBackend) Pull(ctx context.Context, dep *artifact.OCIDependency, outputDir string) error {
	repoPath := dep.RegistryHost + "/" + dep.Repository
	reg, err := oci.NewRepository(repoPath)
	if err != nil {
		return fmt.Errorf("create repository for %s: %w", dep.CanonicalRef(), err)
	}
	return oci.Pull(ctx, reg, b.resolveRef(dep), outputDir)
}
