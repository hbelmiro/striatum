package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

const validManifestJSON = `{
  "apiVersion": "striatum.dev/v1alpha1",
  "kind": "Skill",
  "metadata": {
    "name": "my-skill",
    "version": "1.0.0",
    "description": "A test skill",
    "authors": ["author1"],
    "license": "Apache-2.0",
    "tags": ["tag1"]
  },
  "spec": {
    "entrypoint": "SKILL.md",
    "files": ["SKILL.md", "other.md"]
  },
  "dependencies": [
    {"name": "dep1", "version": "1.0.0", "registry": "localhost:5000/skills"}
  ]
}`

func TestLoad_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	if err := os.WriteFile(path, []byte(validManifestJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load() err = %v", err)
	}
	if m.APIVersion != "striatum.dev/v1alpha1" {
		t.Errorf("APIVersion = %q, want striatum.dev/v1alpha1", m.APIVersion)
	}
	if m.Kind != "Skill" {
		t.Errorf("Kind = %q, want Skill", m.Kind)
	}
	if m.Metadata.Name != "my-skill" {
		t.Errorf("Metadata.Name = %q, want my-skill", m.Metadata.Name)
	}
	if m.Metadata.Version != "1.0.0" {
		t.Errorf("Metadata.Version = %q, want 1.0.0", m.Metadata.Version)
	}
	if m.Spec.Entrypoint != "SKILL.md" {
		t.Errorf("Spec.Entrypoint = %q, want SKILL.md", m.Spec.Entrypoint)
	}
	if len(m.Spec.Files) != 2 || m.Spec.Files[0] != "SKILL.md" || m.Spec.Files[1] != "other.md" {
		t.Errorf("Spec.Files = %v", m.Spec.Files)
	}
	if len(m.Dependencies) != 1 || m.Dependencies[0].Name != "dep1" || m.Dependencies[0].Version != "1.0.0" {
		t.Errorf("Dependencies = %v", m.Dependencies)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Error("Load() err = nil, want error")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	if err := os.WriteFile(path, []byte(`{invalid`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("Load() err = nil, want error")
	}
}

func TestValidate_NilManifest(t *testing.T) {
	if err := Validate(nil); err == nil {
		t.Error("Validate(nil) err = nil, want error")
	}
}

func TestValidate_ValidManifest(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err != nil {
		t.Errorf("Validate() err = %v", err)
	}
}

func TestValidate_InvalidAPIVersion(t *testing.T) {
	m := &Manifest{
		APIVersion: "v1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for invalid apiVersion")
	}
}

func TestValidate_UnsupportedKind(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Unknown",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for unsupported kind")
	}
}

func TestValidate_EmptyName(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for empty name")
	}
}

func TestValidate_EmptyVersion(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: ""},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for empty version")
	}
}

func TestValidate_EntrypointNotInFiles(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"other.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error when entrypoint not in files")
	}
}

func TestValidate_EmptyFiles(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for empty files")
	}
}

func TestValidate_DependencyMissingName(t *testing.T) {
	m := &Manifest{
		APIVersion:   "striatum.dev/v1alpha1",
		Kind:         "Skill",
		Metadata:     Metadata{Name: "x", Version: "1.0.0"},
		Spec:         Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []Dependency{{Name: "", Version: "1.0.0"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for dependency with empty name")
	}
}

func TestValidate_DependencyMissingVersion(t *testing.T) {
	m := &Manifest{
		APIVersion:   "striatum.dev/v1alpha1",
		Kind:         "Skill",
		Metadata:     Metadata{Name: "x", Version: "1.0.0"},
		Spec:         Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []Dependency{{Name: "dep", Version: ""}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for dependency with empty version")
	}
}

func TestValidateLocal_AllFilesExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := ValidateLocal(m, dir); err != nil {
		t.Errorf("ValidateLocal() err = %v", err)
	}
}

func TestValidateLocal_FileMissing(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "missing.md"}},
	}
	if err := ValidateLocal(m, dir); err == nil {
		t.Error("ValidateLocal() err = nil, want error for missing file")
	}
}

func TestValidateLocal_NilManifest(t *testing.T) {
	if err := ValidateLocal(nil, t.TempDir()); err == nil {
		t.Error("ValidateLocal(nil, dir) err = nil, want error")
	}
}

func TestValidateLocal_InvalidPath_Empty(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", ""}},
	}
	if err := ValidateLocal(m, dir); err == nil {
		t.Error("ValidateLocal() with empty path in files: err = nil, want error")
	}
}

func TestValidateLocal_InvalidPath_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "../other.md"}},
	}
	if err := ValidateLocal(m, dir); err == nil {
		t.Error("ValidateLocal() with .. in path: err = nil, want error")
	}
}

func TestValidateLocal_InvalidPath_Absolute(t *testing.T) {
	dir := t.TempDir()
	absPath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(absPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{absPath}},
	}
	if err := ValidateLocal(m, dir); err == nil {
		t.Error("ValidateLocal() with absolute path in files: err = nil, want error")
	}
}
