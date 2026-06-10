package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

const (
	cacheDirName   = "cache"
	digestFileName = ".oci-digest"
)

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

// CacheDir returns the cache directory for the given kind/name@version.
func CacheDir(kind, name, version string) string {
	return filepath.Join(CacheRoot(), cacheDirName, kind, name+"@"+version)
}

// FindCacheDir probes all supported kind subdirectories for name@version
// and returns the first match. Use when the caller does not know the artifact kind.
func FindCacheDir(name, version string) (string, bool) {
	nv := name + "@" + version
	cacheRoot := filepath.Join(CacheRoot(), cacheDirName)
	for _, kind := range artifact.SupportedKinds() {
		candidate := filepath.Join(cacheRoot, kind, nv)
		if _, err := os.Stat(filepath.Join(candidate, "artifact.json")); err == nil {
			return candidate, true
		}
	}
	return "", false
}

// PullFunc is called to pull an artifact into outputDir.
type PullFunc func(ctx context.Context, outputDir string) error

// DigestFunc resolves the current remote OCI manifest digest.
// Returns (digest, nil) on success, ("", error) when the registry is unreachable.
type DigestFunc func(ctx context.Context) (string, error)

// ReadDigest returns the stored OCI manifest digest from cacheDir/.oci-digest.
// Returns ("", nil) when the file does not exist.
func ReadDigest(cacheDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(cacheDir, digestFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteDigest writes the OCI manifest digest to cacheDir/.oci-digest.
func WriteDigest(cacheDir, digest string) error {
	return os.WriteFile(filepath.Join(cacheDir, digestFileName), []byte(digest), 0o600)
}

// isCacheFresh checks whether the cached artifact matches the remote digest.
// Returns (true, digest) when fresh, (false, digest) when stale or unverifiable.
func isCacheFresh(ctx context.Context, cacheDir string, resolveDigest DigestFunc) (bool, string) {
	remoteDigest, err := resolveDigest(ctx)
	if err != nil {
		return false, ""
	}
	localDigest, err := ReadDigest(cacheDir)
	if err != nil || localDigest == "" {
		return false, remoteDigest
	}
	return localDigest == remoteDigest, remoteDigest
}

// EnsureInCache ensures the artifact is in cacheDir with digest verification when resolveDigest is provided.
// When resolveDigest is non-nil and the cache cannot be verified as fresh, the cache is discarded and re-pulled.
// When resolveDigest is nil, an existing cache is trusted without verification.
func EnsureInCache(ctx context.Context, cacheDir string, pull PullFunc, resolveDigest DigestFunc) error {
	manifestPath := filepath.Join(cacheDir, "artifact.json")
	_, statErr := os.Stat(manifestPath)

	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("stat cache dir %s: %w", cacheDir, statErr)
	}
	cacheExists := statErr == nil

	var remoteDigest string
	if cacheExists {
		if resolveDigest == nil {
			return nil
		}
		var fresh bool
		fresh, remoteDigest = isCacheFresh(ctx, cacheDir, resolveDigest)
		if fresh {
			return nil
		}
		if err := os.RemoveAll(cacheDir); err != nil {
			return fmt.Errorf("remove stale cache %s: %w", cacheDir, err)
		}
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	if err := pull(ctx, cacheDir); err != nil {
		return err
	}

	if remoteDigest == "" && resolveDigest != nil {
		if d, err := resolveDigest(ctx); err == nil {
			remoteDigest = d
		}
	}
	if remoteDigest != "" {
		_ = WriteDigest(cacheDir, remoteDigest)
	}
	return nil
}
