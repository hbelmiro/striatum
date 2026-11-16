package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// setupLocalGitPromptRepo creates a bare git repo holding a Prompt artifact
// (git-prompt) with a PROMPT.md and an example file, tagged with tagName.
// Returns the file:// URL to the bare repo.
func setupLocalGitPromptRepo(t *testing.T, tagName string) string {
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

	version := strings.TrimPrefix(tagName, "v")
	manifest := fmt.Sprintf(`{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Prompt",
  "metadata": {"name": "git-prompt", "version": %q},
  "spec": {"entrypoint": "PROMPT.md", "files": ["PROMPT.md", "examples/case.md"]}
}`, version)
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "PROMPT.md"), []byte("git prompt body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "examples", "case.md"), []byte("case"), 0o644); err != nil {
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

func TestPackExport_GitDependency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	repoURL := setupLocalGitPromptRepo(t, "v1.0.0")
	root := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "git-root-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.GitDependency{URL: repoURL, Ref: "v1.0.0"},
		},
	}
	rootJSON, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: git-root-skill\n---\nSee [p](../../prompts/git-prompt/PROMPT.md).\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(project, "dist")
	if out, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatalf("pack --self-contained with git dep: %v (out: %s)", err, out)
	}
	store, titles := openExportLayout(t, layout, "git-root-skill:1.0.0")
	for _, want := range []string{
		"deps/prompts/git-prompt/PROMPT.md",
		"deps/prompts/git-prompt/examples/case.md",
		".striatum/provenance.json",
	} {
		if _, ok := titles[want]; !ok {
			t.Errorf("layers missing %q (have %v)", want, titles)
		}
	}
	rootMD := fetchLayerByTitle(t, store, titles, "SKILL.md")
	if !strings.Contains(rootMD, "](deps/prompts/git-prompt/PROMPT.md)") {
		t.Errorf("root link to git dep not rewritten: %q", rootMD)
	}

	// Provenance records the git source identity, including the resolved commit.
	provBytes := fetchLayerByTitle(t, store, titles, ".striatum/provenance.json")
	var prov struct {
		Dependencies []struct {
			Kind           string `json:"kind"`
			Name           string `json:"name"`
			ResolvedSource struct {
				Type   string `json:"type"`
				URL    string `json:"url"`
				Commit string `json:"commit"`
			} `json:"resolvedSource"`
			ImagePaths []string `json:"imagePaths"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(provBytes), &prov); err != nil {
		t.Fatal(err)
	}
	if len(prov.Dependencies) != 1 {
		t.Fatalf("provenance deps = %d, want 1", len(prov.Dependencies))
	}
	d := prov.Dependencies[0]
	if d.ResolvedSource.Type != "git" || !strings.Contains(d.ResolvedSource.URL, "file://") {
		t.Errorf("resolved source = %+v, want git file:// URL", d.ResolvedSource)
	}
	if len(d.ResolvedSource.Commit) != 40 {
		t.Errorf("resolved commit = %q, want a 40-hex SHA", d.ResolvedSource.Commit)
	}
	if len(d.ImagePaths) != 2 {
		t.Errorf("imagePaths = %v, want 2 entries", d.ImagePaths)
	}

	// Install from the layout with the git source's cache evicted.
	if err := os.RemoveAll(filepath.Join(home, "cache", "Prompt")); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "install", "--target", "claude", "oci:"+layout+":git-root-skill:1.0.0"); err != nil {
		t.Fatalf("install exported with git dep: %v", err)
	}
	targetDir := filepath.Join(home, ".claude", "skills", "git-root-skill")
	if _, err := os.Stat(filepath.Join(targetDir, "deps", "prompts", "git-prompt", "examples", "case.md")); err != nil {
		t.Errorf("installed git dep example missing: %v", err)
	}
}

// setupLocalGitRepoWithSymlink creates a repo whose artifact declares a
// symlinked file; extraction must reject it rather than materialize the
// link-target string as file content.
func setupLocalGitRepoWithSymlink(t *testing.T, tagName string) string {
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

	version := strings.TrimPrefix(tagName, "v")
	manifest := fmt.Sprintf(`{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": "git-symlink", "version": %q},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md", "linked.md"]}
}`, version)
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("---\nname: git-symlink\n---\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("SKILL.md", filepath.Join(workDir, "linked.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
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

func TestPrepare_SelfContainedRejectsGitSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	repoURL := setupLocalGitRepoWithSymlink(t, "v1.0.0")
	root := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "git-symlink-root", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.GitDependency{URL: repoURL, Ref: "v1.0.0"},
		},
	}
	rootJSON, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: git-symlink-root\n---\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(project, "dist")
	_, err := runCLI(t, "pack", "--self-contained", "-o", layout)
	if err == nil {
		t.Fatal("expected error for symlinked dependency file")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v, want non-regular-file rejection", err)
	}
	if _, statErr := os.Stat(filepath.Join(layout, "index.json")); !os.IsNotExist(statErr) {
		t.Errorf("partial layout written: %v", statErr)
	}
}

// A Git dependency declaring an executable file must export it (go-git's
// IsRegular excludes the executable mode).
func TestPrepare_SelfContainedAcceptsGitExecutable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

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
	manifest := `{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": "git-exec", "version": "1.0.0"},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md", "run.sh"]}
}`
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("---\nname: git-exec\n---\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	run(workDir, "git", "tag", "v1.0.0")
	run(workDir, "git", "push", "origin", "HEAD", "--tags")
	bare := filepath.ToSlash(bareDir)
	if bare[0] != '/' {
		bare = "/" + bare
	}

	root := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "git-exec-root", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.GitDependency{URL: "file://" + bare, Ref: "v1.0.0"},
		},
	}
	rootJSON, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: git-exec-root\n---\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(project, "dist")
	if out, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatalf("pack with executable git dep: %v (out: %s)", err, out)
	}
	_, titles := openExportLayout(t, layout, "git-exec-root:1.0.0")
	if _, ok := titles["deps/skills/git-exec/run.sh"]; !ok {
		t.Errorf("executable dep file missing from image (have %v)", titles)
	}
}
