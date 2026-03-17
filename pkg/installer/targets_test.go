package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTargets_CursorEmptyProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := Targets("cursor", "")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(home, ".cursor", "skills")
	if got != want {
		t.Errorf("Targets(cursor, \"\") = %q, want %q", got, want)
	}
}

func TestTargets_ClaudeEmptyProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := Targets("claude", "")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(home, ".claude", "skills")
	if got != want {
		t.Errorf("Targets(claude, \"\") = %q, want %q", got, want)
	}
}

func TestTargets_CursorWithProject(t *testing.T) {
	proj := t.TempDir()
	got, err := Targets("cursor", proj)
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(proj, ".cursor", "skills")
	if got != want {
		t.Errorf("Targets(cursor, proj) = %q, want %q", got, want)
	}
}

func TestTargets_ClaudeWithProject(t *testing.T) {
	proj := t.TempDir()
	got, err := Targets("claude", proj)
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(proj, ".claude", "skills")
	if got != want {
		t.Errorf("Targets(claude, proj) = %q, want %q", got, want)
	}
}

func TestTargets_InvalidTarget(t *testing.T) {
	_, err := Targets("all", "")
	if err == nil {
		t.Error("Targets(all) want error")
	}
	_, err = Targets("", "")
	if err == nil {
		t.Error("Targets(empty) want error")
	}
}

func TestInstallToTarget_CreatesCopy(t *testing.T) {
	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetDir := t.TempDir()
	err := InstallToTarget(cacheDir, targetDir, "my-skill")
	if err != nil {
		t.Fatalf("InstallToTarget: %v", err)
	}
	dest := filepath.Join(targetDir, "my-skill")
	if _, err := os.Stat(filepath.Join(dest, "artifact.json")); err != nil {
		t.Errorf("artifact.json not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not copied: %v", err)
	}
}

func TestInstallToTarget_OverwritesExisting(t *testing.T) {
	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte(`{"version":"2.0.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	targetDir := t.TempDir()
	existing := filepath.Join(targetDir, "my-skill")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existing, "old"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := InstallToTarget(cacheDir, targetDir, "my-skill")
	if err != nil {
		t.Fatalf("InstallToTarget: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "my-skill", "old")); err == nil {
		t.Error("old file should be gone after overwrite")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "my-skill", "artifact.json")); err != nil {
		t.Errorf("artifact.json not present: %v", err)
	}
}

func TestRemoveFromTarget_RemovesDir(t *testing.T) {
	targetDir := t.TempDir()
	skillDir := filepath.Join(targetDir, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := RemoveFromTarget(targetDir, "my-skill")
	if err != nil {
		t.Fatalf("RemoveFromTarget: %v", err)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Errorf("skill dir should be removed: %v", err)
	}
}

func TestRemoveFromTarget_NoOpWhenMissing(t *testing.T) {
	targetDir := t.TempDir()
	err := RemoveFromTarget(targetDir, "nonexistent")
	if err != nil {
		t.Fatalf("RemoveFromTarget(missing): %v", err)
	}
}

func TestRemoveFromTarget_ErrorWhenNotDir(t *testing.T) {
	targetDir := t.TempDir()
	filePath := filepath.Join(targetDir, "file-skill")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := RemoveFromTarget(targetDir, "file-skill")
	if err == nil {
		t.Error("RemoveFromTarget(file) want error")
	}
}
