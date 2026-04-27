package resolver

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

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

// versionOrigin tracks which parent requested a particular version of a name.
type versionOrigin struct {
	version  string
	parentNV string // name@version of the requesting parent; empty for root
}

// Resolve walks the dependency tree starting at root, deduplicates by name@version,
// detects cycles and version conflicts, and returns an ordered list (root first,
// then deps in discovery order).
//
// Dedup strategy: each unique CanonicalRef is fetched at most once (cached).
// Results are also deduped by name@version, so if two different sources
// (e.g. an OCI registry and a Git repo) both resolve to the same name@version,
// the first one encountered wins. This "first wins" behavior is intentional
// to avoid duplicate artifacts in the resolved tree, but callers should be aware
// that it can mask differences if different backends publish different content
// under the same name@version.
//
// If the resolved tree contains the same skill name at two or more different
// versions, Resolve returns an error listing the conflicting versions and which
// parent artifacts requested each one.
func Resolve(ctx context.Context, root *artifact.Manifest, fetcher DependencyFetcher) ([]ResolvedArtifact, error) {
	if root == nil {
		return nil, errors.New("root manifest is nil")
	}
	if fetcher == nil {
		return nil, errors.New("fetcher is nil")
	}

	fetchCache := make(map[string]*artifact.Manifest)
	visitedNV := make(map[string]bool)
	pathNV := make(map[string]bool)
	nameOrigins := make(map[string][]versionOrigin)
	var result []ResolvedArtifact

	var walk func(m *artifact.Manifest, dep artifact.Dependency, parentNV string) error
	walk = func(m *artifact.Manifest, dep artifact.Dependency, parentNV string) error {
		if m == nil {
			return errors.New("manifest is nil")
		}
		nvKey := m.Metadata.Name + "@" + m.Metadata.Version
		if pathNV[nvKey] {
			return fmt.Errorf("cycle detected: %s", nvKey)
		}

		// Record origin before dedup so all parents are tracked for conflict detection.
		nameOrigins[m.Metadata.Name] = append(nameOrigins[m.Metadata.Name], versionOrigin{
			version:  m.Metadata.Version,
			parentNV: parentNV,
		})

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
			depManifest, ok := fetchCache[refKey]
			if !ok {
				var err error
				depManifest, err = fetcher.FetchManifest(ctx, d)
				if err != nil {
					return fmt.Errorf("fetch %s: %w", refKey, err)
				}
				fetchCache[refKey] = depManifest
			}
			if err := walk(depManifest, d, nvKey); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(root, nil, ""); err != nil {
		return nil, err
	}

	// Detect version conflicts: same name resolved at different versions.
	if conflicts := detectConflicts(nameOrigins); conflicts != "" {
		return nil, fmt.Errorf("dependency version conflict:\n%s", conflicts)
	}

	return result, nil
}

// detectConflicts checks nameOrigins for any name that has more than one distinct version.
// Returns a formatted error string or "" if no conflicts.
func detectConflicts(nameOrigins map[string][]versionOrigin) string {
	var conflicting []string

	for name, origins := range nameOrigins {
		versions := make(map[string][]string) // version -> list of parents
		for _, o := range origins {
			parent := o.parentNV
			if parent == "" {
				parent = "(root)"
			}
			versions[o.version] = append(versions[o.version], parent)
		}
		if len(versions) <= 1 {
			continue
		}

		// Sort version keys for deterministic output
		versionKeys := make([]string, 0, len(versions))
		for v := range versions {
			versionKeys = append(versionKeys, v)
		}
		sort.Strings(versionKeys)

		var parts []string
		for _, v := range versionKeys {
			parents := dedup(versions[v])
			sort.Strings(parents)
			parts = append(parts, fmt.Sprintf("  %s@%s (required by %s)", name, v, strings.Join(parents, ", ")))
		}
		conflicting = append(conflicting, strings.Join(parts, "\n"))
	}

	if len(conflicting) == 0 {
		return ""
	}

	sort.Strings(conflicting)
	return strings.Join(conflicting, "\n")
}

func dedup(s []string) []string {
	seen := make(map[string]bool, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
