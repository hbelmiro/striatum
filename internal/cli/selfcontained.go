package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	gitbackend "github.com/hbelmiro/striatum/pkg/registry/git"
	"github.com/hbelmiro/striatum/pkg/resolver"
	"github.com/hbelmiro/striatum/pkg/selfcontained"
)

// prepareSelfContained prepares the self-contained image of the root Skill
// manifest: resolves the dependency closure (cache-first), pulls missing
// dependencies into the ordinary cache, stages the transformed tree, and
// validates it against the self-contained image consumer constraints. The caller must call
// Prepared.Cleanup when done.
func prepareSelfContained(ctx context.Context, m *artifact.Manifest, projectRoot string) (*selfcontained.Prepared, error) {
	var fetcher resolver.DependencyFetcher
	if len(m.Dependencies) > 0 {
		fetcher = NewCacheFirstFetcher(NewRemoteFetcher())
	}
	cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
	mat := func(ctx context.Context, r resolver.ResolvedArtifact) (selfcontained.DepMaterialization, error) {
		cacheDir := installer.CacheDir(resolvedKind(r), r.Name, r.Version)
		pullFn := func(ctx context.Context, _ string) error {
			created, cleanup, err := pullToStagingDir(cacheRoot, r.Name, func(stagingDir string) error {
				// Pinned OCI dependencies are fetched by digest so the
				// image always bundles the pinned content, even if the
				// mutable tag has moved.
				if ociDep, ok := r.Dependency.(*artifact.OCIDependency); ok && ociDep.Digest != "" {
					reg, err := oci.NewRepository(ociDep.RegistryHost + "/" + ociDep.Repository)
					if err != nil {
						return err
					}
					return oci.Pull(ctx, reg, ociDep.Digest, stagingDir)
				}
				return pullDependency(ctx, r.Dependency, stagingDir)
			})
			if err != nil {
				cleanup()
				return err
			}
			defer cleanup()
			return atomicReplaceCacheDir(created, cacheDir)
		}
		// Pinned dependencies always verify the cache against the declared
		// pin (a warm cache holding different content is re-pulled by
		// digest). Unpinned dependencies trust a warm cache hit (the closure
		// was resolved cache-first, and offline preparation keeps working);
		// only a cache miss resolves the current tag digest.
		var digestFn installer.DigestFunc
		_, statErr := os.Stat(filepath.Join(cacheDir, "artifact.json"))
		cacheHit := statErr == nil
		ociDep, isOCI := r.Dependency.(*artifact.OCIDependency)
		gitDep, isGit := r.Dependency.(*artifact.GitDependency)
		isPinned := (isOCI && ociDep.Digest != "") || (isGit && gitDep.Commit != "")
		if isPinned || !cacheHit {
			switch d := r.Dependency.(type) {
			case *artifact.OCIDependency:
				captured := d
				digestFn = func(ctx context.Context) (string, error) {
					if captured.Digest != "" {
						return captured.Digest, nil // honor the declared pin
					}
					reg, err := oci.NewRepository(captured.RegistryHost + "/" + captured.Repository)
					if err != nil {
						return "", err
					}
					return oci.ResolveDigest(ctx, reg, captured.Tag)
				}
			case *artifact.GitDependency:
				captured := d
				gitBack := &gitbackend.Backend{}
				digestFn = func(ctx context.Context) (string, error) {
					if captured.Commit != "" {
						return captured.Commit, nil // honor the declared pin
					}
					return gitBack.ResolveCommit(ctx, captured)
				}
			}
		}
		if err := installer.EnsureInCache(ctx, cacheDir, pullFn, digestFn); err != nil {
			return selfcontained.DepMaterialization{}, err
		}
		src := selfcontained.ResolvedSource{}
		switch d := r.Dependency.(type) {
		case *artifact.OCIDependency:
			src = selfcontained.ResolvedSource{
				Type:   "oci",
				Ref:    d.RegistryHost + "/" + d.Repository + ":" + d.Tag,
				Digest: d.Digest,
			}
		case *artifact.GitDependency:
			src = selfcontained.ResolvedSource{
				Type:   "git",
				URL:    d.URL,
				Ref:    d.Ref,
				Commit: d.Commit,
			}
		}
		// Record the identity actually materialized (the cache's digest file
		// holds the resolved digest even for unpinned dependencies).
		if digest, err := installer.ReadDigest(cacheDir); err == nil && digest != "" {
			switch src.Type {
			case "oci":
				src.Digest = digest
			case "git":
				src.Commit = digest
			}
		}
		return selfcontained.DepMaterialization{CacheDir: cacheDir, Source: src}, nil
	}
	return selfcontained.Prepare(ctx, selfcontained.Options{
		Manifest:       m,
		ProjectRoot:    projectRoot,
		Fetcher:        fetcher,
		MaterializeDep: mat,
		StagingParent:  cacheRoot,
	})
}

// preparedImageFrom builds the oci.PreparedImage for a self-contained image: the manifest
// config with the self-contained image marker, one layer per staged file, and the manifest
// annotation marking the image as a self-contained image.
func preparedImageFrom(m *artifact.Manifest, p *selfcontained.Prepared) (oci.PreparedImage, error) {
	config, err := selfcontained.ConfigWithMarker(m)
	if err != nil {
		return oci.PreparedImage{}, fmt.Errorf("build self-contained config: %w", err)
	}
	return oci.PreparedImage{
		Config:              config,
		ArtifactType:        oci.ArtifactTypeForKind(m.Kind),
		Dir:                 p.Dir,
		Files:               p.Files,
		ManifestAnnotations: map[string]string{selfcontained.MarkerKey: selfcontained.MarkerValue},
	}, nil
}
