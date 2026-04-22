package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// setupLocalRepo creates a bare git repo with an artifact.json and skill file
// at the given subPath (empty string = repo root), tagged with tagName.
// Returns the file:// URL to the bare repo.
func setupLocalRepo(t *testing.T, subPath, tagName string) string {
	t.Helper()
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "bare.git")

	run := func(wd string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = wd
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %s\n%s", args, err, out)
		}
	}

	run(dir, "git", "init", "--bare", bareDir)
	run(dir, "git", "clone", bareDir, workDir)

	artifactDir := workDir
	if subPath != "" {
		artifactDir = filepath.Join(workDir, subPath)
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	manifest := `{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": "test-skill", "version": "1.0.0"},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]}
}`
	if err := os.WriteFile(filepath.Join(artifactDir, "artifact.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "SKILL.md"), []byte("# Test Skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	run(workDir, "git", "tag", tagName)
	run(workDir, "git", "push", "origin", "HEAD", "--tags")

	return "file://" + bareDir
}

func TestBackend_Inspect_RootPath(t *testing.T) {
	url := setupLocalRepo(t, "", "v1.0.0")
	b := &Backend{}
	m, err := b.Inspect(context.Background(), &artifact.GitDependency{
		URL: url, Ref: "v1.0.0",
	})
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Metadata.Name != "test-skill" || m.Metadata.Version != "1.0.0" {
		t.Errorf("manifest = %+v", m.Metadata)
	}
}

func TestBackend_Inspect_SubPath(t *testing.T) {
	url := setupLocalRepo(t, "packages/skill", "v2.0.0")
	b := &Backend{}
	m, err := b.Inspect(context.Background(), &artifact.GitDependency{
		URL: url, Ref: "v2.0.0", Path: "packages/skill",
	})
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Metadata.Name != "test-skill" {
		t.Errorf("name = %q", m.Metadata.Name)
	}
}

func TestBackend_Inspect_InvalidRef(t *testing.T) {
	url := setupLocalRepo(t, "", "v1.0.0")
	b := &Backend{}
	_, err := b.Inspect(context.Background(), &artifact.GitDependency{
		URL: url, Ref: "nonexistent-tag",
	})
	if err == nil {
		t.Fatal("Inspect() err = nil, want error for bad ref")
	}
}

func TestBackend_Inspect_InvalidURL(t *testing.T) {
	b := &Backend{}
	_, err := b.Inspect(context.Background(), &artifact.GitDependency{
		URL: "file:///nonexistent/repo.git", Ref: "v1",
	})
	if err == nil {
		t.Fatal("Inspect() err = nil, want error for bad URL")
	}
}

func TestBackend_Pull_RootPath(t *testing.T) {
	url := setupLocalRepo(t, "", "v1.0.0")
	b := &Backend{}
	outDir := t.TempDir()
	err := b.Pull(context.Background(), &artifact.GitDependency{
		URL: url, Ref: "v1.0.0",
	}, outDir)
	if err != nil {
		t.Fatalf("Pull() err = %v", err)
	}

	skillDir := filepath.Join(outDir, "test-skill")
	if _, err := os.Stat(filepath.Join(skillDir, "artifact.json")); err != nil {
		t.Errorf("artifact.json missing: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "Test Skill") {
		t.Errorf("SKILL.md content = %q", data)
	}
}

func TestBackend_Pull_SubPath(t *testing.T) {
	url := setupLocalRepo(t, "packages/skill", "v1.0.0")
	b := &Backend{}
	outDir := t.TempDir()
	err := b.Pull(context.Background(), &artifact.GitDependency{
		URL: url, Ref: "v1.0.0", Path: "packages/skill",
	}, outDir)
	if err != nil {
		t.Fatalf("Pull() err = %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "test-skill", "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing: %v", err)
	}
}

func TestBackend_Inspect_MissingManifestAtSubPath(t *testing.T) {
	url := setupLocalRepo(t, "", "v1.0.0")
	b := &Backend{}
	_, err := b.Inspect(context.Background(), &artifact.GitDependency{
		URL: url, Ref: "v1.0.0", Path: "wrong/path",
	})
	if err == nil {
		t.Fatal("Inspect() should fail for wrong subpath")
	}
	if !strings.Contains(err.Error(), "artifact.json not found") {
		t.Errorf("error should mention artifact.json not found: %v", err)
	}
}

func TestBackend_Inspect_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "bare.git")

	run := func(wd string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = wd
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %s\n%s", args, err, out)
		}
	}

	run(dir, "git", "init", "--bare", bareDir)
	run(dir, "git", "clone", bareDir, workDir)
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	run(workDir, "git", "tag", "v1.0.0")
	run(workDir, "git", "push", "origin", "HEAD", "--tags")

	b := &Backend{}
	_, err := b.Inspect(context.Background(), &artifact.GitDependency{
		URL: "file://" + bareDir, Ref: "v1.0.0",
	})
	if err == nil {
		t.Fatal("Inspect() should fail for invalid JSON in artifact.json")
	}
	if !strings.Contains(err.Error(), "parse artifact.json") {
		t.Errorf("error should mention parse artifact.json: %v", err)
	}
}

func TestBackend_Pull_MissingSpecFile(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "bare.git")

	run := func(wd string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = wd
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %s\n%s", args, err, out)
		}
	}

	run(dir, "git", "init", "--bare", bareDir)
	run(dir, "git", "clone", bareDir, workDir)

	manifest := `{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": "incomplete", "version": "1.0.0"},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md", "missing.md"]}
}`
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("# OK"), 0o644); err != nil {
		t.Fatal(err)
	}

	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	run(workDir, "git", "tag", "v1.0.0")
	run(workDir, "git", "push", "origin", "HEAD", "--tags")

	b := &Backend{}
	outDir := t.TempDir()
	err := b.Pull(context.Background(), &artifact.GitDependency{
		URL: "file://" + bareDir, Ref: "v1.0.0",
	}, outDir)
	if err == nil {
		t.Fatal("Pull() should fail when spec.files lists a missing file")
	}
	if !strings.Contains(err.Error(), "missing.md") {
		t.Errorf("error should mention the missing file: %v", err)
	}
}

func TestBackend_Pull_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "bare.git")

	run := func(wd string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = wd
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %s\n%s", args, err, out)
		}
	}

	run(dir, "git", "init", "--bare", bareDir)
	run(dir, "git", "clone", bareDir, workDir)

	manifest := `{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": "evil-skill", "version": "1.0.0"},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md", "../../../etc/passwd"]}
}`
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("# Evil"), 0o644); err != nil {
		t.Fatal(err)
	}

	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	run(workDir, "git", "tag", "v1.0.0")
	run(workDir, "git", "push", "origin", "HEAD", "--tags")

	repoURL := "file://" + bareDir
	b := &Backend{}
	outDir := t.TempDir()
	err := b.Pull(context.Background(), &artifact.GitDependency{
		URL: repoURL, Ref: "v1.0.0",
	}, outDir)
	if err == nil {
		t.Fatal("Pull() should reject path traversal in spec.files")
	}
	if !strings.Contains(err.Error(), "unsafe file path") {
		t.Errorf("error should mention unsafe file path, got: %v", err)
	}
}

func TestValidateFilePaths(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		wantErr bool
	}{
		{"normal files", []string{"SKILL.md", "lib/helper.py"}, false},
		{"parent traversal", []string{"../etc/passwd"}, true},
		{"absolute path", []string{"/etc/passwd"}, true},
		{"dot-dot in middle", []string{"foo/../../etc/passwd"}, true},
		{"current dir prefix", []string{"./SKILL.md"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilePaths(tt.files)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFilePaths(%v) err = %v, wantErr %v", tt.files, err, tt.wantErr)
			}
		})
	}
}
