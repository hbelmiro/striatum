package installer

import (
	"context"
	"os"
	"path/filepath"
)

const cacheDirName = "cache"

// CacheRoot returns the striatum config root (~/.striatum or STRIATUM_HOME).
// If STRIATUM_HOME is unset and os.UserHomeDir() fails, it returns ".striatum"
// (relative to the current working directory). Callers in constrained environments
// should set STRIATUM_HOME explicitly.
func CacheRoot() string {
	if s := os.Getenv("STRIATUM_HOME"); s != "" {
		return s
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".striatum")
}

// CacheDir returns the cache directory for the given name@version.
func CacheDir(name, version string) string {
	return filepath.Join(CacheRoot(), cacheDirName, name+"@"+version)
}

// PullFunc is called to pull an artifact into outputDir.
type PullFunc func(ctx context.Context, outputDir string) error

// EnsureInCache ensures the artifact is in cacheDir. If artifact.json already exists there, skip pull.
// Only pulls when the manifest is missing (os.IsNotExist); other Stat errors (e.g. permission) are returned.
func EnsureInCache(ctx context.Context, cacheDir string, pull PullFunc) error {
	manifestPath := filepath.Join(cacheDir, "artifact.json")
	_, err := os.Stat(manifestPath)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	return pull(ctx, cacheDir)
}
