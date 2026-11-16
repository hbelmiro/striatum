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
	got := CacheDir("Skill", "foo", "1.0.0")
	want := filepath.Join(dir, "cache", "Skill", "foo@1.0.0")
	if got != want {
		t.Errorf("CacheDir(%q, %q, %q) = %q, want %q", "Skill", "foo", "1.0.0", got, want)
	}
}

func TestFindCacheDir_Found(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("Prompt", "my-prompt", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok, err := FindCacheDir("my-prompt", "1.0.0")
	if err != nil {
		t.Fatalf("FindCacheDir err = %v", err)
	}
	if !ok {
		t.Fatal("FindCacheDir should find existing entry")
	}
	if got != cacheDir {
		t.Errorf("FindCacheDir = %q, want %q", got, cacheDir)
	}
}

func TestFindCacheDir_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	_, ok, err := FindCacheDir("nonexistent", "1.0.0")
	if err != nil {
		t.Fatalf("FindCacheDir err = %v", err)
	}
	if ok {
		t.Fatal("FindCacheDir should return false for missing entry")
	}
}

func TestFindCacheDir_Ambiguous(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	for _, kind := range []string{"Prompt", "Skill"} {
		cacheDir := CacheDir(kind, "foo", "1.0.0")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := FindCacheDir("foo", "1.0.0")
	if err == nil {
		t.Fatal("FindCacheDir should return error when multiple kinds match")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should mention ambiguous: %v", err)
	}
}

func TestEnsureInCache_SkipsWhenArtifactJSONExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("Skill", "skip", "1.0.0")
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
	cacheDir := CacheDir("Skill", "missing", "1.0.0")
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
	cacheDir := CacheDir("Skill", "err", "1.0.0")
	wantErr := os.ErrNotExist
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		return wantErr
	}, nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("EnsureInCache err = %v, want %v", err, wantErr)
	}
}

// A stale cache entry must survive a failed re-pull: the user keeps the
// previously usable content (e.g. an offline run against a moved tag).
func TestEnsureInCache_FailedRepullPreservesStaleCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("Skill", "stale", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte(`{"kind":"Skill"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDigest(cacheDir, "sha256:old"); err != nil {
		t.Fatal(err)
	}

	// The remote digest differs from the stored one (stale), and the
	// re-pull fails (e.g. offline).
	digestFn := func(context.Context) (string, error) { return "sha256:new", nil }
	pullErr := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		return errors.New("registry unreachable")
	}, digestFn)
	if pullErr == nil || pullErr.Error() != "registry unreachable" {
		t.Fatalf("EnsureInCache err = %v, want %v", pullErr, errors.New("registry unreachable"))
	}

	// The stale entry is intact.
	content, err := os.ReadFile(filepath.Join(cacheDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("stale cache entry destroyed by failed re-pull: %v", err)
	}
	if string(content) != "old content" {
		t.Fatalf("stale cache content changed: %q", content)
	}
	digest, err := ReadDigest(cacheDir)
	if err != nil || digest != "sha256:old" {
		t.Errorf("stored digest = %q (%v), want sha256:old", digest, err)
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
	cacheDir := CacheDir("Skill", "digest-match", "1.0.0")
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
	cacheDir := CacheDir("Skill", "digest-mismatch", "1.0.0")
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
	cacheDir := CacheDir("Skill", "digest-error", "1.0.0")
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
	cacheDir := CacheDir("Skill", "no-digest", "1.0.0")
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
	cacheDir := CacheDir("Skill", "nil-digest", "1.0.0")
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
	cacheDir := CacheDir("Skill", "cache-miss", "1.0.0")

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
	cacheDir := CacheDir("Skill", "cache-miss-err", "1.0.0")

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
	cacheDir := CacheDir("Skill", "cache-miss-nil", "1.0.0")

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
	cacheDir := CacheDir("Skill", "transient", "1.0.0")
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
	cacheDir := CacheDir("Skill", "incomplete", "1.0.0")
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
	cacheDir := CacheDir("Skill", "corrupt-digest", "1.0.0")
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
	cacheDir := CacheDir("Skill", "stat-err", "1.0.0")
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
	cacheDir := CacheDir("Skill", "remove-err", "1.0.0")
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
		t.Fatal("EnsureInCache with stale-cache move failure: expected error")
	}
	if !strings.Contains(err.Error(), "set aside stale cache") {
		t.Errorf("error should mention set aside stale cache: %v", err)
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
	cacheDir := CacheDir("Skill", "write-digest-fail", "1.0.0")

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

// A successful re-pull must not leave the .stale-* backup behind: leaked
// backups sit inside cache/<kind>/ and would be listed as real artifacts.
func TestEnsureInCache_SuccessfulRepullRemovesBackup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("Skill", "replaced", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDigest(cacheDir, "sha256:old"); err != nil {
		t.Fatal(err)
	}

	digestFn := func(context.Context) (string, error) { return "sha256:new", nil }
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	}, digestFn)
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(cacheDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".stale-") {
			t.Errorf("backup %q leaked after successful re-pull", e.Name())
		}
	}
}

// restoreBackup must preserve the backup on failure (the warning tells the
// user it was kept) and report success when the rename works.
func TestRestoreBackup(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup")
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Failure: the restore target exists as a non-empty directory.
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "blocker"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restoreBackup(backup, target); err == nil {
		t.Error("restoreBackup over an existing non-empty dir: expected error")
	}
	if _, err := os.Stat(filepath.Join(backup, "keep.txt")); err != nil {
		t.Errorf("backup destroyed on failed restore: %v", err)
	}

	// Success: renaming into an empty target works and consumes the backup.
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	if err := restoreBackup(backup, target); err != nil {
		t.Fatalf("restoreBackup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "keep.txt")); err != nil {
		t.Errorf("restored content missing: %v", err)
	}
}

// When a failed re-pull cannot restore the stale entry (e.g. the cache
// parent becomes unwritable), the backup must survive: the warning names it
// as kept for manual recovery, and nothing may delete it afterward.
func TestEnsureInCache_FailedRestoreKeepsBackup(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission tests do not work as root")
	}
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("Skill", "keepme", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDigest(cacheDir, "sha256:old"); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(cacheDir)

	digestFn := func(context.Context) (string, error) { return "sha256:new", nil }
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		// Make the restore impossible: the parent becomes unwritable
		// mid-pull, then the pull fails as if the registry were unreachable.
		if chErr := os.Chmod(parent, 0o555); chErr != nil {
			t.Fatalf("chmod parent: %v", chErr)
		}
		return errors.New("registry unreachable")
	}, digestFn)
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	_ = os.Chmod(parent, 0o755) // restore for the assertions below
	if err == nil {
		t.Fatal("EnsureInCache: expected pull error")
	}

	// The backup survived: a .stale-* sibling still holds the old content.
	entries, readErr := os.ReadDir(parent)
	if readErr != nil {
		t.Fatal(readErr)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".stale-keepme@1.0.0-") {
			found = true
			content, cErr := os.ReadFile(filepath.Join(parent, e.Name(), "SKILL.md"))
			if cErr != nil || string(content) != "precious" {
				t.Errorf("backup content = %q (%v), want precious", content, cErr)
			}
		}
	}
	if !found {
		t.Fatalf("backup destroyed after failed restore; entries: %v", entries)
	}
}
