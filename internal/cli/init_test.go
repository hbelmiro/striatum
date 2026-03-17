package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

func TestInit_CreatesArtifactJSON(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"init", "--name", "my-skill"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	path := filepath.Join(dir, "artifact.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("artifact.json not created: %v", err)
	}
	m, err := artifact.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Metadata.Name != "my-skill" {
		t.Errorf("name = %q, want my-skill", m.Metadata.Name)
	}
	if m.Metadata.Version != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", m.Metadata.Version)
	}
	if m.Kind != "Skill" {
		t.Errorf("kind = %q, want Skill", m.Kind)
	}
	if m.Spec.Entrypoint != "SKILL.md" {
		t.Errorf("entrypoint = %q, want SKILL.md", m.Spec.Entrypoint)
	}
	if len(m.Spec.Files) != 1 || m.Spec.Files[0] != "SKILL.md" {
		t.Errorf("files = %v", m.Spec.Files)
	}
	if err := artifact.Validate(m); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestInit_WithVersionAndKind(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"init", "--name", "x", "--version", "2.0.0", "--kind", "Skill"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	m, err := artifact.Load(filepath.Join(dir, "artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Metadata.Version != "2.0.0" {
		t.Errorf("version = %q, want 2.0.0", m.Metadata.Version)
	}
}

func TestInit_RequiresName(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"init"})
	err := root.Execute()
	if err == nil {
		t.Error("init without --name: expected error, got nil")
	}
}

func TestInit_CustomEntrypoint(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"init", "--name", "x", "--entrypoint", "OTHER.md"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	m, err := artifact.Load(filepath.Join(dir, "artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Spec.Entrypoint != "OTHER.md" {
		t.Errorf("entrypoint = %q, want OTHER.md", m.Spec.Entrypoint)
	}
	if len(m.Spec.Files) != 1 || m.Spec.Files[0] != "OTHER.md" {
		t.Errorf("files = %v, want [OTHER.md]", m.Spec.Files)
	}
}

func TestInit_OverwritesExistingArtifactJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	if err := os.WriteFile(path, []byte(`{"apiVersion":"striatum.dev/v1alpha1","kind":"Skill","metadata":{"name":"old","version":"0.0.1"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"init", "--name", "new-skill", "--version", "1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	m, err := artifact.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Metadata.Name != "new-skill" || m.Metadata.Version != "1.0.0" {
		t.Errorf("manifest not overwritten: name = %q, version = %q", m.Metadata.Name, m.Metadata.Version)
	}
}
