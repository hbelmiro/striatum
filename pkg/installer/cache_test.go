package installer

import (
	"context"
	"os"
	"path/filepath"
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
	err := EnsureInCache(context.Background(), cacheDir, "", func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return nil
	})
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
	err := EnsureInCache(context.Background(), cacheDir, "", func(ctx context.Context, outputDir string) error {
		pullOutput = outputDir
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	})
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
	err := EnsureInCache(context.Background(), cacheDir, "", func(ctx context.Context, outputDir string) error {
		return wantErr
	})
	if err != wantErr {
		t.Errorf("EnsureInCache err = %v, want %v", err, wantErr)
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
	err := EnsureInCache(context.Background(), cacheDir, digest, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if pullCalled {
		t.Error("pull should not be called when digest matches")
	}
}

func TestEnsureInCache_DigestMismatch_RePulls(t *testing.T) {
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
	pullCalled := false
	newDigest := "sha256:new"
	err := EnsureInCache(context.Background(), cacheDir, newDigest, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called when digest mismatches")
	}
	stored, ok := ReadDigest(cacheDir)
	if !ok || stored != newDigest {
		t.Errorf("stored digest = %q, want %q", stored, newDigest)
	}
}

func TestEnsureInCache_MissingDigestFile_RePulls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("no-digest", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	// No .striatum-digest file written
	pullCalled := false
	remoteDigest := "sha256:remote"
	err := EnsureInCache(context.Background(), cacheDir, remoteDigest, func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return os.WriteFile(filepath.Join(outputDir, "artifact.json"), []byte("{}"), 0o600)
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if !pullCalled {
		t.Error("pull should be called when digest file is missing and remote digest provided")
	}
	stored, ok := ReadDigest(cacheDir)
	if !ok || stored != remoteDigest {
		t.Errorf("stored digest = %q, want %q", stored, remoteDigest)
	}
}

func TestEnsureInCache_EmptyRemoteDigest_TrustsCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("no-remote-digest", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	pullCalled := false
	err := EnsureInCache(context.Background(), cacheDir, "", func(ctx context.Context, outputDir string) error {
		pullCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("EnsureInCache: %v", err)
	}
	if pullCalled {
		t.Error("pull should not be called when remote digest is empty (trust cache)")
	}
}

func TestWriteDigest_ReadDigest_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	digest := "sha256:abc123def456"
	if err := WriteDigest(dir, digest); err != nil {
		t.Fatal(err)
	}
	got, ok := ReadDigest(dir)
	if !ok {
		t.Fatal("ReadDigest returned ok=false")
	}
	if got != digest {
		t.Errorf("ReadDigest = %q, want %q", got, digest)
	}
}

func TestReadDigest_Missing_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	_, ok := ReadDigest(dir)
	if ok {
		t.Error("ReadDigest should return ok=false for missing digest file")
	}
}
