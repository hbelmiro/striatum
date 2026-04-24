package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheRoot_UsesSTRIATUM_HOME(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	got := CacheRoot()
	if got != dir {
		t.Errorf("CacheRoot() = %q, want %q", got, dir)
	}
}

func TestCacheRoot_DefaultsToHome(t *testing.T) {
	t.Setenv("STRIATUM_HOME", "") // clear if set
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got := CacheRoot()
	want := filepath.Join(home, ".striatum")
	if got != want {
		t.Errorf("CacheRoot() = %q, want %q", got, want)
	}
}

func TestCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	got := CacheDir("foo", "1.0.0")
	want := filepath.Join(dir, "cache", "foo@1.0.0")
	if got != want {
		t.Errorf("CacheDir(%q, %q) = %q, want %q", "foo", "1.0.0", got, want)
	}
}

func TestEnsureInCache_SkipsWhenArtifactJSONExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("skip", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if pullCalled {
		t.Error("pull should not be called when artifact.json exists")
	}
}

func TestEnsureInCache_CallsPullWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("missing", "1.0.0")
	var pullOutput string
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullOutput = outputDir
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, nil)
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if pullOutput != cacheDir {
		t.Errorf("pull called with %q, want %q", pullOutput, cacheDir)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "artifact.json")); err != nil {
		t.Errorf("artifact.json not created: %v", err)
	}
}

func TestEnsureInCache_PropagatesPullError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("err", "1.0.0")
	wantErr := os.ErrNotExist
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		return wantErr
	}, nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("EnsureInCache err = %v, want %v", err, wantErr)
	}
}

func TestWriteDigest_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	digest := "sha256:abc123def456"
	if err := WriteDigest(dir, digest); err != nil {
		t.Fatalf("WriteDigest err = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".oci-digest"))
	if err != nil {
		t.Fatalf("read .oci-digest: %v", err)
	}
	if got := string(data); got != digest {
		t.Errorf("digest file content = %q, want %q", got, digest)
	}
}

func TestReadDigest_ReturnsStoredDigest(t *testing.T) {
	dir := t.TempDir()
	digest := "sha256:xyz789"
	if err := WriteDigest(dir, digest); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDigest(dir)
	if err != nil {
		t.Fatalf("ReadDigest err = %v", err)
	}
	if got != digest {
		t.Errorf("ReadDigest = %q, want %q", got, digest)
	}
}

func TestReadDigest_ReturnsEmptyWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadDigest(dir)
	if err != nil {
		t.Fatalf("ReadDigest err = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("ReadDigest = %q, want empty string", got)
	}
}

func TestEnsureInCache_DigestMatch_SkipsPull(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("digest-match", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := "sha256:abc123"
	if err := WriteDigest(cacheDir, digest); err != nil {
		t.Fatal(err)
	}

	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return nil
	}, func(ctx context.Context) (string, error) {
		return digest, nil
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if pullCalled {
		t.Error("pull should not be called when digest matches")
	}
}

func TestEnsureInCache_DigestMismatch_Repulls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("digest-mismatch", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDigest(cacheDir, "sha256:old"); err != nil {
		t.Fatal(err)
	}

	newDigest := "sha256:new"
	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, func(ctx context.Context) (string, error) {
		return newDigest, nil
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called when digest mismatches")
	}

	storedDigest, err := ReadDigest(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if storedDigest != newDigest {
		t.Errorf("stored digest = %q, want %q", storedDigest, newDigest)
	}
}

func TestEnsureInCache_DigestResolveError_Repulls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("digest-error", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDigest(cacheDir, "sha256:cached"); err != nil {
		t.Fatal(err)
	}

	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, func(ctx context.Context) (string, error) {
		return "", fmt.Errorf("registry unreachable")
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called when digest resolve fails (when in doubt, discard)")
	}
}

func TestEnsureInCache_NoStoredDigest_Repulls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("no-digest", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	digest := "sha256:remote"
	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, func(ctx context.Context) (string, error) {
		return digest, nil
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called when no digest stored (when in doubt, discard)")
	}

	storedDigest, err := ReadDigest(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if storedDigest != digest {
		t.Errorf("stored digest = %q, want %q", storedDigest, digest)
	}
}

func TestEnsureInCache_NilDigestFunc_SkipsPull(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("nil-digest", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if pullCalled {
		t.Error("pull should not be called when DigestFunc is nil (no remote to check)")
	}
}

func TestEnsureInCache_CacheMiss_PullsAndWritesDigest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("cache-miss", "1.0.0")

	digest := "sha256:fresh"
	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, func(ctx context.Context) (string, error) {
		return digest, nil
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called on cache miss")
	}

	storedDigest, err := ReadDigest(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if storedDigest != digest {
		t.Errorf("stored digest = %q, want %q", storedDigest, digest)
	}
}

func TestEnsureInCache_CacheMiss_DigestFuncError_PullsNoDigest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("cache-miss-err", "1.0.0")

	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, func(ctx context.Context) (string, error) {
		return "", fmt.Errorf("registry down")
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called on cache miss even if DigestFunc fails")
	}

	storedDigest, err := ReadDigest(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if storedDigest != "" {
		t.Errorf("digest should not be stored when DigestFunc fails, got %q", storedDigest)
	}
}

func TestEnsureInCache_CacheMiss_NilDigestFunc_Pulls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("cache-miss-nil", "1.0.0")

	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, nil)
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called on cache miss")
	}

	storedDigest, err := ReadDigest(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if storedDigest != "" {
		t.Errorf("digest should not be stored when DigestFunc is nil, got %q", storedDigest)
	}
}

func TestEnsureInCache_TransientDigestError_RetrySucceeds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("transient", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDigest(cacheDir, "sha256:cached"); err != nil {
		t.Fatal(err)
	}

	callCount := 0
	digest := "sha256:recovered"
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, func(ctx context.Context) (string, error) {
		callCount++
		if callCount == 1 {
			return "", fmt.Errorf("transient network error")
		}
		return digest, nil
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if callCount != 2 {
		t.Errorf("DigestFunc should be called twice (first fails, retry succeeds), got %d", callCount)
	}

	storedDigest, err := ReadDigest(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if storedDigest != digest {
		t.Errorf("stored digest = %q, want %q", storedDigest, digest)
	}
}

func TestEnsureInCache_IncompleteCacheDir_Repulls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("incomplete", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, nil)
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called when cache dir exists but artifact.json is missing")
	}
}

func TestEnsureInCache_DigestFileCorrupted_Repulls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("corrupt-digest", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	digestPath := filepath.Join(cacheDir, ".oci-digest")
	if err := os.Mkdir(digestPath, 0o755); err != nil {
		t.Fatal(err)
	}

	digest := "sha256:fresh"
	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, func(ctx context.Context) (string, error) {
		return digest, nil
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called when .oci-digest is unreadable")
	}

	storedDigest, err := ReadDigest(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if storedDigest != digest {
		t.Errorf("stored digest = %q, want %q", storedDigest, digest)
	}
}

func TestReadDigest_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	digestWithWhitespace := "sha256:abc\n  "
	if err := os.WriteFile(filepath.Join(dir, ".oci-digest"), []byte(digestWithWhitespace), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDigest(dir)
	if err != nil {
		t.Fatalf("ReadDigest err = %v", err)
	}
	want := "sha256:abc"
	if got != want {
		t.Errorf("ReadDigest = %q, want %q (whitespace not trimmed)", got, want)
	}
}

func TestEnsureInCache_StatPermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission tests do not work as root")
	}
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("stat-err", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(cacheDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(cacheDir, 0o755)
	})

	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		return nil
	}, nil)
	if err == nil {
		t.Fatal("EnsureInCache with permission error: expected error")
	}
	if !strings.Contains(err.Error(), "stat cache dir") {
		t.Errorf("error should mention stat cache dir: %v", err)
	}
}

func TestEnsureInCache_RemoveStaleCacheFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission tests do not work as root")
	}
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("remove-err", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDigest(cacheDir, "sha256:old"); err != nil {
		t.Fatal(err)
	}

	parent := filepath.Dir(cacheDir)
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(parent, 0o755)
	})

	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, func(ctx context.Context) (string, error) {
		return "sha256:new", nil
	})
	if err == nil {
		t.Fatal("EnsureInCache with RemoveAll failure: expected error")
	}
	if !strings.Contains(err.Error(), "remove stale cache") {
		t.Errorf("error should mention remove stale cache: %v", err)
	}
}

func TestEnsureInCache_MkdirAllFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission tests do not work as root")
	}
	baseDir := t.TempDir()
	t.Setenv("STRIATUM_HOME", baseDir)

	parent := filepath.Join(baseDir, "readonly-parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(parent, 0o755)
	})

	cacheDir := filepath.Join(parent, "cache", "mkdir-err@1.0.0")

	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, nil)
	if err == nil {
		t.Fatal("EnsureInCache with MkdirAll failure: expected error")
	}
	if !strings.Contains(err.Error(), "create cache dir") {
		t.Errorf("error should mention create cache dir: %v", err)
	}
}

func TestEnsureInCache_WriteDigestFailure_PullStillSucceeds(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission tests do not work as root")
	}
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("write-digest-fail", "1.0.0")

	digest := "sha256:test"
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		if err := os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
			return err
		}
		return os.Chmod(outputDir, 0o555)
	}, func(ctx context.Context) (string, error) {
		return digest, nil
	})

	t.Cleanup(func() {
		_ = os.Chmod(cacheDir, 0o755)
	})

	if err != nil {
		t.Fatalf("EnsureInCache should succeed even if WriteDigest fails, got err: %v", err)
	}

	digestPath := filepath.Join(cacheDir, ".oci-digest")
	if _, statErr := os.Stat(digestPath); !os.IsNotExist(statErr) {
		t.Errorf(".oci-digest should not exist when WriteDigest fails, stat err: %v", statErr)
	}
}
