package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearCache_MissingCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	removed, err := ClearCache()
	if err != nil {
		t.Fatalf("ClearCache on missing cache dir: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}

func TestClearCache_RemovesAllEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	for _, entry := range []struct{ kind, name, version string }{
		{"Skill", "foo", "1.0.0"},
		{"Skill", "bar", "2.0.0"},
		{"Prompt", "baz", "1.0.0"},
	} {
		cacheDir := CacheDir(entry.kind, entry.name, entry.version)
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	scDir := SelfContainedCacheDir("bundle", "1.0.0")
	if err := os.MkdirAll(scDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	removed, err := ClearCache()
	if err != nil {
		t.Fatalf("ClearCache: %v", err)
	}
	if removed != 4 {
		t.Errorf("removed = %d, want 4", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache")); !os.IsNotExist(err) {
		t.Errorf("cache dir should be gone after ClearCache; stat err: %v", err)
	}
}

// ClearCache must not touch anything outside the cache directory, e.g. the
// installed artifacts registry that lives beside it in the striatum home.
func TestClearCache_PreservesRestOfStriatumHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	cacheDir := CacheDir("Skill", "foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(other, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ClearCache(); err != nil {
		t.Fatalf("ClearCache: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("file outside cache destroyed: %v", err)
	}
}

// A non-directory at the cache path (e.g. a stray file) is an error, not a
// silent success.
func TestClearCache_CachePathIsFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "cache"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ClearCache()
	if err == nil {
		t.Fatal("ClearCache with file at cache path: expected error")
	}
}

// .stale-* backup dirs left by a failed re-pull restore are removed with the
// rest of the cache, but must not be counted as cache entries: the reported
// count describes real artifacts only.
func TestClearCache_StaleBackupsRemovedButNotCounted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	cacheDir := CacheDir("Skill", "foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(filepath.Dir(cacheDir), ".stale-foo@1.0.0-abc123")
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, err := ClearCache()
	if err != nil {
		t.Fatalf("ClearCache: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1 (stale backup must not count)", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache")); !os.IsNotExist(err) {
		t.Errorf("cache dir should be gone; stat err: %v", err)
	}
}

// A symlinked entry dir is followed for counting (os.Stat, not lstat) so the
// reported count matches what the user had in cache.
func TestClearCache_SymlinkedEntryDir_Counted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	real := filepath.Join(dir, "real-foo")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	kindDir := filepath.Join(dir, "cache", "Skill")
	if err := os.MkdirAll(kindDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(kindDir, "foo@1.0.0")); err != nil {
		t.Fatal(err)
	}

	removed, err := ClearCache()
	if err != nil {
		t.Fatalf("ClearCache: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1 (symlinked entry must count)", removed)
	}
}

// When the cache path itself is a symlink to a directory, the real cache
// directory must be removed, not just the link.
func TestClearCache_CachePathIsSymlink_RemovesTarget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	realCache := filepath.Join(dir, "real-cache")
	entry := filepath.Join(realCache, "Skill", "foo@1.0.0")
	if err := os.MkdirAll(entry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realCache, filepath.Join(dir, "cache")); err != nil {
		t.Fatal(err)
	}

	removed, err := ClearCache()
	if err != nil {
		t.Fatalf("ClearCache: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(realCache); !os.IsNotExist(err) {
		t.Errorf("real cache dir should be removed; stat err: %v", err)
	}
}

// A broken symlink entry is cache junk: it must be removed without failing
// the count, and must not be counted as an artifact.
func TestClearCache_BrokenSymlinkEntry_RemovedWithoutError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	cacheDir := CacheDir("Skill", "foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	kindDir := filepath.Dir(cacheDir)
	if err := os.Symlink(filepath.Join(dir, "does-not-exist"), filepath.Join(kindDir, "gone@1.0.0")); err != nil {
		t.Fatal(err)
	}

	removed, err := ClearCache()
	if err != nil {
		t.Fatalf("ClearCache with broken symlink entry: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1 (broken symlink must not count)", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache")); !os.IsNotExist(err) {
		t.Errorf("cache dir should be gone; stat err: %v", err)
	}
}

// A broken symlink in place of a kind dir must not block clearing the cache.
func TestClearCache_BrokenSymlinkKindDir_RemovedWithoutError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	if err := os.MkdirAll(filepath.Join(dir, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "nope"), filepath.Join(dir, "cache", "Skill")); err != nil {
		t.Fatal(err)
	}

	removed, err := ClearCache()
	if err != nil {
		t.Fatalf("ClearCache with broken symlink kind dir: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache")); !os.IsNotExist(err) {
		t.Errorf("cache dir should be gone; stat err: %v", err)
	}
}

func TestClearCache_RemoveAllFailure_ReturnsError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission tests do not work as root")
	}
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	cacheDir := CacheDir("Skill", "foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err := ClearCache()
	if err == nil {
		t.Fatal("ClearCache with unwritable striatum home: expected error")
	}
	if !strings.Contains(err.Error(), "remove cache dir") {
		t.Errorf("error should mention remove cache dir: %v", err)
	}
}

func TestClearCache_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)

	if _, err := ClearCache(); err != nil {
		t.Fatalf("first ClearCache: %v", err)
	}
	removed, err := ClearCache()
	if err != nil {
		t.Fatalf("second ClearCache: %v", err)
	}
	if removed != 0 {
		t.Errorf("second ClearCache removed = %d, want 0", removed)
	}
}
