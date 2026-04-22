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
	root.SetArgs([]string{"skill", "uninstall", "--target", "all", "foo"})
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
	cacheDir := installer.CacheDir("example-skill", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, cacheDir, manifest)
	targetDir, err := installer.Targets("cursor", "")
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
		{Skill: "example-skill", Version: "1.0.0", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "uninstall", "--target", "cursor", "example-skill:1.0.0"})
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
		d := installer.CacheDir(name, "1.0.0")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		writeArtifact(t, d, m)
	}

	targetDir, err := installer.Targets("cursor", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root-a", "root-b", "shared-dep"} {
		if err := installer.InstallToTarget(installer.CacheDir(name, "1.0.0"), targetDir, name); err != nil {
			t.Fatal(err)
		}
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Skill: "root-a", Version: "1.0.0", Registry: "reg/root-a:1.0.0", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Skill: "root-b", Version: "1.0.0", Registry: "reg/root-b:1.0.0", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Skill: "shared-dep", Version: "1.0.0", Registry: "reg/shared-dep:1.0.0", Target: "cursor", InstalledWith: "root-a root-b", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "uninstall", "--target", "cursor", "root-a"})
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
		if e.Skill == "root-a" {
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
	cacheDirA := installer.CacheDir("root-x", "1.0.0")
	if err := os.MkdirAll(cacheDirA, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifact(t, cacheDirA, manifestA)

	targetDir, err := installer.Targets("cursor", "")
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
		{Skill: "root-x", Version: "1.0.0", Target: "cursor", InstalledWith: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Skill: "orphan-dep", Version: "1.0.0", Target: "cursor", InstalledWith: "root-x", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	stderr := &strings.Builder{}
	root := NewRootCommand()
	root.SetErr(stderr)
	root.SetArgs([]string{"skill", "uninstall", "--target", "cursor", "root-x"})
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
		{Skill: "root-b", Version: "1.0.0", Target: "cursor", InstalledWith: ""},
		{Skill: "dep", Version: "1.0.0", Target: "cursor", InstalledWith: "root-a root-b"},
	}
	orphans := computeOrphans(entries)
	if len(orphans) != 0 {
		t.Errorf("dep should not be orphaned (root-b still present), got %d orphans", len(orphans))
	}

	entriesAllGone := []installer.InstalledEntry{
		{Skill: "dep", Version: "1.0.0", Target: "cursor", InstalledWith: "root-a root-b"},
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
