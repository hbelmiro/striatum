package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// CachedArtifact describes an artifact present in the local cache.
type CachedArtifact struct {
	Name        string
	Version     string
	Kind        string
	Description string
}

// ListCachedArtifacts returns all artifacts in the cache.
// The cache layout is cache/<kind>/<name>@<version>/ with an artifact.json inside.
// Returns a non-nil empty slice and nil error when cache dir is missing or empty.
func ListCachedArtifacts() ([]CachedArtifact, error) {
	cacheRoot := filepath.Join(CacheRoot(), cacheDirName)
	kindDirs, err := os.ReadDir(cacheRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []CachedArtifact{}, nil
		}
		return nil, fmt.Errorf("read cache dir %s: %w", cacheRoot, err)
	}
	var result []CachedArtifact
	for _, kd := range kindDirs {
		if !kd.IsDir() {
			continue
		}
		kindPath := filepath.Join(cacheRoot, kd.Name())
		entries, err := os.ReadDir(kindPath)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			i := strings.LastIndex(name, "@")
			if i < 0 {
				continue
			}
			artName, version := name[:i], name[i+1:]
			if strings.TrimSpace(artName) == "" || version == "" {
				continue
			}
			dirPath := filepath.Join(kindPath, name)
			manifestPath := filepath.Join(dirPath, "artifact.json")
			if _, err := os.Stat(manifestPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("stat manifest %s: %w", manifestPath, err)
			}
			m, err := artifact.Load(manifestPath)
			if err != nil {
				continue
			}
			if !artifact.IsSupportedKind(m.Kind) {
				continue
			}
			result = append(result, CachedArtifact{Name: artName, Version: version, Kind: m.Kind, Description: m.Metadata.Description})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Version < result[j].Version
	})
	if result == nil {
		result = []CachedArtifact{}
	}
	return result, nil
}
