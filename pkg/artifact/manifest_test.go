package artifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Dependency type unit tests ---

func TestOCIDependency_Source(t *testing.T) {
	d := &OCIDependency{RegistryHost: "localhost:5000", Repository: "skills/base", Tag: "1.0.0"}
	if got := d.Source(); got != "oci" {
		t.Errorf("Source() = %q, want %q", got, "oci")
	}
}

func TestGitDependency_Source(t *testing.T) {
	d := &GitDependency{URL: "https://example.com/repo.git", Ref: "v1.0.0"}
	if got := d.Source(); got != "git" {
		t.Errorf("Source() = %q, want %q", got, "git")
	}
}

func TestOCIDependency_CanonicalRef(t *testing.T) {
	d := &OCIDependency{RegistryHost: "localhost:5000", Repository: "skills/base", Tag: "1.0.0"}
	want := "localhost:5000/skills/base:1.0.0"
	if got := d.CanonicalRef(); got != want {
		t.Errorf("CanonicalRef() = %q, want %q", got, want)
	}
}

func TestGitDependency_CanonicalRef(t *testing.T) {
	tests := []struct {
		name string
		dep  *GitDependency
		want string
	}{
		{
			name: "without path",
			dep:  &GitDependency{URL: "https://example.com/repo.git", Ref: "v1.0.0"},
			want: "git:https://example.com/repo.git@v1.0.0",
		},
		{
			name: "with path",
			dep:  &GitDependency{URL: "https://example.com/repo.git", Ref: "v1.0.0", Path: "packages/skill"},
			want: "git:https://example.com/repo.git@v1.0.0#packages/skill",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dep.CanonicalRef(); got != tt.want {
				t.Errorf("CanonicalRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOCIDependency_Validate(t *testing.T) {
	tests := []struct {
		name    string
		dep     *OCIDependency
		wantErr string
	}{
		{"valid", &OCIDependency{"localhost:5000", "skills/base", "1.0.0"}, ""},
		{"missing registry", &OCIDependency{"", "skills/base", "1.0.0"}, "registry"},
		{"whitespace registry", &OCIDependency{"  ", "skills/base", "1.0.0"}, "registry"},
		{"missing repository", &OCIDependency{"localhost:5000", "", "1.0.0"}, "repository"},
		{"missing tag", &OCIDependency{"localhost:5000", "skills/base", ""}, "tag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dep.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() err = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Validate() err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestGitDependency_Validate(t *testing.T) {
	tests := []struct {
		name    string
		dep     *GitDependency
		wantErr string
	}{
		{"valid", &GitDependency{URL: "https://example.com/repo.git", Ref: "v1.0.0"}, ""},
		{"valid with path", &GitDependency{URL: "https://example.com/repo.git", Ref: "v1.0.0", Path: "sub"}, ""},
		{"missing url", &GitDependency{URL: "", Ref: "v1.0.0"}, "url"},
		{"whitespace url", &GitDependency{URL: "  ", Ref: "v1.0.0"}, "url"},
		{"missing ref", &GitDependency{URL: "https://example.com/repo.git", Ref: ""}, "ref"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dep.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() err = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Validate() err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// --- Unmarshal tests ---

func TestManifest_UnmarshalJSON_OCIDep(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
		"dependencies": [
			{"source": "oci", "registry": "localhost:5000", "repository": "skills/base", "tag": "1.0.0"}
		]
	}`
	var m Manifest
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("UnmarshalJSON() err = %v", err)
	}
	if len(m.Dependencies) != 1 {
		t.Fatalf("len(Dependencies) = %d, want 1", len(m.Dependencies))
	}
	d, ok := m.Dependencies[0].(*OCIDependency)
	if !ok {
		t.Fatalf("Dependencies[0] type = %T, want *OCIDependency", m.Dependencies[0])
	}
	if d.RegistryHost != "localhost:5000" || d.Repository != "skills/base" || d.Tag != "1.0.0" {
		t.Errorf("OCIDependency = %+v", d)
	}
}

func TestManifest_UnmarshalJSON_GitDep(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
		"dependencies": [
			{"source": "git", "url": "https://example.com/repo.git", "ref": "v1.0.0", "path": "sub"}
		]
	}`
	var m Manifest
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("UnmarshalJSON() err = %v", err)
	}
	if len(m.Dependencies) != 1 {
		t.Fatalf("len(Dependencies) = %d, want 1", len(m.Dependencies))
	}
	d, ok := m.Dependencies[0].(*GitDependency)
	if !ok {
		t.Fatalf("Dependencies[0] type = %T, want *GitDependency", m.Dependencies[0])
	}
	if d.URL != "https://example.com/repo.git" || d.Ref != "v1.0.0" || d.Path != "sub" {
		t.Errorf("GitDependency = %+v", d)
	}
}

func TestManifest_UnmarshalJSON_MixedDeps(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
		"dependencies": [
			{"source": "oci", "registry": "reg", "repository": "repo", "tag": "v1"},
			{"source": "git", "url": "https://example.com/repo.git", "ref": "main"}
		]
	}`
	var m Manifest
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("UnmarshalJSON() err = %v", err)
	}
	if len(m.Dependencies) != 2 {
		t.Fatalf("len(Dependencies) = %d, want 2", len(m.Dependencies))
	}
	if _, ok := m.Dependencies[0].(*OCIDependency); !ok {
		t.Errorf("Dependencies[0] type = %T, want *OCIDependency", m.Dependencies[0])
	}
	if _, ok := m.Dependencies[1].(*GitDependency); !ok {
		t.Errorf("Dependencies[1] type = %T, want *GitDependency", m.Dependencies[1])
	}
}

func TestManifest_UnmarshalJSON_NoDeps(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]}
	}`
	var m Manifest
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("UnmarshalJSON() err = %v", err)
	}
	if len(m.Dependencies) != 0 {
		t.Errorf("len(Dependencies) = %d, want 0", len(m.Dependencies))
	}
}

func TestManifest_UnmarshalJSON_EmptyDepsArray(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
		"dependencies": []
	}`
	var m Manifest
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("UnmarshalJSON() err = %v", err)
	}
	if len(m.Dependencies) != 0 {
		t.Errorf("len(Dependencies) = %d, want 0", len(m.Dependencies))
	}
}

func TestManifest_UnmarshalJSON_UnknownSource(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
		"dependencies": [
			{"source": "zip", "url": "https://example.com/archive.zip"}
		]
	}`
	var m Manifest
	err := json.Unmarshal([]byte(data), &m)
	if err == nil {
		t.Fatal("UnmarshalJSON() err = nil, want error for unknown source")
	}
	if !strings.Contains(err.Error(), "unsupported source") {
		t.Errorf("error %q should mention unsupported source", err.Error())
	}
}

func TestManifest_UnmarshalJSON_MissingSource(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
		"dependencies": [
			{"registry": "localhost:5000", "repository": "repo", "tag": "1.0.0"}
		]
	}`
	var m Manifest
	err := json.Unmarshal([]byte(data), &m)
	if err == nil {
		t.Fatal("UnmarshalJSON() err = nil, want error for missing source")
	}
	if !strings.Contains(err.Error(), "source is required") {
		t.Errorf("error %q should mention source is required", err.Error())
	}
}

func TestManifest_UnmarshalJSON_MalformedOCIDep(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
		"dependencies": [
			{"source": "oci", "registry": 123, "repository": "repo", "tag": "1.0.0"}
		]
	}`
	var m Manifest
	err := json.Unmarshal([]byte(data), &m)
	if err == nil {
		t.Fatal("UnmarshalJSON() err = nil, want error for wrong-typed OCI field")
	}
	if !strings.Contains(err.Error(), "dependencies[0]") {
		t.Errorf("error should mention index: %v", err)
	}
}

func TestManifest_UnmarshalJSON_MalformedGitDep(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
		"dependencies": [
			{"source": "git", "url": ["not-a-string"], "ref": "v1"}
		]
	}`
	var m Manifest
	err := json.Unmarshal([]byte(data), &m)
	if err == nil {
		t.Fatal("UnmarshalJSON() err = nil, want error for wrong-typed Git field")
	}
	if !strings.Contains(err.Error(), "dependencies[0]") {
		t.Errorf("error should mention index: %v", err)
	}
}

func TestManifest_UnmarshalJSON_InvalidDependencyJSON(t *testing.T) {
	data := `{
		"apiVersion": "striatum.dev/v1alpha2",
		"kind": "Skill",
		"metadata": {"name": "x", "version": "1.0.0"},
		"spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]},
		"dependencies": ["not-an-object"]
	}`
	var m Manifest
	if err := json.Unmarshal([]byte(data), &m); err == nil {
		t.Fatal("UnmarshalJSON() err = nil, want error for non-object dependency")
	}
}

// --- Marshal / roundtrip tests ---

func TestManifest_MarshalJSON_IncludesSource(t *testing.T) {
	m := Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []Dependency{
			&OCIDependency{RegistryHost: "reg", Repository: "repo", Tag: "v1"},
			&GitDependency{URL: "https://example.com/r.git", Ref: "main"},
		},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("MarshalJSON() err = %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"source":"oci"`) {
		t.Errorf("marshaled JSON should contain source=oci: %s", s)
	}
	if !strings.Contains(s, `"source":"git"`) {
		t.Errorf("marshaled JSON should contain source=git: %s", s)
	}
}

func TestManifest_MarshalJSON_NoDeps(t *testing.T) {
	m := Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("MarshalJSON() err = %v", err)
	}
	if strings.Contains(string(data), `"dependencies"`) {
		t.Errorf("marshaled JSON should omit empty dependencies: %s", data)
	}
}

func TestManifest_Roundtrip(t *testing.T) {
	original := Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "test", Version: "2.0.0", Description: "desc"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "lib.md"}},
		Dependencies: []Dependency{
			&OCIDependency{RegistryHost: "reg.io", Repository: "skills/a", Tag: "1.0.0"},
			&GitDependency{URL: "https://gh.com/o/r.git", Ref: "v3.0.0", Path: "sub"},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal err = %v", err)
	}
	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal err = %v", err)
	}
	if decoded.APIVersion != original.APIVersion {
		t.Errorf("APIVersion = %q", decoded.APIVersion)
	}
	if decoded.Metadata.Name != original.Metadata.Name || decoded.Metadata.Description != original.Metadata.Description {
		t.Errorf("Metadata = %+v", decoded.Metadata)
	}
	if len(decoded.Spec.Files) != 2 {
		t.Errorf("Spec.Files = %v", decoded.Spec.Files)
	}
	if len(decoded.Dependencies) != 2 {
		t.Fatalf("len(Dependencies) = %d, want 2", len(decoded.Dependencies))
	}

	oci, ok := decoded.Dependencies[0].(*OCIDependency)
	if !ok {
		t.Fatalf("dep[0] type = %T", decoded.Dependencies[0])
	}
	if oci.RegistryHost != "reg.io" || oci.Repository != "skills/a" || oci.Tag != "1.0.0" {
		t.Errorf("OCI = %+v", oci)
	}

	git, ok := decoded.Dependencies[1].(*GitDependency)
	if !ok {
		t.Fatalf("dep[1] type = %T", decoded.Dependencies[1])
	}
	if git.URL != "https://gh.com/o/r.git" || git.Ref != "v3.0.0" || git.Path != "sub" {
		t.Errorf("Git = %+v", git)
	}
}

// --- Load tests ---

const validManifestJSON = `{
  "apiVersion": "striatum.dev/v1alpha2",
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
    {"source": "oci", "registry": "localhost:5000", "repository": "skills/dep1", "tag": "1.0.0"}
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
	if m.APIVersion != "striatum.dev/v1alpha2" {
		t.Errorf("APIVersion = %q", m.APIVersion)
	}
	if m.Kind != "Skill" {
		t.Errorf("Kind = %q", m.Kind)
	}
	if m.Metadata.Name != "my-skill" {
		t.Errorf("Metadata.Name = %q", m.Metadata.Name)
	}
	if m.Metadata.Version != "1.0.0" {
		t.Errorf("Metadata.Version = %q", m.Metadata.Version)
	}
	if len(m.Dependencies) != 1 {
		t.Fatalf("len(Dependencies) = %d, want 1", len(m.Dependencies))
	}
	oci, ok := m.Dependencies[0].(*OCIDependency)
	if !ok {
		t.Fatalf("dep type = %T, want *OCIDependency", m.Dependencies[0])
	}
	if oci.Repository != "skills/dep1" {
		t.Errorf("dep Repository = %q", oci.Repository)
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

// --- Validate tests ---

func TestValidate_NilManifest(t *testing.T) {
	if err := Validate(nil); err == nil {
		t.Error("Validate(nil) err = nil, want error")
	}
}

func TestValidate_ValidManifest(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err != nil {
		t.Errorf("Validate() err = %v", err)
	}
}

func TestValidate_ValidManifestWithDeps(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []Dependency{
			&OCIDependency{RegistryHost: "localhost:5000", Repository: "repo", Tag: "1.0.0"},
			&GitDependency{URL: "https://example.com/r.git", Ref: "v1.0.0"},
		},
	}
	if err := Validate(m); err != nil {
		t.Errorf("Validate() err = %v", err)
	}
}

func TestValidate_RejectsV1alpha1(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	err := Validate(m)
	if err == nil {
		t.Fatal("Validate() err = nil, want error for v1alpha1")
	}
	if !strings.Contains(err.Error(), "v1alpha1") {
		t.Errorf("error should mention v1alpha1: %v", err)
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
		APIVersion: "striatum.dev/v1alpha2",
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
		APIVersion: "striatum.dev/v1alpha2",
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
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: ""},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for empty version")
	}
}

func TestValidate_EmptyEntrypoint(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for empty entrypoint")
	}
}

func TestValidate_EntrypointNotInFiles(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha2",
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
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for empty files")
	}
}

func TestValidate_DependencyValidationDelegated(t *testing.T) {
	tests := []struct {
		name   string
		dep    Dependency
		errMsg string
	}{
		{"invalid OCI dep", &OCIDependency{RegistryHost: "", Repository: "repo", Tag: "1.0.0"}, "registry"},
		{"invalid Git dep", &GitDependency{URL: "", Ref: "v1.0.0"}, "url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				APIVersion:   "striatum.dev/v1alpha2",
				Kind:         "Skill",
				Metadata:     Metadata{Name: "x", Version: "1.0.0"},
				Spec:         Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
				Dependencies: []Dependency{tt.dep},
			}
			err := Validate(m)
			if err == nil {
				t.Fatal("Validate() err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

// --- ValidateLocal tests ---

func TestValidateLocal_AllFilesExist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Spec: Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}}}
	if err := ValidateLocal(m, dir); err != nil {
		t.Errorf("ValidateLocal() err = %v", err)
	}
}

func TestValidateLocal_FileMissing(t *testing.T) {
	m := &Manifest{Spec: Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "missing.md"}}}
	if err := ValidateLocal(m, t.TempDir()); err == nil {
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
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Spec: Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", ""}}}
	err := ValidateLocal(m, dir)
	if err == nil {
		t.Fatal("ValidateLocal() err = nil, want error for empty path")
	}
	if !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("error %q should contain 'invalid file path'", err.Error())
	}
}

func TestValidateLocal_InvalidPath_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Spec: Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "../other.md"}}}
	err := ValidateLocal(m, dir)
	if err == nil {
		t.Fatal("ValidateLocal() err = nil, want error for path traversal")
	}
	if !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("error %q should contain 'invalid file path'", err.Error())
	}
}

func TestValidate_WhitespaceOnlyName(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "   ", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for whitespace-only name")
	}
}

func TestValidate_WhitespaceOnlyVersion(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "  \t "},
		Spec:       Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for whitespace-only version")
	}
}

func TestValidate_WhitespaceOnlyEntrypoint(t *testing.T) {
	m := &Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   Metadata{Name: "x", Version: "1.0.0"},
		Spec:       Spec{Entrypoint: "   ", Files: []string{"SKILL.md"}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() err = nil, want error for whitespace-only entrypoint")
	}
}

func TestLoad_SucceedsThenValidateFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	data := `{"apiVersion":"bad","kind":"Skill","metadata":{"name":"x","version":"1"},"spec":{"entrypoint":"a","files":["a"]}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load() should succeed for syntactically valid JSON, got err = %v", err)
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() should reject loaded manifest with bad apiVersion")
	}
}

func TestValidateLocal_StatError_NonNotExist(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(subDir, 0o755)
	})

	m := &Manifest{Spec: Spec{Entrypoint: "SKILL.md", Files: []string{"sub/nested.md"}}}
	err := ValidateLocal(m, dir)
	if err == nil {
		t.Skip("OS does not restrict stat on mode 000 dirs (e.g. running as root)")
	}
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("error should be a stat error, not 'not found': %v", err)
	}
}

func TestValidateLocal_InvalidPath_Absolute(t *testing.T) {
	dir := t.TempDir()
	absPath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(absPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Spec: Spec{Entrypoint: "SKILL.md", Files: []string{absPath}}}
	err := ValidateLocal(m, dir)
	if err == nil {
		t.Fatal("ValidateLocal() err = nil, want error for absolute path")
	}
	if !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("error %q should contain 'invalid file path'", err.Error())
	}
}
