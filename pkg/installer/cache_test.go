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
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
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
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
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
	err := EnsureInCache(context.Background(), cacheDir, func(ctx context.Context, outputDir string) error {
		return wantErr
	})
	if err != wantErr {
		t.Errorf("EnsureInCache err = %v, want %v", err, wantErr)
	}
}
