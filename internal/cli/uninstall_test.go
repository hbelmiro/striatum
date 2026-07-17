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
	root.SetArgs([]string{"uninstall", "foo"})
	err := root.Execute()
	if err == nil {
		t.Error("uninstall without --target: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "target") {
		t.Errorf("error should mention target: %v", err)
	}
}

func TestNormalizeUninstallName(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"example-skill:1.0.0", "example-skill"},
		{"my-skill", "my-skill"},
		{"foo:1.0.0", "foo"},
		{"  a:b  ", "a"},
		{"localhost:5000/skills/foo:1.0.0", "localhost:5000/skills/foo:1.0.0"},
		{"oci:/path/layout:my-skill:1.0.0", "oci:/path/layout:my-skill:1.0.0"},
	}
	for _, tt := range tests {
		got := normalizeUninstallName(tt.arg)
		if got != tt.want {
			t.Errorf("normalizeUninstallName(%q) = %q, want %q", tt.arg, got, tt.want)
		}
	}
}

func TestUninstall_InvalidTarget_Errors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "all", "foo"})
	err := root.Execute()
	if err == nil {
		t.Error("uninstall --target all: expected error")
	}
	if !strings.Contains(err.Error(), "must be cursor or claude") {
		t.Errorf("error should mention valid targets: %v", err)
	}
}

func TestUninstall_UnknownNameErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
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

func TestUninstall_DependencyName_ErrorListsRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "root-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "my-dep", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "root-a", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "my-dep"})
	err := root.Execute()
	if err == nil {
		t.Fatal("uninstall dependency: expected error")
	}
	if !strings.Contains(err.Error(), "dependency") {
		t.Errorf("error should mention dependency, got: %v", err)
	}
	if !strings.Contains(err.Error(), "root-a") {
		t.Errorf("error should list root skill(s), got: %v", err)
	}
}

func TestUninstall_DependencyName_DeduplicatesRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "root-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "my-dep", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "root-a", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "my-dep", Kind: "Prompt", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "root-a", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "my-dep"})
	err := root.Execute()
	if err == nil {
		t.Fatal("uninstall dependency: expected error")
	}
	if strings.Count(err.Error(), "root-a") != 1 {
		t.Errorf("root-a should appear exactly once (deduplicated), got: %v", err)
	}
}

func TestUninstall_RemovesSkillAndOrphans(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifestA := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-a", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{&artifact.OCIDependency{
			RegistryHost: "reg", Repository: "skill-b", Tag: "1.0.0",
		}},
	}
	manifestB := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-b", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}

	cacheDirA := installer.CacheDir("Skill", "skill-a", "1.0.0")
	cacheDirB := installer.CacheDir("Skill", "skill-b", "1.0.0")
	if err := os.MkdirAll(cacheDirA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDirB, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, cacheDirA, manifestA)
	writeArtifact(t, cacheDirB, manifestB)
	targetDir, err := installer.Targets("cursor", "", "Skill")
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
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "skill-b", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "skill-a", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

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
	entries, err2 := installer.LoadInstalled()
	if err2 != nil {
		t.Fatal(err2)
	}
	if len(entries) != 0 {
		t.Errorf("DB should be empty after uninstall, got %d entries", len(entries))
	}
}

func TestUninstall_AcceptsNameVersionRef(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "example-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	cacheDir := installer.CacheDir("Skill", "example-skill", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, cacheDir, manifest)
	targetDir, err := installer.Targets("cursor", "", "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDir, targetDir, "example-skill"); err != nil {
		t.Fatal(err)
	}
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "example-skill", Kind: "Skill", Version: "1.0.0", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "example-skill:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall with name:version ref: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "example-skill")); !os.IsNotExist(err) {
		t.Error("example-skill dir should be removed")
	}
	entries, err2 := installer.LoadInstalled()
	if err2 != nil {
		t.Fatal(err2)
	}
	if len(entries) != 0 {
		t.Errorf("DB should be empty after uninstall, got %d entries", len(entries))
	}
}

func TestUninstall_PreservesNonOrphanDeps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifestA := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-a", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	manifestB := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-b", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	manifestC := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-c", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}

	cacheDirA := installer.CacheDir("Skill", "skill-a", "1.0.0")
	cacheDirB := installer.CacheDir("Skill", "skill-b", "1.0.0")
	cacheDirC := installer.CacheDir("Skill", "skill-c", "1.0.0")
	for _, d := range []string{cacheDirA, cacheDirB, cacheDirC} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeArtifact(t, cacheDirA, manifestA)
	writeArtifact(t, cacheDirB, manifestB)
	writeArtifact(t, cacheDirC, manifestC)

	targetDir, err := installer.Targets("cursor", "", "Skill")
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
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "skill-b", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "skill-c", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "skill-b", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

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
	if _, err := os.Stat(filepath.Join(targetDir, "skill-b")); os.IsNotExist(err) {
		t.Error("skill-b (still a root) should remain")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "skill-c")); os.IsNotExist(err) {
		t.Error("skill-c (dep of B which is still a root) should remain")
	}
	entries, err3 := installer.LoadInstalled()
	if err3 != nil {
		t.Fatal(err3)
	}
	if len(entries) != 2 {
		t.Errorf("DB should have 2 entries (B and C), got %d", len(entries))
	}
}

func TestUninstall_SharedDepNotOrphanedWhenOneRootRemains(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	for _, name := range []string{"root-a", "root-b", "shared-dep"} {
		m := &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       "Skill",
			Metadata:   artifact.Metadata{Name: name, Version: "1.0.0"},
			Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		}
		d := installer.CacheDir("Skill", name, "1.0.0")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		writeArtifact(t, d, m)
	}

	targetDir, err := installer.Targets("cursor", "", "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root-a", "root-b", "shared-dep"} {
		if err := installer.InstallToTarget(installer.CacheDir("Skill", name, "1.0.0"), targetDir, name); err != nil {
			t.Fatal(err)
		}
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "root-a", Kind: "Skill", Version: "1.0.0", Registry: "reg/root-a:1.0.0", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "root-b", Kind: "Skill", Version: "1.0.0", Registry: "reg/root-b:1.0.0", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "shared-dep", Kind: "Skill", Version: "1.0.0", Registry: "reg/shared-dep:1.0.0", Target: "cursor", InstalledWith: "root-a root-b", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"uninstall", "--target", "cursor", "root-a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "root-a")); !os.IsNotExist(err) {
		t.Error("root-a should be removed")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "root-b")); os.IsNotExist(err) {
		t.Error("root-b should remain")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "shared-dep")); os.IsNotExist(err) {
		t.Error("shared-dep should remain (root-b still needs it)")
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("DB should have 2 entries (root-b + shared-dep), got %d", len(entries))
	}
	for _, e := range entries {
		if e.Name == "root-a" {
			t.Error("root-a should not be in DB")
		}
	}
}

func TestUninstall_OrphanRemoveWarning_StillSaves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifestA := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "root-x", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	cacheDirA := installer.CacheDir("Skill", "root-x", "1.0.0")
	if err := os.MkdirAll(cacheDirA, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, cacheDirA, manifestA)

	targetDir, err := installer.Targets("cursor", "", "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDirA, targetDir, "root-x"); err != nil {
		t.Fatal(err)
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "root-x", Kind: "Skill", Version: "1.0.0", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "orphan-dep", Kind: "Skill", Version: "1.0.0", Target: "cursor", InstalledWith: "root-x", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	stderr := &strings.Builder{}
	root := NewRootCommand()
	root.SetErr(stderr)
	root.SetArgs([]string{"uninstall", "--target", "cursor", "root-x"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if !strings.Contains(stderr.String(), "Warning") {
		t.Logf("stderr: %s (warning expected for orphan-dep removal failure)", stderr.String())
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("DB should be empty after uninstall + orphan cleanup, got %d", len(entries))
	}
}

func TestComputeOrphans_MultiRoot(t *testing.T) {
	entries := []installer.InstalledEntry{
		{Name: "root-b", Kind: "Skill", Version: "1.0.0", Target: "cursor", InstalledWith: ""},
		{Name: "dep", Kind: "Skill", Version: "1.0.0", Target: "cursor", InstalledWith: "root-a root-b"},
	}
	orphans := computeOrphans(entries)
	if len(orphans) != 0 {
		t.Errorf("dep should not be orphaned (root-b still present), got %d orphans", len(orphans))
	}

	entriesAllGone := []installer.InstalledEntry{
		{Name: "dep", Kind: "Skill", Version: "1.0.0", Target: "cursor", InstalledWith: "root-a root-b"},
	}
	orphans2 := computeOrphans(entriesAllGone)
	if len(orphans2) != 1 {
		t.Errorf("dep should be orphaned (no roots present), got %d orphans", len(orphans2))
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

func TestUninstall_GlobalScope_LeavesProjectScoped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	projectDir := t.TempDir()

	// Setup cache for skill-a
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-a", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	cacheDir := installer.CacheDir("Skill", "skill-a", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, cacheDir, manifest)

	// Install globally
	globalTargetDir, err := installer.Targets("cursor", "", "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(globalTargetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDir, globalTargetDir, "skill-a"); err != nil {
		t.Fatal(err)
	}

	// Install for project
	projectTargetDir, err := installer.Targets("cursor", projectDir, "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectTargetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDir, projectTargetDir, "skill-a"); err != nil {
		t.Fatal(err)
	}

	// Save DB with both entries
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: "", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: projectDir, InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	// Uninstall globally (no --project flag)
	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "skill-a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall global: %v", err)
	}

	// Verify global entry removed
	if _, err := os.Stat(filepath.Join(globalTargetDir, "skill-a")); !os.IsNotExist(err) {
		t.Error("global skill-a should be removed")
	}

	// Verify project entry still exists
	if _, err := os.Stat(filepath.Join(projectTargetDir, "skill-a")); os.IsNotExist(err) {
		t.Error("project skill-a should still exist")
	}

	// Verify DB has only project entry
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("DB should have 1 entry (project), got %d", len(entries))
	}
	if entries[0].ProjectPath != projectDir {
		t.Errorf("remaining entry ProjectPath = %q, want %q", entries[0].ProjectPath, projectDir)
	}
}

func TestUninstall_ProjectScope_LeavesGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	projectDir := t.TempDir()

	// Setup cache for skill-a
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-a", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	cacheDir := installer.CacheDir("Skill", "skill-a", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, cacheDir, manifest)

	// Install globally
	globalTargetDir, err := installer.Targets("cursor", "", "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(globalTargetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDir, globalTargetDir, "skill-a"); err != nil {
		t.Fatal(err)
	}

	// Install for project
	projectTargetDir, err := installer.Targets("cursor", projectDir, "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectTargetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDir, projectTargetDir, "skill-a"); err != nil {
		t.Fatal(err)
	}

	// Save DB with both entries
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: "", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: projectDir, InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	// Uninstall from project scope
	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "--project", projectDir, "skill-a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall project: %v", err)
	}

	// Verify project entry removed
	if _, err := os.Stat(filepath.Join(projectTargetDir, "skill-a")); !os.IsNotExist(err) {
		t.Error("project skill-a should be removed")
	}

	// Verify global entry still exists
	if _, err := os.Stat(filepath.Join(globalTargetDir, "skill-a")); os.IsNotExist(err) {
		t.Error("global skill-a should still exist")
	}

	// Verify DB has only global entry
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("DB should have 1 entry (global), got %d", len(entries))
	}
	if entries[0].ProjectPath != "" {
		t.Errorf("remaining entry ProjectPath = %q, want empty (global)", entries[0].ProjectPath)
	}
}

func TestUninstall_OrphanCleanup_RespectsScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	projectDir := t.TempDir()

	// Setup cache for root-a, dep-x, root-b
	for _, name := range []string{"root-a", "dep-x", "root-b"} {
		m := &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       "Skill",
			Metadata:   artifact.Metadata{Name: name, Version: "1.0.0"},
			Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		}
		cacheDir := installer.CacheDir("Skill", name, "1.0.0")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeArtifact(t, cacheDir, m)
	}

	// Install to global target
	globalTargetDir, err := installer.Targets("cursor", "", "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(globalTargetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root-a", "dep-x"} {
		cacheDir := installer.CacheDir("Skill", name, "1.0.0")
		if err := installer.InstallToTarget(cacheDir, globalTargetDir, name); err != nil {
			t.Fatal(err)
		}
	}

	// Install to project target
	projectTargetDir, err := installer.Targets("cursor", projectDir, "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectTargetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cacheDir := installer.CacheDir("Skill", "root-b", "1.0.0")
	if err := installer.InstallToTarget(cacheDir, projectTargetDir, "root-b"); err != nil {
		t.Fatal(err)
	}

	// Save DB: global (root-a + dep-x), project (root-b)
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "root-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: "", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "dep-x", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: "", InstalledWith: "root-a", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "root-b", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: projectDir, InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	// Uninstall root-a from global scope
	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "root-a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall root-a: %v", err)
	}

	// Verify global: root-a and dep-x removed (orphan cleanup)
	if _, err := os.Stat(filepath.Join(globalTargetDir, "root-a")); !os.IsNotExist(err) {
		t.Error("global root-a should be removed")
	}
	if _, err := os.Stat(filepath.Join(globalTargetDir, "dep-x")); !os.IsNotExist(err) {
		t.Error("global dep-x should be removed (orphan)")
	}

	// Verify project: root-b still exists (different scope)
	if _, err := os.Stat(filepath.Join(projectTargetDir, "root-b")); os.IsNotExist(err) {
		t.Error("project root-b should still exist (different scope)")
	}

	// Verify DB has only root-b
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("DB should have 1 entry (root-b), got %d", len(entries))
	}
	if entries[0].Name != "root-b" || entries[0].ProjectPath != projectDir {
		t.Errorf("remaining entry = %+v, want root-b in project scope", entries[0])
	}
}

func TestUninstall_RemovesPromptOrphansFromPromptsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifestA := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-a", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	manifestPrompt := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: "prompt-dep", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "prompt.md", Files: []string{"prompt.md"}},
	}

	cacheDirA := installer.CacheDir("Skill", "skill-a", "1.0.0")
	cacheDirPrompt := installer.CacheDir("Prompt", "prompt-dep", "1.0.0")
	if err := os.MkdirAll(cacheDirA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDirPrompt, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, cacheDirA, manifestA)
	writeArtifact(t, cacheDirPrompt, manifestPrompt)

	skillsDir, err := installer.Targets("cursor", "", "Skill")
	if err != nil {
		t.Fatal(err)
	}
	promptsDir, err := installer.Targets("cursor", "", "Prompt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDirA, skillsDir, "skill-a"); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDirPrompt, promptsDir, "prompt-dep"); err != nil {
		t.Fatal(err)
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "prompt-dep", Kind: "Prompt", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "skill-a", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"uninstall", "--target", "cursor", "skill-a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "skill-a")); !os.IsNotExist(err) {
		t.Error("skill-a dir should be removed from skills/")
	}
	if _, err := os.Stat(filepath.Join(promptsDir, "prompt-dep")); !os.IsNotExist(err) {
		t.Error("prompt-dep (orphan) dir should be removed from prompts/")
	}
	entries, err2 := installer.LoadInstalled()
	if err2 != nil {
		t.Fatal(err2)
	}
	if len(entries) != 0 {
		t.Errorf("DB should be empty after uninstall, got %d entries", len(entries))
	}
}

func TestUninstall_CrossKindSameNameOrphan_DoesNotRemoveOtherKind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	cacheDirSkill := installer.CacheDir("Skill", "shared-name", "1.0.0")
	cacheDirPrompt := installer.CacheDir("Prompt", "shared-name", "2.0.0")
	cacheDirRoot := installer.CacheDir("Skill", "root-skill", "1.0.0")
	for _, d := range []string{cacheDirSkill, cacheDirPrompt, cacheDirRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeArtifact(t, cacheDirRoot, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2", Kind: "Skill",
		Metadata: artifact.Metadata{Name: "root-skill", Version: "1.0.0"},
		Spec:     artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})
	writeArtifact(t, cacheDirSkill, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2", Kind: "Skill",
		Metadata: artifact.Metadata{Name: "shared-name", Version: "1.0.0"},
		Spec:     artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})
	writeArtifact(t, cacheDirPrompt, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2", Kind: "Prompt",
		Metadata: artifact.Metadata{Name: "shared-name", Version: "2.0.0"},
		Spec:     artifact.Spec{Entrypoint: "prompt.md", Files: []string{"prompt.md"}},
	})

	skillsDir, _ := installer.Targets("cursor", "", "Skill")
	promptsDir, _ := installer.Targets("cursor", "", "Prompt")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDirRoot, skillsDir, "root-skill"); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDirSkill, skillsDir, "shared-name"); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDirPrompt, promptsDir, "shared-name"); err != nil {
		t.Fatal(err)
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "root-skill", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "shared-name", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "shared-name", Kind: "Prompt", Version: "2.0.0", Registry: "reg", Target: "cursor", InstalledWith: "root-skill", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"uninstall", "--target", "cursor", "root-skill"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	// root-skill should be removed
	if _, err := os.Stat(filepath.Join(skillsDir, "root-skill")); !os.IsNotExist(err) {
		t.Error("root-skill should be removed from skills/")
	}
	// Prompt "shared-name" is an orphan and should be removed from prompts/
	if _, err := os.Stat(filepath.Join(promptsDir, "shared-name")); !os.IsNotExist(err) {
		t.Error("prompt shared-name (orphan) should be removed from prompts/")
	}
	// Skill "shared-name" is a root and should NOT be removed
	if _, err := os.Stat(filepath.Join(skillsDir, "shared-name")); os.IsNotExist(err) {
		t.Error("skill shared-name (root) should still exist in skills/")
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("DB should have 1 entry (Skill shared-name), got %d", len(entries))
	}
	if entries[0].Name != "shared-name" || entries[0].Kind != "Skill" {
		t.Errorf("remaining entry = %+v, want Skill shared-name", entries[0])
	}
}

func TestComputeOrphans_CrossScope_NoFalseOrphan(t *testing.T) {
	projectDir := "/proj"

	// After removing root-a from global scope
	remainingAfterRemoveRootA := []installer.InstalledEntry{
		{Name: "dep-x", Kind: "Skill", Version: "1.0.0", Target: "cursor", ProjectPath: "", InstalledWith: "root-a"},
		{Name: "root-b", Kind: "Skill", Version: "1.0.0", Target: "cursor", ProjectPath: projectDir, InstalledWith: ""},
		{Name: "dep-x", Kind: "Skill", Version: "1.0.0", Target: "cursor", ProjectPath: projectDir, InstalledWith: "root-b"},
	}

	orphans := computeOrphans(remainingAfterRemoveRootA)

	// Should find 1 orphan: global dep-x (root-a gone in global scope)
	// Should NOT find project dep-x as orphan (root-b still exists in project scope)
	if len(orphans) != 1 {
		t.Fatalf("len(orphans) = %d, want 1 (global dep-x only)", len(orphans))
	}

	if orphans[0].Name != "dep-x" || orphans[0].ProjectPath != "" {
		t.Errorf("orphan = %+v, want global dep-x", orphans[0])
	}
}

func TestUninstall_AmbiguousKind_ErrorsWithRecommendation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "shared-name", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "shared-name", Kind: "Prompt", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "shared-name"})
	err := root.Execute()
	if err == nil {
		t.Fatal("uninstall with ambiguous kind: expected error")
	}
	if !strings.Contains(err.Error(), "--kind") {
		t.Errorf("error should recommend --kind flag, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Skill") || !strings.Contains(err.Error(), "Prompt") {
		t.Errorf("error should list the conflicting kinds, got: %v", err)
	}
}

func TestUninstall_KindFlag_Disambiguates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	for _, kind := range []string{"Skill", "Prompt"} {
		m := &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       kind,
			Metadata:   artifact.Metadata{Name: "shared-name", Version: "1.0.0"},
			Spec:       artifact.Spec{Entrypoint: "file.md", Files: []string{"file.md"}},
		}
		cacheDir := installer.CacheDir(kind, "shared-name", "1.0.0")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeArtifact(t, cacheDir, m)
		targetDir, err := installer.Targets("cursor", "", kind)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := installer.InstallToTarget(cacheDir, targetDir, "shared-name"); err != nil {
			t.Fatal(err)
		}
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "shared-name", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "shared-name", Kind: "Prompt", Version: "1.0.0", Registry: "reg", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "--kind", "Skill", "shared-name"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall with --kind Skill: %v", err)
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("DB should have 1 entry (Prompt), got %d", len(entries))
	}
	if entries[0].Kind != "Prompt" {
		t.Errorf("remaining entry kind = %q, want Prompt", entries[0].Kind)
	}
}

func TestUninstall_InvalidKind_ReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "cursor", "--kind", "Foo", "some-artifact"})
	err := root.Execute()
	if err == nil {
		t.Fatal("uninstall --kind Foo: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported kind") {
		t.Errorf("error should mention 'unsupported kind', got: %v", err)
	}
}

func TestUninstall_Workflow_RemovesFromWorkflowsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "thorough-review", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "review.js", Files: []string{"review.js"}},
	}
	cacheDir := installer.CacheDir("Workflow", "thorough-review", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "review.js"), []byte("// workflow"), 0o600); err != nil {
		t.Fatal(err)
	}

	workflowsDir, err := installer.Targets("claude", "", "Workflow")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDir, workflowsDir, "thorough-review"); err != nil {
		t.Fatal(err)
	}
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "thorough-review", Kind: "Workflow", Version: "1.0.0", Registry: "reg", Target: "claude", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	// Verify installed
	if _, err := os.Stat(filepath.Join(workflowsDir, "thorough-review", "review.js")); err != nil {
		t.Fatalf("workflow not installed: %v", err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"uninstall", "--target", "claude", "thorough-review"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall workflow: %v", err)
	}

	// Verify removed from workflows/ dir
	if _, err := os.Stat(filepath.Join(workflowsDir, "thorough-review")); !os.IsNotExist(err) {
		t.Error("thorough-review dir should be removed from workflows/")
	}

	// Verify DB empty
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("DB should be empty after uninstall, got %d entries", len(entries))
	}
}

func TestUninstall_Workflow_RemovesSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "thorough-review", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "review.js", Files: []string{"review.js"}},
	}
	cacheDir := installer.CacheDir("Workflow", "thorough-review", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "review.js"), []byte("// workflow"), 0o600); err != nil {
		t.Fatal(err)
	}

	workflowsDir, err := installer.Targets("claude", "", "Workflow")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDir, workflowsDir, "thorough-review"); err != nil {
		t.Fatal(err)
	}
	// Create the symlink that install would have created
	linkPath := filepath.Join(workflowsDir, "thorough-review.js")
	if err := os.Symlink(filepath.Join("thorough-review", "review.js"), linkPath); err != nil {
		t.Fatal(err)
	}
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "thorough-review", Kind: "Workflow", Version: "1.0.0", Registry: "reg", Target: "claude", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"uninstall", "--target", "claude", "thorough-review"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall workflow: %v", err)
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink thorough-review.js should be removed after uninstall")
	}
}

func TestUninstall_Workflow_NoSymlink_StillSucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	cacheDir := installer.CacheDir("Workflow", "my-wf", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "my-wf", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "run.js", Files: []string{"run.js"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "run.js"), []byte("// wf"), 0o600); err != nil {
		t.Fatal(err)
	}

	workflowsDir, err := installer.Targets("claude", "", "Workflow")
	if err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(cacheDir, workflowsDir, "my-wf"); err != nil {
		t.Fatal(err)
	}
	// Intentionally do NOT create the symlink — simulates manual removal
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "my-wf", Kind: "Workflow", Version: "1.0.0", Registry: "reg", Target: "claude", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"uninstall", "--target", "claude", "my-wf"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall workflow without symlink: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workflowsDir, "my-wf")); !os.IsNotExist(err) {
		t.Error("workflow dir should be removed")
	}
}

func TestUninstall_Memory_RemovesFilesAndIndex(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	slug := installer.ProjectPathToSlug(projectDir)
	memoryDir := filepath.Join(home, ".claude", "projects", slug, "memory")
	artDir := filepath.Join(memoryDir, "team-conv")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "fb.md"), []byte("---\nname: fb\ndescription: test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	memoryMD := filepath.Join(memoryDir, "MEMORY.md")
	if err := os.WriteFile(memoryMD, []byte("- [Fb](team-conv/fb.md) — test\n- [Other](other/x.md) — keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "team-conv", Kind: "Memory", Version: "1.0.0", Target: "claude", ProjectPath: projectDir, Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"uninstall", "--target", "claude", "--project", projectDir, "team-conv"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall memory: %v", err)
	}

	if _, err := os.Stat(artDir); !os.IsNotExist(err) {
		t.Error("artifact dir should be removed")
	}

	data, _ := os.ReadFile(memoryMD)
	content := string(data)
	if strings.Contains(content, "team-conv") {
		t.Error("MEMORY.md should have team-conv entries removed")
	}
	if !strings.Contains(content, "Other") {
		t.Error("MEMORY.md should preserve entries from other artifacts")
	}

	entries, _ := installer.LoadInstalled()
	if len(entries) != 0 {
		t.Errorf("DB should be empty, got %d entries", len(entries))
	}
}

func TestUninstall_Memory_AllExistingProjects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	proj1 := t.TempDir()
	proj2 := t.TempDir()
	for _, p := range []string{proj1, proj2} {
		if err := os.MkdirAll(filepath.Join(p, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, p := range []string{proj1, proj2} {
		slug := installer.ProjectPathToSlug(p)
		memoryDir := filepath.Join(home, ".claude", "projects", slug, "memory")
		artDir := filepath.Join(memoryDir, "team-conv")
		if err := os.MkdirAll(artDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(artDir, "fb.md"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "team-conv", Kind: "Memory", Version: "1.0.0", Target: "claude", ProjectPath: proj1, Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "team-conv", Kind: "Memory", Version: "1.0.0", Target: "claude", ProjectPath: proj2, Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"uninstall", "--target", "claude", "--all-existing-projects", "team-conv"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall memory all-existing-projects: %v", err)
	}

	for _, p := range []string{proj1, proj2} {
		slug := installer.ProjectPathToSlug(p)
		artDir := filepath.Join(home, ".claude", "projects", slug, "memory", "team-conv")
		if _, err := os.Stat(artDir); !os.IsNotExist(err) {
			t.Errorf("artifact dir should be removed from %s", p)
		}
	}

	entries, _ := installer.LoadInstalled()
	if len(entries) != 0 {
		t.Errorf("DB should be empty, got %d entries", len(entries))
	}
}

func TestUninstall_Memory_WithoutProjectFlag_NotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "team-conv", Kind: "Memory", Version: "1.0.0", Target: "claude", ProjectPath: projectDir, Status: "ok"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"uninstall", "--target", "claude", "team-conv"})
	err := root.Execute()
	if err == nil {
		t.Fatal("uninstall memory without --project should not find the entry")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error should mention not installed: %v", err)
	}
}

func TestUninstall_WorkflowOrphan_RemovesSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	// Set up a root skill that a workflow depends on (InstalledWith = root skill name)
	rootManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "root-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	rootCache := installer.CacheDir("Skill", "root-skill", "1.0.0")
	if err := os.MkdirAll(rootCache, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, rootCache, rootManifest)

	skillsDir, err := installer.Targets("claude", "", "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(rootCache, skillsDir, "root-skill"); err != nil {
		t.Fatal(err)
	}

	// Set up a workflow orphan (InstalledWith points to root-skill)
	wfManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "orphan-wf", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "run.js", Files: []string{"run.js"}},
	}
	wfCache := installer.CacheDir("Workflow", "orphan-wf", "1.0.0")
	if err := os.MkdirAll(wfCache, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, wfCache, wfManifest)
	if err := os.WriteFile(filepath.Join(wfCache, "run.js"), []byte("// wf"), 0o600); err != nil {
		t.Fatal(err)
	}

	workflowsDir, err := installer.Targets("claude", "", "Workflow")
	if err != nil {
		t.Fatal(err)
	}
	if err := installer.InstallToTarget(wfCache, workflowsDir, "orphan-wf"); err != nil {
		t.Fatal(err)
	}
	// Create the symlink
	linkPath := filepath.Join(workflowsDir, "orphan-wf.js")
	if err := os.Symlink(filepath.Join("orphan-wf", "run.js"), linkPath); err != nil {
		t.Fatal(err)
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "root-skill", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "claude", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "orphan-wf", Kind: "Workflow", Version: "1.0.0", Registry: "reg", Target: "claude", InstalledWith: "root-skill", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	// Uninstall root-skill — orphan-wf should be cleaned up including its symlink
	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"uninstall", "--target", "claude", "root-skill"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall root-skill: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workflowsDir, "orphan-wf")); !os.IsNotExist(err) {
		t.Error("orphan workflow dir should be removed")
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("orphan workflow symlink should be removed")
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("DB should be empty, got %d entries", len(entries))
	}
}
