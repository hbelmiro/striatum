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

func TestList_TargetWithoutInstalled_ReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	root := NewRootCommand()
	root.SetArgs([]string{"list", "--target", "cursor"})
	err := root.Execute()
	if err == nil {
		t.Error("list --target cursor without --installed: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "only valid with --installed") {
		t.Errorf("error should mention --target only valid with --installed: %v", err)
	}
}

func TestList_EmptyCache_ExitsZeroAndShowsNoArtifacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "No artifacts") {
		t.Errorf("output should contain 'No artifacts'; got %q", got)
	}
}

func TestList_OneCachedArtifact_OutputContainsNameAndVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	cacheDir := installer.CacheDir("foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactForList(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "foo", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})
	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "foo") || !strings.Contains(got, "1.0.0") {
		t.Errorf("output should contain name and version; got %q", got)
	}
}

func TestList_Installed_Empty_ExitsZero(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--installed"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --installed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "No ") {
		t.Errorf("output should indicate no installed artifacts; got %q", got)
	}
}

func TestList_Installed_WithEntries_ShowsTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "bar", Kind: "Skill", Version: "2.0.0", Registry: "reg", Target: "cursor", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--installed"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --installed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "bar") || !strings.Contains(got, "2.0.0") || !strings.Contains(got, "cursor") {
		t.Errorf("output should contain name, version, target; got %q", got)
	}
}

func TestList_Installed_CorruptInstalledYAML_ReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	installedPath := installer.InstalledPath()
	if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installedPath, []byte("invalid: [[["), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	root.SetArgs([]string{"list", "--installed"})
	err := root.Execute()
	if err == nil {
		t.Error("list --installed with corrupt installed.yaml: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "load installed") {
		t.Errorf("error should mention load installed: %v", err)
	}
}

func TestList_Installed_WithTarget_Filters(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "only-cursor", Kind: "Skill", Version: "1.0.0", Registry: "r", Target: "cursor", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "only-claude", Kind: "Skill", Version: "1.0.0", Registry: "r", Target: "claude", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--installed", "--target", "cursor"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --installed --target cursor: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "only-cursor") {
		t.Errorf("output should contain only-cursor; got %q", got)
	}
	if strings.Contains(got, "only-claude") {
		t.Errorf("output should not contain only-claude when filtering by cursor; got %q", got)
	}
}

func TestList_CachedWorkflow_ShowsKindColumn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	cacheDir := installer.CacheDir("my-wf", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactForList(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "my-wf", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "script.js", Files: []string{"script.js"}},
	})
	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "KIND") {
		t.Errorf("output should contain KIND header; got %q", got)
	}
	if !strings.Contains(got, "Workflow") {
		t.Errorf("output should contain Workflow kind; got %q", got)
	}
}

func writeArtifactForList(t *testing.T, dir string, m *artifact.Manifest) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestList_Installed_ShowsKindColumn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "my-wf", Kind: "Workflow", Version: "1.0.0", Registry: "reg", Target: "claude", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--installed"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --installed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "KIND") {
		t.Errorf("output should contain KIND header; got %q", got)
	}
	if !strings.Contains(got, "Workflow") {
		t.Errorf("output should contain Workflow kind; got %q", got)
	}
}

func TestList_Installed_ShowsScopeColumn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--installed"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --installed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "SCOPE") {
		t.Errorf("output should contain SCOPE header; got %q", got)
	}
	if !strings.Contains(got, "global") {
		t.Errorf("output should contain 'global' for entry with empty ProjectPath; got %q", got)
	}
}

func TestList_Installed_ProjectScopeShowsPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	projectPath := "/Users/dev/project-a"
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: projectPath, Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--installed"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --installed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, projectPath) {
		t.Errorf("output should contain project path %q; got %q", projectPath, got)
	}
}

func TestList_Installed_MixedScopes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	projectPath := "/Users/dev/project-a"
	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "global-skill", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "project-skill", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: projectPath, Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--installed"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --installed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "global-skill") {
		t.Errorf("output should contain global-skill; got %q", got)
	}
	if !strings.Contains(got, "project-skill") {
		t.Errorf("output should contain project-skill; got %q", got)
	}
	if !strings.Contains(got, "global") {
		t.Errorf("output should contain 'global' scope; got %q", got)
	}
	if !strings.Contains(got, projectPath) {
		t.Errorf("output should contain project path; got %q", got)
	}
}

func TestList_Installed_ProjectFilter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	projectA := t.TempDir()
	projectB := t.TempDir()

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "global-skill", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "project-a-skill", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: projectA, Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Name: "project-b-skill", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: projectB, Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--installed", "--project", projectA})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --installed --project: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "project-a-skill") {
		t.Errorf("output should contain project-a-skill; got %q", got)
	}
	if strings.Contains(got, "global-skill") {
		t.Errorf("output should NOT contain global-skill when filtering by project; got %q", got)
	}
	if strings.Contains(got, "project-b-skill") {
		t.Errorf("output should NOT contain project-b-skill when filtering by projectA; got %q", got)
	}
}

func TestList_Installed_ProjectFilter_NoMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "global-skill", Kind: "Skill", Version: "1.0.0", Registry: "reg", Target: "cursor", ProjectPath: "", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	nonExistentProject := t.TempDir()

	root := NewRootCommand()
	out := &strings.Builder{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--installed", "--project", nonExistentProject})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --installed --project (no match): %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "No ") {
		t.Errorf("output should indicate no installed artifacts; got %q", got)
	}
}

func TestList_ProjectWithoutInstalled_Errors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	root := NewRootCommand()
	root.SetArgs([]string{"list", "--project", "/some/path"})
	err := root.Execute()
	if err == nil {
		t.Error("list --project without --installed: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "only valid with --installed") {
		t.Errorf("error should mention --project only valid with --installed: %v", err)
	}
}
