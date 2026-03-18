package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// CachedSkill describes a skill present in the local cache.
type CachedSkill struct {
	Name        string
	Version     string
	Description string
}

// ListCachedSkills returns all skills in the cache (directories name@version with artifact.json).
// Returns a non-nil empty slice and nil error when cache dir is missing or empty.
func ListCachedSkills() ([]CachedSkill, error) {
	cacheRoot := filepath.Join(CacheRoot(), cacheDirName)
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []CachedSkill{}, nil
		}
		return nil, fmt.Errorf("read cache dir %s: %w", cacheRoot, err)
	}
	var result []CachedSkill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		i := strings.LastIndex(name, "@")
		if i < 0 {
			continue
		}
		skillName, version := name[:i], name[i+1:]
		if strings.TrimSpace(skillName) == "" || version == "" {
			continue
		}
		dirPath := filepath.Join(cacheRoot, name)
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
		if m.Kind != "Skill" {
			continue
		}
		result = append(result, CachedSkill{Name: skillName, Version: version, Description: m.Metadata.Description})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Version < result[j].Version
	})
	if result == nil {
		result = []CachedSkill{}
	}
	return result, nil
}
