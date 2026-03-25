package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/resolver"
)

// loadCachedSkillManifest tries to load a Skill manifest from the Striatum cache for
// the given name@version. Returns (manifest, nil) on cache hit, (nil, nil) on cache miss
// or after removing a corrupt entry, or (nil, error) on unrecoverable failures.
func loadCachedSkillManifest(name, version string) (*artifact.Manifest, error) {
	cacheDir := installer.CacheDir(name, version)
	manifestPath := filepath.Join(cacheDir, "artifact.json")
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat cache %s: %w", manifestPath, err)
	}
	m, err := artifact.Load(manifestPath)
	if err != nil {
		if removeErr := os.Remove(manifestPath); removeErr != nil {
			return nil, fmt.Errorf("cache corruption for %s@%s; remove failed: %w", name, version, removeErr)
		}
		return nil, nil
	}
	if m.Metadata.Name != name || m.Metadata.Version != version || m.Kind != "Skill" {
		if removeErr := os.Remove(manifestPath); removeErr != nil {
			return nil, fmt.Errorf("cache corruption for %s@%s; remove failed: %w", name, version, removeErr)
		}
		return nil, nil
	}
	return m, nil
}

// cacheFirstFetcher tries the local cache (name@version) before delegating to a remote fetcher.
type cacheFirstFetcher struct {
	next resolver.ManifestFetcher
}

// NewCacheFirstFetcher returns a ManifestFetcher that tries cache first, then next.
func NewCacheFirstFetcher(next resolver.ManifestFetcher) resolver.ManifestFetcher {
	return &cacheFirstFetcher{next: next}
}

// FetchManifest loads from cache when the reference maps to a cached name@version; otherwise delegates.
func (f *cacheFirstFetcher) FetchManifest(ctx context.Context, reference string) (*artifact.Manifest, error) {
	name, version, ok := refToCacheCandidate(reference)
	if !ok {
		return f.next.FetchManifest(ctx, reference)
	}
	m, err := loadCachedSkillManifest(name, version)
	if err != nil {
		return nil, err
	}
	if m != nil {
		return m, nil
	}
	m, err = f.next.FetchManifest(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("%s@%s cache miss; remote fetch failed: %w", name, version, err)
	}
	return m, nil
}

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
