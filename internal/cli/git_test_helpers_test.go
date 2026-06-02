package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupLocalGitRepo creates a bare git repo with an artifact.json and skill file
// at the given subPath (empty string = repo root), tagged with tagName.
// Returns the file:// URL to the bare repo.
func setupLocalGitRepo(t *testing.T, subPath, tagName string) string {
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
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %s\n%s", args, err, out)
		}
	}

	run(dir, "git", "init", "--bare", "-b", "master", bareDir)
	run(dir, "git", "clone", bareDir, workDir)

	artifactDir := workDir
	if subPath != "" {
		artifactDir = filepath.Join(workDir, subPath)
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	version := strings.TrimPrefix(tagName, "v")
	manifest := fmt.Sprintf(`{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": "git-skill", "version": %q},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]}
}`, version)
	if err := os.WriteFile(filepath.Join(artifactDir, "artifact.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "SKILL.md"), []byte("# Git Skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	run(workDir, "git", "tag", tagName)
	run(workDir, "git", "push", "origin", "HEAD", "--tags")

	p := filepath.ToSlash(bareDir)
	if len(p) > 0 && p[0] != '/' {
		p = "/" + p
	}
	return "file://" + p
}
