package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClearCache removes the entire local cache directory (~/.striatum/cache),
// including self-contained payloads and .stale-* recovery backups, and
// returns the number of cache entries removed (.stale-* backups are removed
// but not counted). A missing cache directory is not an error (returns 0).
// When the cache path is a symlink, the target directory is removed.
func ClearCache() (int, error) {
	cachePath := filepath.Join(CacheRoot(), cacheDirName)

	// Follow a symlinked cache path so the real cache directory is removed,
	// not just the link.
	if resolved, err := filepath.EvalSymlinks(cachePath); err == nil {
		cachePath = resolved
	} else if !os.IsNotExist(err) {
		return 0, fmt.Errorf("resolve cache dir %s: %w", cachePath, err)
	}

	entries, err := countCacheEntries(cachePath)
	if err != nil {
		return 0, fmt.Errorf("count cache entries: %w", err)
	}

	if err := os.RemoveAll(cachePath); err != nil {
		return 0, fmt.Errorf("remove cache dir %s: %w", cachePath, err)
	}
	return entries, nil
}

// countCacheEntries counts the name@version directories under every
// top-level subdirectory of cachePath (kind dirs and self-contained).
// It uses os.Stat so symlinked entries are followed, and skips .stale-*
// recovery backups, which are cache junk, not artifacts.
func countCacheEntries(cachePath string) (int, error) {
	kinds, err := os.ReadDir(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read cache dir %s: %w", cachePath, err)
	}
	count := 0
	for _, kind := range kinds {
		kindPath := filepath.Join(cachePath, kind.Name())
		info, err := os.Stat(kindPath)
		if err != nil {
			// Unstatable (e.g. a broken symlink): cache junk, removed but
			// not counted.
			continue
		}
		if !info.IsDir() {
			continue
		}
		sub, err := os.ReadDir(kindPath)
		if err != nil {
			return 0, fmt.Errorf("read cache dir %s: %w", kindPath, err)
		}
		for _, e := range sub {
			if strings.HasPrefix(e.Name(), ".stale-") {
				continue
			}
			entryPath := filepath.Join(kindPath, e.Name())
			entryInfo, err := os.Stat(entryPath)
			if err != nil {
				// Unstatable entry: removed but not counted.
				continue
			}
			if entryInfo.IsDir() {
				count++
			}
		}
	}
	return count, nil
}
