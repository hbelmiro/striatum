package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
)

func TestInspect_WithDependencies_PrintsDepSection(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "root-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "reg.io", Repository: "skills/dep-a", Tag: "2.0.0"},
			&artifact.GitDependency{URL: "https://github.com/org/repo.git", Ref: "v3.0.0", Path: "sub"},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"inspect", "oci:" + layoutDir + ":root-skill:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Dependencies:") {
		t.Errorf("output should contain Dependencies: section\n%s", got)
	}
	if !strings.Contains(got, "reg.io/skills/dep-a:2.0.0") {
		t.Errorf("output should contain OCI dep canonical ref\n%s", got)
	}
	if !strings.Contains(got, "[oci]") {
		t.Errorf("output should contain [oci] source\n%s", got)
	}
	if !strings.Contains(got, "github.com/org/repo.git") {
		t.Errorf("output should contain Git dep URL\n%s", got)
	}
	if !strings.Contains(got, "[git]") {
		t.Errorf("output should contain [git] source\n%s", got)
	}
}

func TestInspect_NoDeps_OmitsDepsSection(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "no-deps", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"inspect", "oci:" + layoutDir + ":no-deps:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if strings.Contains(out.String(), "Dependencies:") {
		t.Errorf("output should NOT contain Dependencies: for zero-dep artifact\n%s", out.String())
	}
}

func TestInspect_InvalidRef_Errors(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"inspect", "oci:./nonexistent-layout:tag"})
	if err := root.Execute(); err == nil {
		t.Error("inspect with bad layout: expected error")
	}
}

func TestInspect_OCI_DisplaysDigest(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "digest-test", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"inspect", "oci:" + layoutDir + ":digest-test:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	got := out.String()
	digestPattern := regexp.MustCompile(`Digest:\s+sha256:[0-9a-f]{64}`)
	if !digestPattern.MatchString(got) {
		t.Errorf("output should contain Digest: sha256:<64 hex chars>\n%s", got)
	}
	lines := strings.Split(got, "\n")
	var kindIdx, digestIdx, entryIdx int
	for i, l := range lines {
		if strings.HasPrefix(l, "Kind:") {
			kindIdx = i
		}
		if strings.HasPrefix(l, "Digest:") {
			digestIdx = i
		}
		if strings.HasPrefix(l, "Entrypoint:") {
			entryIdx = i
		}
	}
	if digestIdx == 0 {
		t.Fatalf("Digest line not found in output:\n%s", got)
	}
	if kindIdx >= digestIdx || digestIdx >= entryIdx {
		t.Errorf("Digest should appear between Kind and Entrypoint; Kind=%d Digest=%d Entrypoint=%d\n%s",
			kindIdx, digestIdx, entryIdx, got)
	}
	if strings.Contains(got, "Commit:") {
		t.Errorf("OCI output should not contain Commit: line\n%s", got)
	}
}

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

	manifest := `{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": "git-skill", "version": "1.0.0"},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]}
}`
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

func TestInspect_Git_DisplaysCommit(t *testing.T) {
	repoURL := setupLocalGitRepo(t, "", "v1.0.0")
	ref := "git:" + repoURL + "@v1.0.0"

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"inspect", ref})
	if err := root.Execute(); err != nil {
		t.Fatalf("inspect git: %v", err)
	}
	got := out.String()
	commitPattern := regexp.MustCompile(`Commit:\s+[0-9a-f]{40}`)
	if !commitPattern.MatchString(got) {
		t.Errorf("output should contain Commit: <40 hex chars>\n%s", got)
	}
	if !strings.Contains(got, "Name:") || !strings.Contains(got, "git-skill") {
		t.Errorf("output should contain artifact name\n%s", got)
	}
	lines := strings.Split(got, "\n")
	var kindIdx, commitIdx, entryIdx int
	for i, l := range lines {
		if strings.HasPrefix(l, "Kind:") {
			kindIdx = i
		}
		if strings.HasPrefix(l, "Commit:") {
			commitIdx = i
		}
		if strings.HasPrefix(l, "Entrypoint:") {
			entryIdx = i
		}
	}
	if commitIdx == 0 {
		t.Fatalf("Commit line not found in output:\n%s", got)
	}
	if kindIdx >= commitIdx || commitIdx >= entryIdx {
		t.Errorf("Commit should appear between Kind and Entrypoint; Kind=%d Commit=%d Entrypoint=%d\n%s",
			kindIdx, commitIdx, entryIdx, got)
	}
	if strings.Contains(got, "Digest:") {
		t.Errorf("Git output should not contain Digest: line\n%s", got)
	}
}

func TestInspect_Git_WithSubPath(t *testing.T) {
	repoURL := setupLocalGitRepo(t, "sub/dir", "v2.0.0")
	ref := "git:" + repoURL + "@v2.0.0#sub/dir"

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"inspect", ref})
	if err := root.Execute(); err != nil {
		t.Fatalf("inspect git with subpath: %v", err)
	}
	got := out.String()
	commitPattern := regexp.MustCompile(`Commit:\s+[0-9a-f]{40}`)
	if !commitPattern.MatchString(got) {
		t.Errorf("output should contain Commit: <40 hex chars>\n%s", got)
	}
	if !strings.Contains(got, "git-skill") {
		t.Errorf("output should contain artifact name from subpath\n%s", got)
	}
}

func TestInspect_Git_InvalidRef_Errors(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"inspect", "git:https://example.com/repo.git"})
	if err := root.Execute(); err == nil {
		t.Error("inspect with invalid git ref (missing @ref): expected error")
	}
}

func TestInspect_FromOCILayoutPrintsManifest(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "inspect-cli", Version: "2.0.0", Description: "Desc"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"inspect", "oci:" + layoutDir + ":inspect-cli:2.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "inspect-cli") || !strings.Contains(got, "2.0.0") {
		t.Errorf("output %q missing name or version", got)
	}
}
