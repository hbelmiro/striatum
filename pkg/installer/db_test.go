package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDB_LoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	entries, err := LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0", len(entries))
	}
}

func TestDB_LoadValidYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	path := filepath.Join(dir, "installed.yaml")
	content := `global:
  - skill: foo
    version: "1.0.0"
    registry: localhost:5000/skills
    target: cursor
    status: ok
    updated_at: "2026-01-15T12:00:00Z"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Skill != "foo" || e.Version != "1.0.0" || e.Target != "cursor" || e.InstalledWith != "" || e.Status != "ok" {
		t.Errorf("entry = %+v", e)
	}
}

func TestDB_LoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	path := filepath.Join(dir, "installed.yaml")
	if err := os.WriteFile(path, []byte("invalid: [[["), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadInstalled()
	if err == nil {
		t.Error("LoadInstalled(invalid YAML) want error")
	}
}

func TestDB_SaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	entries := []InstalledEntry{
		{
			Skill:         "a",
			Version:       "1.0.0",
			Registry:      "reg",
			Target:        "cursor",
			ProjectPath:   "/p",
			InstalledWith: "root-a",
			Status:        "ok",
			LastError:     "",
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := SaveInstalled(entries); err != nil {
		t.Fatalf("SaveInstalled: %v", err)
	}
	loaded, err := LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("len(loaded) = %d", len(loaded))
	}
	if loaded[0].Skill != entries[0].Skill || loaded[0].InstalledWith != entries[0].InstalledWith {
		t.Errorf("round-trip: got %+v", loaded[0])
	}
}

func TestDB_SaveEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	if err := SaveInstalled(nil); err != nil {
		t.Fatalf("SaveInstalled(nil): %v", err)
	}
	entries, err := LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0", len(entries))
	}
}

func TestDB_LoadNewFormat_GlobalOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	path := filepath.Join(dir, "installed.yaml")
	content := `global:
  - skill: go-review
    version: 1.0.0
    registry: quay.io/skills
    target: cursor
    status: ok
    updated_at: "2026-01-01T00:00:00Z"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Skill != "go-review" {
		t.Errorf("Skill = %q, want go-review", e.Skill)
	}
	if e.ProjectPath != "" {
		t.Errorf("ProjectPath = %q, want empty (global)", e.ProjectPath)
	}
}

func TestDB_LoadNewFormat_ProjectsOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	path := filepath.Join(dir, "installed.yaml")
	content := `projects:
  /Users/dev/project-a:
    - skill: go-review
      version: 2.0.0
      registry: localhost:5050/skills
      target: cursor
      installed_with: pipelines-review
      status: ok
      updated_at: "2026-01-01T00:00:00Z"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Skill != "go-review" {
		t.Errorf("Skill = %q, want go-review", e.Skill)
	}
	if e.ProjectPath != "/Users/dev/project-a" {
		t.Errorf("ProjectPath = %q, want /Users/dev/project-a", e.ProjectPath)
	}
}

func TestDB_LoadNewFormat_GlobalAndProjects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	path := filepath.Join(dir, "installed.yaml")
	content := `global:
  - skill: go-review
    version: 1.0.0
    registry: quay.io/skills
    target: cursor
    status: ok
    updated_at: "2026-01-01T00:00:00Z"
projects:
  /Users/dev/project-a:
    - skill: go-review
      version: 2.0.0
      registry: localhost:5050/skills
      target: cursor
      installed_with: pipelines-review
      status: ok
      updated_at: "2026-01-01T00:00:00Z"
  /Users/dev/project-b:
    - skill: python-lint
      version: 1.0.0
      registry: quay.io/skills
      target: claude
      status: ok
      updated_at: "2026-01-01T00:00:00Z"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}

	// Verify global entry
	var globalEntry *InstalledEntry
	var projectAEntry *InstalledEntry
	var projectBEntry *InstalledEntry

	for i := range entries {
		e := &entries[i]
		if e.ProjectPath == "" && e.Skill == "go-review" {
			globalEntry = e
		} else if e.ProjectPath == "/Users/dev/project-a" {
			projectAEntry = e
		} else if e.ProjectPath == "/Users/dev/project-b" {
			projectBEntry = e
		}
	}

	if globalEntry == nil {
		t.Error("global entry not found")
	} else if globalEntry.Version != "1.0.0" {
		t.Errorf("global entry version = %q, want 1.0.0", globalEntry.Version)
	}

	if projectAEntry == nil {
		t.Error("project-a entry not found")
	} else if projectAEntry.Version != "2.0.0" {
		t.Errorf("project-a entry version = %q, want 2.0.0", projectAEntry.Version)
	}

	if projectBEntry == nil {
		t.Error("project-b entry not found")
	} else if projectBEntry.Skill != "python-lint" {
		t.Errorf("project-b entry skill = %q, want python-lint", projectBEntry.Skill)
	}
}

func TestDB_SaveRoundTrip_NewFormat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	entries := []InstalledEntry{
		{
			Skill:       "go-review",
			Version:     "1.0.0",
			Registry:    "quay.io/skills",
			Target:      "cursor",
			ProjectPath: "",
			Status:      "ok",
			UpdatedAt:   "2026-01-01T00:00:00Z",
		},
		{
			Skill:         "go-review",
			Version:       "2.0.0",
			Registry:      "localhost:5050/skills",
			Target:        "cursor",
			ProjectPath:   "/Users/dev/project-a",
			InstalledWith: "pipelines-review",
			Status:        "ok",
			UpdatedAt:     "2026-01-01T00:00:00Z",
		},
	}
	if err := SaveInstalled(entries); err != nil {
		t.Fatalf("SaveInstalled: %v", err)
	}
	loaded, err := LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("len(loaded) = %d, want 2", len(loaded))
	}

	// Find global and project entries
	var globalEntry *InstalledEntry
	var projectEntry *InstalledEntry

	for i := range loaded {
		e := &loaded[i]
		switch e.ProjectPath {
		case "":
			globalEntry = e
		case "/Users/dev/project-a":
			projectEntry = e
		}
	}

	if globalEntry == nil {
		t.Fatal("global entry not found after roundtrip")
	}
	if globalEntry.Version != "1.0.0" {
		t.Errorf("global entry version = %q, want 1.0.0", globalEntry.Version)
	}

	if projectEntry == nil {
		t.Fatal("project entry not found after roundtrip")
	}
	if projectEntry.Version != "2.0.0" {
		t.Errorf("project entry version = %q, want 2.0.0", projectEntry.Version)
	}
	if projectEntry.InstalledWith != "pipelines-review" {
		t.Errorf("project entry InstalledWith = %q, want pipelines-review", projectEntry.InstalledWith)
	}
}

func TestDB_SaveRoundTrip_GlobalOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	entries := []InstalledEntry{
		{
			Skill:     "skill-a",
			Version:   "1.0.0",
			Registry:  "reg",
			Target:    "cursor",
			Status:    "ok",
			UpdatedAt: "2026-01-01T00:00:00Z",
		},
	}
	if err := SaveInstalled(entries); err != nil {
		t.Fatalf("SaveInstalled: %v", err)
	}

	// Read raw YAML to verify no projects: key
	path := filepath.Join(dir, "installed.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	yamlStr := string(data)

	// Should have global: but not projects:
	if !strings.Contains(yamlStr, "global:") {
		t.Error("YAML should contain 'global:' section")
	}
	if strings.Contains(yamlStr, "projects:") {
		t.Error("YAML should NOT contain 'projects:' section when only global entries")
	}
}

func TestDB_Save_ProjectPathNotInYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	entries := []InstalledEntry{
		{
			Skill:       "skill-a",
			Version:     "1.0.0",
			Registry:    "reg",
			Target:      "cursor",
			ProjectPath: "/Users/dev/project-a",
			Status:      "ok",
			UpdatedAt:   "2026-01-01T00:00:00Z",
		},
	}
	if err := SaveInstalled(entries); err != nil {
		t.Fatalf("SaveInstalled: %v", err)
	}

	// Read raw YAML
	path := filepath.Join(dir, "installed.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	yamlStr := string(data)

	// Should NOT contain project_path as a field in entries
	if strings.Contains(yamlStr, "project_path:") {
		t.Error("YAML should NOT contain 'project_path:' field in entries (it should be structural)")
	}

	// Should have the project path as a map key
	if !strings.Contains(yamlStr, "/Users/dev/project-a:") {
		t.Error("YAML should contain project path as a map key under 'projects:'")
	}
}

func TestDB_LoadNewFormat_EmptyGlobalAndProjects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	path := filepath.Join(dir, "installed.yaml")
	content := `global: []
projects: {}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0 (empty global and projects)", len(entries))
	}
}
