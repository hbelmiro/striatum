package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
)

func TestUninstall_MissingTargetErrors(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "foo"})
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
	_ = os.Setenv("STRIATUM_HOME", home)
	_ = os.Setenv("HOME", home)
	defer func() {
		_, _ = os.Unsetenv("STRIATUM_HOME"), os.Unsetenv("HOME")
	}()
	// No DB or empty DB
	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "nonexistent"})
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
	_ = os.Setenv("STRIATUM_HOME", home)
	_ = os.Setenv("HOME", home)
	defer func() {
		_, _ = os.Unsetenv("STRIATUM_HOME"), os.Unsetenv("HOME")
	}()

	// Create layout with root A that has dep B
	layoutDir := t.TempDir()
	baseA := t.TempDir()
	manifestA := &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha1",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: "skill-a", Version: "1.0.0"},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{{Name: "skill-b", Version: "1.0.0"}},
	}
	writeArtifact(baseA, manifestA)
	_ = oci.Pack(manifestA, baseA, layoutDir)

	baseB := t.TempDir()
	manifestB := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-b", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	writeArtifact(baseB, manifestB)
	_ = oci.Pack(manifestB, baseB, layoutDir)

	// Install A (would need resolver for real deps; for test just seed DB and copy dirs)
	cacheDirA := installer.CacheDir("skill-a", "1.0.0")
	cacheDirB := installer.CacheDir("skill-b", "1.0.0")
	_ = os.MkdirAll(cacheDirA, 0o755)
	_ = os.MkdirAll(cacheDirB, 0o755)
	writeArtifact(cacheDirA, manifestA)
	writeArtifact(cacheDirB, manifestB)
	targetDir, _ := installer.Targets("cursor", "")
	_ = os.MkdirAll(targetDir, 0o755)
	_ = installer.InstallToTarget(cacheDirA, targetDir, "skill-a")
	_ = installer.InstallToTarget(cacheDirB, targetDir, "skill-b")
	_ = installer.SaveInstalled([]installer.InstalledEntry{
		{Skill: "skill-a", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Skill: "skill-b", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "skill-a", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	})

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"uninstall", "--target", "cursor", "skill-a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "skill-a")); !os.IsNotExist(err) {
		t.Error("skill-a dir should be removed")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "skill-b")); !os.IsNotExist(err) {
		t.Error("skill-b (orphan) dir should be removed")
	}
	entries, _ := installer.LoadInstalled()
	if len(entries) != 0 {
		t.Errorf("DB should be empty after uninstall, got %d entries", len(entries))
	}
}

func TestUninstall_SkipsOrphanCleanupWhenRootManifestUnloadable(t *testing.T) {
	home := t.TempDir()
	_ = os.Setenv("STRIATUM_HOME", home)
	_ = os.Setenv("HOME", home)
	defer func() {
		_, _ = os.Unsetenv("STRIATUM_HOME"), os.Unsetenv("HOME")
	}()

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
	_ = os.MkdirAll(cacheDirA, 0o755)
	_ = os.MkdirAll(cacheDirB, 0o755)
	_ = os.MkdirAll(cacheDirC, 0o755)
	writeArtifact(cacheDirA, manifestA)
	writeArtifact(cacheDirB, manifestB)
	writeArtifact(cacheDirC, manifestC)

	targetDir, _ := installer.Targets("cursor", "")
	_ = os.MkdirAll(targetDir, 0o755)
	_ = installer.InstallToTarget(cacheDirA, targetDir, "skill-a")
	_ = installer.InstallToTarget(cacheDirB, targetDir, "skill-b")
	_ = installer.InstallToTarget(cacheDirC, targetDir, "skill-c")

	_ = installer.SaveInstalled([]installer.InstalledEntry{
		{Skill: "skill-a", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Skill: "skill-b", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Skill: "skill-c", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "skill-b", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	})

	// Remove B's cache so computeOrphans cannot load B's manifest (B is a root).
	_ = os.RemoveAll(cacheDirB)

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"uninstall", "--target", "cursor", "skill-a"})
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
	entries, _ := installer.LoadInstalled()
	if len(entries) != 2 {
		t.Errorf("DB should have 2 entries (B and C) after skipping orphan cleanup, got %d", len(entries))
	}
	if !strings.Contains(out.String(), "skipping orphan cleanup") {
		t.Errorf("expected warning about skipping orphan cleanup in output: %q", out.String())
	}
}

func writeArtifact(dir string, m *artifact.Manifest) {
	data, _ := json.Marshal(m)
	_ = os.WriteFile(filepath.Join(dir, "artifact.json"), data, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# x"), 0o600)
}
