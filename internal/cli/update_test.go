package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
)

func pushToLayout(t *testing.T, layoutDir, name, version string) {
	t.Helper()
	baseDir := t.TempDir()
	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: name, Version: version},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# "+name+" "+version), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := "oci:" + layoutDir + ":" + version
	if _, err := oci.Push(context.Background(), m, baseDir, ref); err != nil {
		t.Fatalf("push %s:%s: %v", name, version, err)
	}
}

func setupInstalledEntry(t *testing.T, layoutDir, name, version, target string) {
	t.Helper()
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	entries = append(entries, installer.InstalledEntry{
		Name:     name,
		Kind:     "Skill",
		Version:  version,
		Registry: "oci:" + layoutDir + ":" + version,
		Target:   target,
		Status:   "ok",
	})
	if err := installer.SaveInstalled(entries); err != nil {
		t.Fatal(err)
	}
}

func TestUpdate_Registered(t *testing.T) {
	root := NewRootCommand()
	cmd, _, err := root.Find([]string{"update"})
	if err != nil {
		t.Fatalf("Find(update): %v", err)
	}
	if cmd == nil {
		t.Error("update subcommand not registered")
	}
}

func TestUpdate_HelpExitsZero(t *testing.T) {
	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"update", "--help"})
	if err := root.Execute(); err != nil {
		t.Errorf("striatum update --help: err = %v", err)
	}
}

func TestUpdate_CheckShowsOutdated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutDir := t.TempDir()

	pushToLayout(t, layoutDir, "my-skill", "1.0.0")
	pushToLayout(t, layoutDir, "my-skill", "2.0.0")
	setupInstalledEntry(t, layoutDir, "my-skill", "1.0.0", "claude")

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"update", "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Checking my-skill (oci)...") {
		t.Errorf("output should show checking status with source type: %q", output)
	}
	if !strings.Contains(output, "==> 1 outdated artifact(s):") {
		t.Errorf("output should show outdated header: %q", output)
	}
	if !strings.Contains(output, "1.0.0  →  2.0.0") {
		t.Errorf("output should show version transition: %q", output)
	}
}

func TestUpdate_CheckShowsUpToDate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutDir := t.TempDir()

	pushToLayout(t, layoutDir, "my-skill", "1.0.0")
	setupInstalledEntry(t, layoutDir, "my-skill", "1.0.0", "claude")

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"update", "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "==> All artifacts are up to date.") {
		t.Errorf("output should indicate up to date: %q", output)
	}
}

func TestUpdate_CheckFiltersByTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutDir := t.TempDir()

	pushToLayout(t, layoutDir, "skill-a", "1.0.0")
	pushToLayout(t, layoutDir, "skill-a", "2.0.0")
	setupInstalledEntry(t, layoutDir, "skill-a", "1.0.0", "claude")
	setupInstalledEntry(t, layoutDir, "skill-a", "1.0.0", "cursor")

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"update", "--check", "--target", "cursor"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check --target cursor: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "skill-a") {
		t.Errorf("output should contain skill-a for cursor target: %q", output)
	}
}

func TestUpdate_NothingInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"update", "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check (empty): %v", err)
	}
}

func TestUpdate_InvalidTargetErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	root := NewRootCommand()
	root.SetArgs([]string{"update", "--target", "invalid"})
	err := root.Execute()
	if err == nil {
		t.Error("update --target invalid: expected error")
	}
}

func TestUpdate_UpgradesArtifact(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutDir := t.TempDir()

	pushToLayout(t, layoutDir, "my-skill", "1.0.0")
	pushToLayout(t, layoutDir, "my-skill", "2.0.0")

	// Install v1 via striatum install (oci:path:tag format)
	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"install", "--target", "claude", "oci:" + layoutDir + ":1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install v1: %v", err)
	}

	// Verify v1 is installed
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "my-skill" && e.Version == "1.0.0" {
			found = true
		}
	}
	if !found {
		t.Fatal("my-skill 1.0.0 not found in install DB after install")
	}

	// Run update
	out.Reset()
	root = NewRootCommand()
	root.SetOut(out)
	root.SetIn(strings.NewReader("y\n"))
	root.SetArgs([]string{"update"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out.String(), "==> Updating my-skill (1.0.0 → 2.0.0)") {
		t.Errorf("output should show update progress: %q", out.String())
	}

	// Verify v2 is now installed
	entries, err = installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "my-skill" {
			if e.Version != "2.0.0" {
				t.Errorf("expected version 2.0.0 after update, got %q", e.Version)
			}
			return
		}
	}
	t.Error("my-skill not found in install DB after update")
}

func TestUpdate_IdempotentSecondRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutDir := t.TempDir()

	pushToLayout(t, layoutDir, "my-skill", "1.0.0")
	setupInstalledEntry(t, layoutDir, "my-skill", "1.0.0", "claude")

	// First update (nothing newer)
	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"update"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first update: %v", err)
	}

	// Second update (still nothing newer)
	root = NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"update"})
	if err := root.Execute(); err != nil {
		t.Fatalf("second update: %v", err)
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range entries {
		if e.Name == "my-skill" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 entry for my-skill, got %d", count)
	}
}

func TestRegistryRepoRef(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"quay.io/hbelmiro/my-skill:1.0.0", "quay.io/hbelmiro/my-skill"},
		{"localhost:5000/skills/foo:2.0.0", "localhost:5000/skills/foo"},
		{"oci:/tmp/layout:1.0.0", "oci:/tmp/layout"},
		{"git:https://github.com/org/repo.git@v1.0.0", ""},
		{"", ""},
		{"quay.io/hbelmiro/my-skill", "quay.io/hbelmiro/my-skill"},
	}
	for _, tt := range tests {
		got := registryRepoRef(tt.input)
		if got != tt.want {
			t.Errorf("registryRepoRef(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUpdate_FiltersByNamedArtifact(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutA := t.TempDir()
	layoutB := t.TempDir()

	pushToLayout(t, layoutA, "skill-a", "1.0.0")
	pushToLayout(t, layoutA, "skill-a", "2.0.0")
	pushToLayout(t, layoutB, "skill-b", "1.0.0")
	pushToLayout(t, layoutB, "skill-b", "2.0.0")
	setupInstalledEntry(t, layoutA, "skill-a", "1.0.0", "claude")
	setupInstalledEntry(t, layoutB, "skill-b", "1.0.0", "claude")

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"update", "--check", "skill-a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check skill-a: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "skill-a") {
		t.Errorf("output should contain skill-a: %q", output)
	}
	if strings.Contains(output, "skill-b") {
		t.Errorf("output should NOT contain skill-b when filtering: %q", output)
	}
}

func TestUpdate_CheckFiltersByProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutA := t.TempDir()
	layoutB := t.TempDir()

	pushToLayout(t, layoutA, "skill-a", "1.0.0")
	pushToLayout(t, layoutA, "skill-a", "2.0.0")
	pushToLayout(t, layoutB, "skill-b", "1.0.0")
	pushToLayout(t, layoutB, "skill-b", "2.0.0")

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "skill-a", Kind: "Skill", Version: "1.0.0", Registry: "oci:" + layoutA + ":1.0.0", Target: "claude", ProjectPath: "/project/a", Status: "ok"},
		{Name: "skill-b", Kind: "Skill", Version: "1.0.0", Registry: "oci:" + layoutB + ":1.0.0", Target: "claude", ProjectPath: "/project/b", Status: "ok"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"update", "--check", "--project", "/project/a"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check --project: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "skill-a") {
		t.Errorf("output should contain skill-a: %q", output)
	}
	if strings.Contains(output, "skill-b") {
		t.Errorf("output should NOT contain skill-b when filtering by project: %q", output)
	}
}

func TestUpdate_WarnsOnListTagsFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	badLayout := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(badLayout, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "my-skill", Kind: "Skill", Version: "1.0.0", Registry: "oci:" + badLayout + ":1.0.0", Target: "claude", Status: "ok"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	errBuf := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"update", "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check: %v", err)
	}
	if !strings.Contains(errBuf.String(), "Warning") {
		t.Errorf("stderr should contain warning about failed tag listing: %q", errBuf.String())
	}
	if !strings.Contains(out.String(), "==> All artifacts are up to date.") {
		t.Errorf("output should show all up to date when tags cannot be listed: %q", out.String())
	}
}

func TestUpdate_PartialFailureReportsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutGood := t.TempDir()
	layoutBad := t.TempDir()

	pushToLayout(t, layoutGood, "skill-good", "1.0.0")
	pushToLayout(t, layoutGood, "skill-good", "2.0.0")
	pushToLayout(t, layoutBad, "skill-bad", "1.0.0")
	pushToLayout(t, layoutBad, "skill-bad", "2.0.0")

	setupInstalledEntry(t, layoutGood, "skill-good", "1.0.0", "claude")
	setupInstalledEntry(t, layoutBad, "skill-bad", "1.0.0", "claude")

	if err := os.RemoveAll(filepath.Join(layoutBad, "blobs")); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	errBuf := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"update", "--yes"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected error from partial failure")
	}
	if !strings.Contains(err.Error(), "failed to update") {
		t.Errorf("error should mention failed updates: %v", err)
	}
	if !strings.Contains(errBuf.String(), "skill-bad") {
		t.Errorf("stderr should mention the failed artifact: %q", errBuf.String())
	}
}

func TestUpdate_ConfirmationAborted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutDir := t.TempDir()

	pushToLayout(t, layoutDir, "my-skill", "1.0.0")
	pushToLayout(t, layoutDir, "my-skill", "2.0.0")

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"install", "--target", "claude", "oci:" + layoutDir + ":1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}

	out := &strings.Builder{}
	root = NewRootCommand()
	root.SetOut(out)
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"update"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update: %v", err)
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "my-skill" && e.Version != "1.0.0" {
			t.Errorf("artifact should remain at 1.0.0 after declining, got %q", e.Version)
		}
	}
	if !strings.Contains(out.String(), "Aborted") {
		t.Errorf("output should contain 'Aborted': %q", out.String())
	}
}

func TestUpdate_YesFlagSkipsPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutDir := t.TempDir()

	pushToLayout(t, layoutDir, "my-skill", "1.0.0")
	pushToLayout(t, layoutDir, "my-skill", "2.0.0")

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"install", "--target", "claude", "oci:" + layoutDir + ":1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}

	out := &strings.Builder{}
	root = NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"update", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --yes: %v", err)
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "my-skill" {
			if e.Version != "2.0.0" {
				t.Errorf("expected 2.0.0 after --yes update, got %q", e.Version)
			}
			return
		}
	}
	t.Error("my-skill not found after --yes update")
}

func TestUpdate_UpdatesMultipleArtifacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutA := t.TempDir()
	layoutB := t.TempDir()

	pushToLayout(t, layoutA, "skill-a", "1.0.0")
	pushToLayout(t, layoutA, "skill-a", "2.0.0")
	pushToLayout(t, layoutB, "skill-b", "1.0.0")
	pushToLayout(t, layoutB, "skill-b", "2.0.0")

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"install", "--target", "claude", "oci:" + layoutA + ":1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install skill-a: %v", err)
	}
	root = NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"install", "--target", "claude", "oci:" + layoutB + ":1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install skill-b: %v", err)
	}

	out := &strings.Builder{}
	root = NewRootCommand()
	root.SetOut(out)
	root.SetIn(strings.NewReader("y\n"))
	root.SetArgs([]string{"update"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update: %v", err)
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	versions := make(map[string]string)
	for _, e := range entries {
		versions[e.Name] = e.Version
	}
	if versions["skill-a"] != "2.0.0" {
		t.Errorf("skill-a version = %q, want 2.0.0", versions["skill-a"])
	}
	if versions["skill-b"] != "2.0.0" {
		t.Errorf("skill-b version = %q, want 2.0.0", versions["skill-b"])
	}
	if !strings.Contains(out.String(), "==> Updated 2 artifact(s).") {
		t.Errorf("output should confirm 2 updates: %q", out.String())
	}
	if !strings.Contains(out.String(), "==> Updating skill-a") || !strings.Contains(out.String(), "==> Updating skill-b") {
		t.Errorf("output should show update progress for each artifact: %q", out.String())
	}
}

func TestUpdate_FilterMatchesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutDir := t.TempDir()

	pushToLayout(t, layoutDir, "skill-a", "1.0.0")
	setupInstalledEntry(t, layoutDir, "skill-a", "1.0.0", "claude")

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"update", "--check", "nonexistent-skill"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check nonexistent-skill: %v", err)
	}
	if !strings.Contains(out.String(), "No matching artifacts found.") {
		t.Errorf("output should say no matching artifacts: %q", out.String())
	}
}

func TestUpdate_GitArtifactSkipped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layoutDir := t.TempDir()

	pushToLayout(t, layoutDir, "oci-skill", "1.0.0")
	pushToLayout(t, layoutDir, "oci-skill", "2.0.0")

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Name: "git-skill", Kind: "Skill", Version: "1.0.0", Registry: "git:https://github.com/org/repo.git@v1.0.0", Target: "claude", Status: "ok"},
		{Name: "oci-skill", Kind: "Skill", Version: "1.0.0", Registry: "oci:" + layoutDir + ":1.0.0", Target: "claude", Status: "ok"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"update", "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "oci-skill") {
		t.Errorf("output should show outdated oci-skill: %q", output)
	}
	if !strings.Contains(output, "Checking git-skill (git)...") {
		t.Errorf("output should show git source type: %q", output)
	}
	if !strings.Contains(output, "Checking oci-skill (oci)...") {
		t.Errorf("output should show oci source type: %q", output)
	}
	if !strings.Contains(output, "==> git-skill: installed from git, not auto-updatable.") {
		t.Errorf("output should warn git artifact is not auto-updatable: %q", output)
	}
	summaryIdx := strings.Index(output, "==> 1 outdated")
	if summaryIdx < 0 {
		t.Fatalf("expected outdated summary in output: %q", output)
	}
	summary := output[summaryIdx:]
	if strings.Contains(summary, "git-skill") {
		t.Errorf("git-skill should not appear in outdated summary: %q", summary)
	}
}
