package installer

import (
	"os"
	"path/filepath"
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
	content := `entries:
  - skill: foo
    version: "1.0.0"
    registry: localhost:5000/skills
    target: cursor
    project_path: ""
    installed_with: ""
    status: ok
    last_error: null
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
