package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const cacheDirName = "cache"

// digestFileName is the name of the file that stores the OCI manifest digest
// for a cached artifact, used to detect stale cache entries.
const digestFileName = ".striatum-digest"

// CacheRoot returns the striatum config root (~/.striatum or STRIATUM_HOME).
// If STRIATUM_HOME is unset and os.UserHomeDir() fails, it falls back to ".striatum"
// (relative to the current working directory) and prints a warning to stderr.
// Callers in constrained environments should set STRIATUM_HOME explicitly.
func CacheRoot() string {
	if s := os.Getenv("STRIATUM_HOME"); s != "" {
		return s
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Warning: cannot determine home directory; using .striatum in current directory. Set STRIATUM_HOME to override.")
		return ".striatum"
	}
	return filepath.Join(home, ".striatum")
}

// CacheDir returns the cache directory for the given name@version.
func CacheDir(name, version string) string {
	return filepath.Join(CacheRoot(), cacheDirName, name+"@"+version)
}

// WriteDigest writes the OCI manifest digest to the cache directory.
func WriteDigest(cacheDir, digest string) error {
	return os.WriteFile(filepath.Join(cacheDir, digestFileName), []byte(digest), 0o600)
}

// ReadDigest reads the stored OCI manifest digest from the cache directory.
// Returns the digest and true on success, or empty string and false if the file
// is missing, empty, or unreadable.
func ReadDigest(cacheDir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(cacheDir, digestFileName))
	if err != nil {
		return "", false
	}
	d := strings.TrimSpace(string(data))
	if d == "" {
		return "", false
	}
	return d, true
}

// PullFunc is called to pull an artifact into outputDir.
type PullFunc func(ctx context.Context, outputDir string) error

// EnsureInCache ensures the artifact is in cacheDir. If artifact.json already exists
// there and remoteDigest is empty or matches the stored digest, skip pull.
// When remoteDigest is non-empty and differs from (or is absent in) the stored digest,
// the stale cache entry is removed and the artifact is re-pulled.
// Only pulls when the manifest is missing (os.IsNotExist); other Stat errors
// (e.g. permission) are returned.
func EnsureInCache(ctx context.Context, cacheDir string, remoteDigest string, pull PullFunc) error {
	manifestPath := filepath.Join(cacheDir, "artifact.json")
	_, err := os.Stat(manifestPath)
	if err == nil {
		if remoteDigest != "" {
			stored, ok := ReadDigest(cacheDir)
			if !ok || stored != remoteDigest {
				// Digest mismatch or missing — invalidate stale cache.
				if rmErr := os.RemoveAll(cacheDir); rmErr != nil {
					return fmt.Errorf("remove stale cache %s: %w", cacheDir, rmErr)
				}
				// Fall through to pull below.
			} else {
				return nil // digest matches, cache is fresh
			}
		} else {
			return nil // no remote digest to check, trust cache
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat cache dir %s: %w", cacheDir, err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	if pullErr := pull(ctx, cacheDir); pullErr != nil {
		return pullErr
	}
	if remoteDigest != "" {
		// Best-effort: store digest for future freshness checks.
		_ = WriteDigest(cacheDir, remoteDigest)
	}
	return nil
}
