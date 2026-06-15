package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargets_CursorEmptyProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := Targets("cursor", "", "Skill")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(home, ".cursor", "skills")
	if got != want {
		t.Errorf("Targets(cursor, \"\", Skill) = %q, want %q", got, want)
	}
}

func TestTargets_ClaudeEmptyProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := Targets("claude", "", "Skill")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(home, ".claude", "skills")
	if got != want {
		t.Errorf("Targets(claude, \"\", Skill) = %q, want %q", got, want)
	}
}

func TestTargets_CursorWithProject(t *testing.T) {
	proj := t.TempDir()
	got, err := Targets("cursor", proj, "Skill")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(proj, ".cursor", "skills")
	if got != want {
		t.Errorf("Targets(cursor, proj, Skill) = %q, want %q", got, want)
	}
}

func TestTargets_ClaudeWithProject(t *testing.T) {
	proj := t.TempDir()
	got, err := Targets("claude", proj, "Skill")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(proj, ".claude", "skills")
	if got != want {
		t.Errorf("Targets(claude, proj, Skill) = %q, want %q", got, want)
	}
}

func TestTargets_PromptKind_ReturnsPromptsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := Targets("claude", "", "Prompt")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(home, ".claude", "prompts")
	if got != want {
		t.Errorf("Targets(claude, \"\", Prompt) = %q, want %q", got, want)
	}
}

func TestTargets_PromptKind_CursorWithProject(t *testing.T) {
	proj := t.TempDir()
	got, err := Targets("cursor", proj, "Prompt")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(proj, ".cursor", "prompts")
	if got != want {
		t.Errorf("Targets(cursor, proj, Prompt) = %q, want %q", got, want)
	}
}

func TestTargets_EmptyKind_ReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, err := Targets("claude", "", "")
	if err == nil {
		t.Fatal("Targets(claude, \"\", \"\") want error for empty kind")
	}
}

func TestTargets_InvalidTarget(t *testing.T) {
	_, err := Targets("all", "", "Skill")
	if err == nil {
		t.Error("Targets(all) want error")
	}
	_, err = Targets("", "", "Skill")
	if err == nil {
		t.Error("Targets(empty) want error")
	}
}

func TestTargets_WorkflowClaudeEmptyProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := Targets("claude", "", "Workflow")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(home, ".claude", "workflows")
	if got != want {
		t.Errorf("Targets(claude, \"\", Workflow) = %q, want %q", got, want)
	}
}

func TestTargets_WorkflowClaudeWithProject(t *testing.T) {
	proj := t.TempDir()
	got, err := Targets("claude", proj, "Workflow")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	want := filepath.Join(proj, ".claude", "workflows")
	if got != want {
		t.Errorf("Targets(claude, proj, Workflow) = %q, want %q", got, want)
	}
}

func TestTargets_WorkflowCursorRejected(t *testing.T) {
	_, err := Targets("cursor", "", "Workflow")
	if err == nil {
		t.Fatal("Targets(cursor, \"\", Workflow) want error, got nil")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should mention claude, got %q", err.Error())
	}
}

func TestTargets_UnsupportedKindRejected(t *testing.T) {
	_, err := Targets("claude", "", "Unknown")
	if err == nil {
		t.Fatal("Targets(claude, \"\", Unknown) want error, got nil")
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

func TestCreateWorkflowSymlink_CreatesRelativeSymlink(t *testing.T) {
	targetDir := t.TempDir()
	wfDir := filepath.Join(targetDir, "my-workflow")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "run.js"), []byte("// script"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CreateWorkflowSymlink(targetDir, "my-workflow", "run.js"); err != nil {
		t.Fatalf("CreateWorkflowSymlink: %v", err)
	}

	linkPath := filepath.Join(targetDir, "my-workflow.js")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	want := filepath.Join("my-workflow", "run.js")
	if target != want {
		t.Errorf("symlink target = %q, want %q", target, want)
	}
	if _, err := os.Stat(linkPath); err != nil {
		t.Errorf("symlink does not resolve: %v", err)
	}
}

func TestCreateWorkflowSymlink_OverwritesExistingSymlink(t *testing.T) {
	targetDir := t.TempDir()
	wfDir := filepath.Join(targetDir, "my-workflow")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "v1.js"), []byte("// v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "v2.js"), []byte("// v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CreateWorkflowSymlink(targetDir, "my-workflow", "v1.js"); err != nil {
		t.Fatalf("first CreateWorkflowSymlink: %v", err)
	}
	if err := CreateWorkflowSymlink(targetDir, "my-workflow", "v2.js"); err != nil {
		t.Fatalf("second CreateWorkflowSymlink: %v", err)
	}

	target, err := os.Readlink(filepath.Join(targetDir, "my-workflow.js"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	want := filepath.Join("my-workflow", "v2.js")
	if target != want {
		t.Errorf("symlink target = %q, want %q", target, want)
	}
}

func TestCreateWorkflowSymlink_ErrorsOnRegularFile(t *testing.T) {
	targetDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(targetDir, "my-workflow.js"), []byte("// user file"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := CreateWorkflowSymlink(targetDir, "my-workflow", "run.js")
	if err == nil {
		t.Fatal("want error when regular file blocks symlink path, got nil")
	}
}

func TestCreateWorkflowSymlink_ErrorsOnExistingDirectory(t *testing.T) {
	targetDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(targetDir, "my-workflow.js"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := CreateWorkflowSymlink(targetDir, "my-workflow", "run.js")
	if err == nil {
		t.Fatal("want error when directory blocks symlink path, got nil")
	}
}

func TestRemoveWorkflowSymlink_RemovesSymlink(t *testing.T) {
	targetDir := t.TempDir()
	linkPath := filepath.Join(targetDir, "my-workflow.js")
	if err := os.Symlink(filepath.Join("my-workflow", "run.js"), linkPath); err != nil {
		t.Fatal(err)
	}

	if err := RemoveWorkflowSymlink(targetDir, "my-workflow"); err != nil {
		t.Fatalf("RemoveWorkflowSymlink: %v", err)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink should be removed")
	}
}

func TestRemoveWorkflowSymlink_NoOpWhenMissing(t *testing.T) {
	targetDir := t.TempDir()
	if err := RemoveWorkflowSymlink(targetDir, "nonexistent"); err != nil {
		t.Fatalf("RemoveWorkflowSymlink(missing): %v", err)
	}
}

func TestRemoveWorkflowSymlink_SkipsRegularFile(t *testing.T) {
	targetDir := t.TempDir()
	filePath := filepath.Join(targetDir, "my-workflow.js")
	if err := os.WriteFile(filePath, []byte("// user file"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveWorkflowSymlink(targetDir, "my-workflow"); err != nil {
		t.Fatalf("RemoveWorkflowSymlink(regular file): %v", err)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Error("regular file should not be removed")
	}
}

func TestCreateWorkflowSymlink_EntrypointInSubdirectory(t *testing.T) {
	targetDir := t.TempDir()
	wfDir := filepath.Join(targetDir, "my-wf")
	if err := os.MkdirAll(filepath.Join(wfDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "src", "main.js"), []byte("// script"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CreateWorkflowSymlink(targetDir, "my-wf", "src/main.js"); err != nil {
		t.Fatalf("CreateWorkflowSymlink with subdir entrypoint: %v", err)
	}

	linkPath := filepath.Join(targetDir, "my-wf.js")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	want := filepath.Join("my-wf", "src", "main.js")
	if target != want {
		t.Errorf("symlink target = %q, want %q", target, want)
	}
	if _, err := os.Stat(linkPath); err != nil {
		t.Errorf("symlink does not resolve: %v", err)
	}
}

func TestCreateWorkflowSymlink_CreatesParentDirectoryIfMissing(t *testing.T) {
	home := t.TempDir()
	workflowsDir := filepath.Join(home, ".claude", "workflows")

	if err := CreateWorkflowSymlink(workflowsDir, "my-workflow", "run.js"); err != nil {
		t.Fatalf("CreateWorkflowSymlink should create parent dir, got: %v", err)
	}

	linkPath := filepath.Join(workflowsDir, "my-workflow.js")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
}
