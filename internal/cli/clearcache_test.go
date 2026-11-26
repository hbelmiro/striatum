package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
)

func TestClearCache_RemovesCacheDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	cacheDir := installer.CacheDir("Skill", "foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactForList(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "foo", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})

	scDir := installer.SelfContainedCacheDir("bundle", "1.0.0")
	if err := os.MkdirAll(scDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"clear-cache"})
	if err := root.Execute(); err != nil {
		t.Fatalf("clear-cache: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, "cache")); !os.IsNotExist(err) {
		t.Errorf("cache dir should be gone; stat err: %v", err)
	}
	if !strings.Contains(out.String(), "Removed 2 cache entries") {
		t.Errorf("output should report 2 removed entries; got %q", out.String())
	}
}

func TestClearCache_EmptyCache_ReportsZero(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"clear-cache"})
	if err := root.Execute(); err != nil {
		t.Fatalf("clear-cache on empty cache: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Removed 0 cache entries") {
		t.Errorf("output should report 'Removed 0 cache entries'; got %q", got)
	}
}

func TestClearCache_SingleEntry_SingularWording(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	cacheDir := installer.CacheDir("Skill", "solo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactForList(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "solo", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})

	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"clear-cache"})
	if err := root.Execute(); err != nil {
		t.Fatalf("clear-cache: %v", err)
	}

	if !strings.Contains(out.String(), "Removed 1 cache entry\n") {
		t.Errorf("output should use singular wording; got %q", out.String())
	}
}

func TestClearCache_RejectsArgs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	root := NewRootCommand()
	root.SetArgs([]string{"clear-cache", "extra"})
	if err := root.Execute(); err == nil {
		t.Error("clear-cache with extra args: expected error")
	}
}
