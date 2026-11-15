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
// manifest: resolves the dependency closure, pulls missing dependencies
// into the ordinary cache, stages the transformed tree, and validates it
// against the self-contained image consumer constraints. The caller must
// call Prepared.Cleanup when done.
func prepareSelfContained(ctx context.Context, m *artifact.Manifest, projectRoot string) (*selfcontained.Prepared, error) {
	var fetcher resolver.DependencyFetcher
	if len(m.Dependencies) > 0 {
		// Pinned OCI dependencies are inspected by their declared digest so
		// the closure traversal (file lists, transitive dependencies) matches
		// the pinned image even if the mutable tag has moved.
		fetcher = &pinAwareFetcher{next: NewCacheFirstFetcher(NewRemoteFetcher())}
	}
	cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
	mat := func(ctx context.Context, r resolver.ResolvedArtifact) (selfcontained.DepMaterialization, error) {
		cacheDir := installer.CacheDir(resolvedKind(r), r.Name, r.Version)
		_, statErr := os.Stat(filepath.Join(cacheDir, "artifact.json"))
		cacheHit := statErr == nil
		ociDep, isOCI := r.Dependency.(*artifact.OCIDependency)
		gitDep, isGit := r.Dependency.(*artifact.GitDependency)

		// Resolve the dependency's immutable identity once, up front: the
		// declared pin when set, otherwise (cold cache only) the current tag
		// or branch head. Extraction, the cache digest, and provenance all
		// use this identity, so a tag moving mid-flight cannot desynchronize
		// them. An unpinned warm cache is trusted as-is (offline preparation
		// keeps working).
		var identity string
		switch {
		case isOCI && ociDep.Digest != "":
			identity = ociDep.Digest
		case isGit && gitDep.Commit != "":
			identity = gitDep.Commit
		case isOCI && !cacheHit:
			reg, err := oci.NewRepository(ociDep.RegistryHost + "/" + ociDep.Repository)
			if err != nil {
				return selfcontained.DepMaterialization{}, err
			}
			identity, err = oci.ResolveDigest(ctx, reg, ociDep.Tag)
			if err != nil {
				return selfcontained.DepMaterialization{}, err
			}
		case isGit && !cacheHit:
			gitBack := &gitbackend.Backend{}
			var err error
			identity, err = gitBack.ResolveCommit(ctx, gitDep)
			if err != nil {
				return selfcontained.DepMaterialization{}, err
			}
		}

		pullFn := func(ctx context.Context, _ string) error {
			// pullFn only runs on a cache miss or a pin mismatch, both of
			// which resolve identity above; pull by it so the extracted
			// bytes match the recorded digest.
			created, cleanup, err := pullToStagingDir(cacheRoot, r.Name, func(stagingDir string) error {
				if isOCI {
					reg, err := oci.NewRepository(ociDep.RegistryHost + "/" + ociDep.Repository)
					if err != nil {
						return err
					}
					return oci.Pull(ctx, reg, identity, stagingDir)
				}
				// Pull the pinned/resolved commit exactly.
				if gitDep != nil {
					at := *gitDep
					at.Commit = identity
					return pullDependency(ctx, &at, stagingDir)
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
		var digestFn installer.DigestFunc
		if identity != "" {
			digestFn = func(context.Context) (string, error) { return identity, nil }
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

// pinAwareFetcher inspects pinned OCI dependencies by their declared digest
// instead of the mutable tag, so closure traversal always follows the
// pinned manifests; everything else delegates to the wrapped fetcher.
type pinAwareFetcher struct {
	next resolver.DependencyFetcher
}

func (f *pinAwareFetcher) FetchManifest(ctx context.Context, dep artifact.Dependency) (*artifact.Manifest, error) {
	if ociDep, ok := dep.(*artifact.OCIDependency); ok && ociDep.Digest != "" {
		reg, err := oci.NewRepository(ociDep.RegistryHost + "/" + ociDep.Repository)
		if err == nil {
			if m, inspectErr := oci.Inspect(ctx, reg, ociDep.Digest); inspectErr == nil {
				return m, nil
			}
		}
		// Best-effort fallback to the wrapped (cache-first) fetcher so
		// offline preparation with a warm cache still traverses the closure;
		// the materializer verifies the payload against the pin regardless.
	}
	return f.next.FetchManifest(ctx, dep)
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
