package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// ManifestFetcher fetches a manifest by reference (e.g. "host/repo/name:version").
type ManifestFetcher interface {
	FetchManifest(ctx context.Context, reference string) (*artifact.Manifest, error)
}

// ResolvedArtifact is a single node in the resolved dependency tree.
type ResolvedArtifact struct {
	Name     string
	Version  string
	Registry string
	Manifest *artifact.Manifest
}

// Resolve walks the dependency tree starting at root, deduplicates by name@version,
// detects cycles, and returns an ordered list (root first, then deps in discovery order).
// defaultRegistry is used for the root and for dependencies that do not specify Registry.
// Empty defaultRegistry with a root that has dependencies returns an error.
func Resolve(ctx context.Context, root *artifact.Manifest, defaultRegistry string, fetcher ManifestFetcher) ([]ResolvedArtifact, error) {
	if root == nil {
		return nil, errors.New("root manifest is nil")
	}
	if fetcher == nil {
		return nil, errors.New("fetcher is nil")
	}
	if len(root.Dependencies) > 0 && strings.TrimSpace(defaultRegistry) == "" {
		return nil, errors.New("default registry is required when root has dependencies")
	}

	registry := strings.TrimSuffix(strings.TrimSpace(defaultRegistry), "/")
	visited := make(map[string]bool) // "name@version" -> true
	path := make(map[string]bool)    // "name@version" on current path (for cycle detection)
	var result []ResolvedArtifact

	var walk func(m *artifact.Manifest, reg string) error
	walk = func(m *artifact.Manifest, reg string) error {
		if m == nil {
			return errors.New("manifest is nil")
		}
		key := m.Metadata.Name + "@" + m.Metadata.Version
		if path[key] {
			return fmt.Errorf("cycle detected: %s", key)
		}
		if visited[key] {
			return nil
		}
		visited[key] = true
		path[key] = true
		defer func() { delete(path, key) }()

		result = append(result, ResolvedArtifact{
			Name:     m.Metadata.Name,
			Version:  m.Metadata.Version,
			Registry: reg,
			Manifest: m,
		})

		for _, d := range m.Dependencies {
			depKey := d.Name + "@" + d.Version
			if path[depKey] {
				return fmt.Errorf("cycle detected: %s", depKey)
			}
			if visited[depKey] {
				continue
			}
			depReg := d.Registry
			if depReg == "" {
				depReg = reg
			} else {
				depReg = strings.TrimSuffix(strings.TrimSpace(depReg), "/")
			}
			ref := depReg + "/" + d.Name + ":" + d.Version
			depManifest, err := fetcher.FetchManifest(ctx, ref)
			if err != nil {
				return fmt.Errorf("fetch %s: %w", ref, err)
			}
			if err := walk(depManifest, depReg); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(root, registry); err != nil {
		return nil, err
	}
	return result, nil
}
