package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_NoArtifactJSON(t *testing.T) {
	t.Chdir(t.TempDir())

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
	t.Chdir(dir)

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
	valid := `{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md","other.md"]}}`
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

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
	valid := `{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md"]}}`
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

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

func TestValidate_CheckDepsNoDeps_Succeeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	valid := `{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md"]}}`
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"validate", "--check-deps"})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate --check-deps: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Resolving dependency tree") {
		t.Errorf("output %q does not contain Resolving dependency tree", got)
	}
	if !strings.Contains(got, "All dependencies resolved") {
		t.Errorf("output %q does not contain All dependencies resolved", got)
	}
}

func TestValidate_CheckDepsWithDeps_FailsOnUnreachable(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": "x", "version": "1.0.0"},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
  "dependencies": [
    {"source": "oci", "registry": "localhost:9999", "repository": "skills/dep", "tag": "1.0.0"}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	root := NewRootCommand()
	root.SetArgs([]string{"validate", "--check-deps"})
	err := root.Execute()
	if err == nil {
		t.Fatal("validate --check-deps with unreachable dep: expected error")
	}
	if !strings.Contains(err.Error(), "resolving dependencies") {
		t.Errorf("error should mention resolving dependencies: %v", err)
	}
}

func TestValidate_SuccessWithManifestFlagFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	cwd := t.TempDir()
	valid := `{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md"]}}`
	if err := os.WriteFile(filepath.Join(projectDir, "artifact.json"), []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "SKILL.md"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"validate", "--manifest", filepath.Join(projectDir, "artifact.json")})
	if err := root.Execute(); err != nil {
		t.Fatalf("validate --manifest from other dir: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "valid") {
		t.Errorf("output %q does not contain valid", got)
	}
}

func TestValidate_FileMissingWithManifestFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	cwd := t.TempDir()
	valid := `{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md","other.md"]}}`
	if err := os.WriteFile(filepath.Join(projectDir, "artifact.json"), []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	root := NewRootCommand()
	root.SetArgs([]string{"validate", "-f", filepath.Join(projectDir, "artifact.json")})
	if err := root.Execute(); err == nil {
		t.Fatal("validate -f with missing spec file: expected error, got nil")
	}
}
