package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_NoArtifactJSON(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"validate"})
	err := root.Execute()
	if err == nil {
		t.Error("validate with no artifact.json: expected error, got nil")
	}
}

func TestValidate_InvalidSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	if err := os.WriteFile(path, []byte(`{"apiVersion":"v1","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"validate"})
	err := root.Execute()
	if err == nil {
		t.Error("validate with invalid schema: expected error, got nil")
	}
}

func TestValidate_FileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	valid := `{"apiVersion":"striatum.dev/v1alpha1","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md","other.md"]}}`
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	// Create only SKILL.md, not other.md
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"validate"})
	err := root.Execute()
	if err == nil {
		t.Error("validate with missing file: expected error, got nil")
	}
}

func TestValidate_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	valid := `{"apiVersion":"striatum.dev/v1alpha1","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md"]}}`
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"validate"})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "valid") {
		t.Errorf("output %q does not contain valid", got)
	}
	if !strings.Contains(got, "exist") {
		t.Errorf("output %q does not contain exist", got)
	}
}
