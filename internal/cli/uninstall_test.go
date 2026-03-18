package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
)

func TestUninstall_MissingTargetErrors(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "uninstall", "foo"})
	err := root.Execute()
	if err == nil {
		t.Error("uninstall without --target: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "target") {
		t.Errorf("error should mention target: %v", err)
	}
}

func TestUninstall_UnknownNameErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	// No DB or empty DB
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "uninstall", "--target", "cursor", "nonexistent"})
	err := root.Execute()
	if err == nil {
		t.Error("uninstall unknown name: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error should mention not installed: %v", err)
	}
}

func TestUninstall_RemovesSkillAndOrphans(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifestA := &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha1",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: "skill-a", Version: "1.0.0"},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{{Name: "skill-b", Version: "1.0.0"}},
	}
	manifestB := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-b", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}

	cacheDirA := installer.CacheDir("skill-a", "1.0.0")
	cacheDirB := installer.CacheDir("skill-b", "1.0.0")
	if err := os.MkdirAll(cacheDirA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDirB, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, cacheDirA, manifestA)
	writeArtifact(t, cacheDirB, manifestB)
	targetDir, err := installer.Targets("cursor", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDirA, targetDir, "skill-a"); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDirB, targetDir, "skill-b"); err != nil {
		t.Fatal(err)
	}
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Skill: "skill-a", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Skill: "skill-b", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "skill-a", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "uninstall", "--target", "cursor", "skill-a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "skill-a")); !os.IsNotExist(err) {
		t.Error("skill-a dir should be removed")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "skill-b")); !os.IsNotExist(err) {
		t.Error("skill-b (orphan) dir should be removed")
	}
	entries, err2 := installer.LoadInstalled()
	if err2 != nil {
		t.Fatal(err2)
	}
	if len(entries) != 0 {
		t.Errorf("DB should be empty after uninstall, got %d entries", len(entries))
	}
}

func TestUninstall_SkipsOrphanCleanupWhenRootManifestUnloadable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	// Two roots: A and B. B has dependency C. Remove B's cache so we cannot load B's manifest.
	// Uninstall A: A is removed; orphan cleanup sees unloadable root B and skips, so C stays.
	manifestA := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-a", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	manifestB := &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha1",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: "skill-b", Version: "1.0.0"},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{{Name: "skill-c", Version: "1.0.0"}},
	}
	manifestC := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-c", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}

	cacheDirA := installer.CacheDir("skill-a", "1.0.0")
	cacheDirB := installer.CacheDir("skill-b", "1.0.0")
	cacheDirC := installer.CacheDir("skill-c", "1.0.0")
	for _, d := range []string{cacheDirA, cacheDirB, cacheDirC} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeArtifact(t, cacheDirA, manifestA)
	writeArtifact(t, cacheDirB, manifestB)
	writeArtifact(t, cacheDirC, manifestC)

	targetDir, err := installer.Targets("cursor", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct{ dir, name string }{{cacheDirA, "skill-a"}, {cacheDirB, "skill-b"}, {cacheDirC, "skill-c"}} {
		if err := installer.InstallToTarget(pair.dir, targetDir, pair.name); err != nil {
			t.Fatal(err)
		}
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Skill: "skill-a", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Skill: "skill-b", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Skill: "skill-c", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "skill-b", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	// Remove B's cache so computeOrphans cannot load B's manifest (B is a root).
	if err := os.RemoveAll(cacheDirB); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "uninstall", "--target", "cursor", "skill-a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	// A removed from target and DB
	if _, err := os.Stat(filepath.Join(targetDir, "skill-a")); !os.IsNotExist(err) {
		t.Error("skill-a dir should be removed")
	}
	// Orphan cleanup was skipped (unloadable root B), so B and C remain on disk and in DB
	if _, err := os.Stat(filepath.Join(targetDir, "skill-b")); os.IsNotExist(err) {
		t.Error("skill-b dir should remain (orphan cleanup skipped)")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "skill-c")); os.IsNotExist(err) {
		t.Error("skill-c dir should remain (orphan cleanup skipped)")
	}
	entries, err3 := installer.LoadInstalled()
	if err3 != nil {
		t.Fatal(err3)
	}
	if len(entries) != 2 {
		t.Errorf("DB should have 2 entries (B and C) after skipping orphan cleanup, got %d", len(entries))
	}
	if !strings.Contains(out.String(), "skipping orphan cleanup") {
		t.Errorf("expected warning about skipping orphan cleanup in output: %q", out.String())
	}
}

func writeArtifact(t *testing.T, dir string, m *artifact.Manifest) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
