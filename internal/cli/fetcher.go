package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/registry"
	gitbackend "github.com/hbelmiro/striatum/pkg/registry/git"
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
	next resolver.DependencyFetcher
}

// NewCacheFirstFetcher returns a DependencyFetcher that tries cache first, then next.
func NewCacheFirstFetcher(next resolver.DependencyFetcher) resolver.DependencyFetcher {
	return &cacheFirstFetcher{next: next}
}

func (f *cacheFirstFetcher) FetchManifest(ctx context.Context, dep artifact.Dependency) (*artifact.Manifest, error) {
	name, version, ok := depToCacheCandidate(dep)
	if !ok {
		return f.next.FetchManifest(ctx, dep)
	}
	m, err := loadCachedSkillManifest(name, version)
	if err != nil {
		return nil, err
	}
	if m != nil {
		return m, nil
	}
	m, err = f.next.FetchManifest(ctx, dep)
	if err != nil {
		return nil, fmt.Errorf("%s cache miss; remote fetch failed: %w", dep.CanonicalRef(), err)
	}
	return m, nil
}

// depToCacheCandidate derives a cache key (name, version) from a dependency.
// Only OCI dependencies can be mapped to name@version; Git deps return ok=false.
func depToCacheCandidate(dep artifact.Dependency) (name, version string, ok bool) {
	d, isOCI := dep.(*artifact.OCIDependency)
	if !isOCI {
		return "", "", false
	}
	repo := d.Repository
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		repo = repo[i+1:]
	}
	name = strings.TrimSpace(repo)
	version = strings.TrimSpace(d.Tag)
	return name, version, name != "" && version != ""
}

// defaultRouter returns a Router wired with production backends (OCI + Git).
// A new Router is created per call; this is fine because the backends are stateless.
func defaultRouter() *registry.Router {
	return &registry.Router{
		OCI: &registry.OCIRemoteBackend{},
		Git: &gitbackend.Backend{},
	}
}

// remoteFetcher fetches manifests from remote registries via the Router.
type remoteFetcher struct {
	router *registry.Router
}

// NewRemoteFetcher returns a DependencyFetcher that fetches from remote registries.
func NewRemoteFetcher() resolver.DependencyFetcher {
	return &remoteFetcher{router: defaultRouter()}
}

func (f *remoteFetcher) FetchManifest(ctx context.Context, dep artifact.Dependency) (*artifact.Manifest, error) {
	if dep == nil {
		return nil, fmt.Errorf("nil dependency")
	}
	return f.router.Inspect(ctx, dep)
}
