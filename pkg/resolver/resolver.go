package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// DependencyFetcher fetches a manifest for a given dependency locator.
type DependencyFetcher interface {
	FetchManifest(ctx context.Context, dep artifact.Dependency) (*artifact.Manifest, error)
}

// ResolvedArtifact is a single node in the resolved dependency tree.
type ResolvedArtifact struct {
	Name       string
	Version    string
	Dependency artifact.Dependency // nil for root
	Manifest   *artifact.Manifest
}

// Resolve walks the dependency tree starting at root, deduplicates by name@version,
// detects cycles, and returns an ordered list (root first, then deps in discovery order).
//
// Dedup strategy: each unique CanonicalRef is fetched at most once. Results are
// also deduped by name@version, so if two different sources (e.g. an OCI registry
// and a Git repo) both resolve to the same name@version, the first one encountered
// wins and the second is silently skipped. This "first wins" behavior is intentional
// to avoid duplicate artifacts in the resolved tree, but callers should be aware that
// it can mask conflicts if different backends publish different content under the
// same name@version.
func Resolve(ctx context.Context, root *artifact.Manifest, fetcher DependencyFetcher) ([]ResolvedArtifact, error) {
	if root == nil {
		return nil, errors.New("root manifest is nil")
	}
	if fetcher == nil {
		return nil, errors.New("fetcher is nil")
	}

	fetchedRefs := make(map[string]bool)
	visitedNV := make(map[string]bool)
	pathNV := make(map[string]bool)
	var result []ResolvedArtifact

	var walk func(m *artifact.Manifest, dep artifact.Dependency) error
	walk = func(m *artifact.Manifest, dep artifact.Dependency) error {
		if m == nil {
			return errors.New("manifest is nil")
		}
		nvKey := m.Metadata.Name + "@" + m.Metadata.Version
		if pathNV[nvKey] {
			return fmt.Errorf("cycle detected: %s", nvKey)
		}
		if visitedNV[nvKey] {
			return nil
		}
		visitedNV[nvKey] = true
		pathNV[nvKey] = true
		defer func() { delete(pathNV, nvKey) }()

		result = append(result, ResolvedArtifact{
			Name:       m.Metadata.Name,
			Version:    m.Metadata.Version,
			Dependency: dep,
			Manifest:   m,
		})

		for _, d := range m.Dependencies {
			refKey := d.CanonicalRef()
			if fetchedRefs[refKey] {
				continue
			}
			fetchedRefs[refKey] = true

			depManifest, err := fetcher.FetchManifest(ctx, d)
			if err != nil {
				return fmt.Errorf("fetch %s: %w", refKey, err)
			}
			if err := walk(depManifest, d); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(root, nil); err != nil {
		return nil, err
	}
	return result, nil
}
