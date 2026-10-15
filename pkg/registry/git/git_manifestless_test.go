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

// setupLocalRepoWithoutManifest creates a bare git repo with the given files
// at subPath but NO artifact.json, tagged with tagName.
// files maps relative path -> content.
// Returns (file:// URL, commit SHA).
func setupLocalRepoWithoutManifest(t *testing.T, subPath, tagName string, files map[string]string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "bare.git")

	run := func(wd string, args ...string) string {
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
		return strings.TrimSpace(string(out))
	}

	run(dir, "git", "init", "--bare", "-b", "master", bareDir)
	run(dir, "git", "clone", bareDir, workDir)

	baseDir := workDir
	if subPath != "" {
		baseDir = filepath.Join(workDir, subPath)
		if err := os.MkdirAll(baseDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for path, content := range files {
		full := filepath.Join(baseDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	sha := run(workDir, "git", "rev-parse", "HEAD")
	run(workDir, "git", "tag", tagName)
	run(workDir, "git", "push", "origin", "HEAD", "--tags")

	return fileURL(bareDir), sha
}

// manifestlessDep builds a GitDependency for a manifestless repo with required fields.
func manifestlessDep(url, ref, path, commit string, files []string) *artifact.GitDependency {
	return &artifact.GitDependency{
		URL:        url,
		Ref:        ref,
		Path:       path,
		Commit:     commit,
		Files:      files,
		Name:       "",
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
}

// --- Inspect: manifestless ---

func TestBackend_Inspect_NoManifest_WithFiles(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/my-skill", "v1.0.0", map[string]string{
		"SKILL.md": "# My Skill",
		"lib.md":   "# Lib",
	})
	b := &Backend{}
	dep := manifestlessDep(url, "v1.0.0", "skills/my-skill", sha, []string{"SKILL.md", "lib.md"})
	m, err := b.Inspect(context.Background(), dep)
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Metadata.Name != "my-skill" {
		t.Errorf("Name = %q, want %q", m.Metadata.Name, "my-skill")
	}
	if m.Metadata.Version != sha {
		t.Errorf("Version = %q, want commit SHA %q", m.Metadata.Version, sha)
	}
	if m.Kind != "Skill" {
		t.Errorf("Kind = %q, want %q", m.Kind, "Skill")
	}
	if m.Spec.Entrypoint != "SKILL.md" {
		t.Errorf("Entrypoint = %q, want %q", m.Spec.Entrypoint, "SKILL.md")
	}
	if len(m.Spec.Files) != 2 {
		t.Errorf("Files = %v, want 2 files", m.Spec.Files)
	}
	if m.APIVersion != artifact.SupportedAPIVersion {
		t.Errorf("APIVersion = %q, want %q", m.APIVersion, artifact.SupportedAPIVersion)
	}
	if len(m.Dependencies) != 0 {
		t.Errorf("Dependencies should be nil/empty, got %v", m.Dependencies)
	}
}

func TestBackend_Inspect_NoManifest_WithoutFiles_AllFilesUnderPath(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/review", "v1.0.0", map[string]string{
		"SKILL.md":      "# Skill",
		"lib/helper.md": "# Helper",
		"lib/utils.md":  "# Utils",
	})
	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Path:       "skills/review",
		Commit:     sha,
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	m, err := b.Inspect(context.Background(), dep)
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if len(m.Spec.Files) != 3 {
		t.Errorf("Files = %v, want 3 files (all under path)", m.Spec.Files)
	}
}

func TestBackend_Inspect_NoManifest_RootPath_AllFiles(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "", "v1.0.0", map[string]string{
		"SKILL.md": "# Root Skill",
		"extra.md": "# Extra",
	})
	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Commit:     sha,
		Name:       "root-skill",
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	m, err := b.Inspect(context.Background(), dep)
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Metadata.Name != "root-skill" {
		t.Errorf("Name = %q, want %q", m.Metadata.Name, "root-skill")
	}
	if len(m.Spec.Files) != 2 {
		t.Errorf("Files = %v, want 2 files", m.Spec.Files)
	}
}

func TestBackend_Inspect_NoManifest_UsesDepName(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/review", "v1.0.0", map[string]string{
		"SKILL.md": "# Skill",
	})
	b := &Backend{}
	dep := manifestlessDep(url, "v1.0.0", "skills/review", sha, []string{"SKILL.md"})
	dep.Name = "custom-name"
	m, err := b.Inspect(context.Background(), dep)
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Metadata.Name != "custom-name" {
		t.Errorf("Name = %q, want %q", m.Metadata.Name, "custom-name")
	}
}

func TestBackend_Inspect_NoManifest_PathDoesNotExist_Fails(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "", "v1.0.0", map[string]string{
		"SKILL.md": "# Skill",
	})
	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Path:       "nonexistent/path",
		Commit:     sha,
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	_, err := b.Inspect(context.Background(), dep)
	if err == nil {
		t.Fatal("Inspect() should fail for nonexistent path")
	}
}

func TestBackend_Inspect_NoManifest_NoEntrypoint_NonSkillKind_Fails(t *testing.T) {
	// Entry point inference only covers Skill (SKILL.md); other kinds must
	// declare an entrypoint explicitly.
	url, sha := setupLocalRepoWithoutManifest(t, "prompts/p", "v1.0.0", map[string]string{
		"prompt.md": "# Prompt",
	})
	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:    url,
		Ref:    "v1.0.0",
		Path:   "prompts/p",
		Commit: sha,
		Files:  []string{"prompt.md"},
		Kind:   "Prompt",
	}
	_, err := b.Inspect(context.Background(), dep)
	if err == nil {
		t.Fatal("Inspect() should fail when entrypoint is missing and cannot be inferred")
	}
	if !strings.Contains(err.Error(), "entrypoint") {
		t.Errorf("error should mention entrypoint: %v", err)
	}
}

func TestBackend_Inspect_NoManifest_NoCommit_InfersFromRef(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/s", "v1.0.0", map[string]string{
		"SKILL.md": "# Skill",
	})
	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Path:       "skills/s",
		Files:      []string{"SKILL.md"},
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	m, err := b.Inspect(context.Background(), dep)
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Metadata.Version != sha[:7] {
		t.Errorf("Version = %q, want short SHA %q inferred from ref", m.Metadata.Version, sha[:7])
	}
}

func TestBackend_Inspect_NoManifest_NoNameNoPath_InfersFromRepo(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "", "v1.0.0", map[string]string{
		"SKILL.md": "# Skill",
	})
	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Files:      []string{"SKILL.md"},
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	m, err := b.Inspect(context.Background(), dep)
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Metadata.Name != "bare" {
		t.Errorf("Name = %q, want repo basename %q", m.Metadata.Name, "bare")
	}
	if m.Metadata.Version != sha[:7] {
		t.Errorf("Version = %q, want short SHA %q", m.Metadata.Version, sha[:7])
	}
}

func TestBackend_Inspect_NoManifest_EntrypointNotInCollectedFiles_Fails(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/s", "v1.0.0", map[string]string{
		"README.md": "# Readme",
	})
	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Path:       "skills/s",
		Commit:     sha,
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	_, err := b.Inspect(context.Background(), dep)
	if err == nil {
		t.Fatal("Inspect() should fail when entrypoint is not among collected files")
	}
	if !strings.Contains(err.Error(), "entrypoint") {
		t.Errorf("error should mention entrypoint: %v", err)
	}
}

func TestBackend_Inspect_HasManifest_IgnoresDepFields(t *testing.T) {
	url := setupLocalRepo(t, "packages/skill", "v1.0.0")
	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Path:       "packages/skill",
		Files:      []string{"different.md"},
		Name:       "override-name",
		Kind:       "Prompt",
		Entrypoint: "different.md",
	}
	m, err := b.Inspect(context.Background(), dep)
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Metadata.Name != "test-skill" {
		t.Errorf("Name = %q, want %q (from artifact.json, not dep)", m.Metadata.Name, "test-skill")
	}
	if m.Kind != "Skill" {
		t.Errorf("Kind = %q, want %q (from artifact.json)", m.Kind, "Skill")
	}
}

func TestBackend_Inspect_MalformedManifest_WithFiles_StillErrors(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "bare.git")

	run := func(wd string, args ...string) string {
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
		return strings.TrimSpace(string(out))
	}

	run(dir, "git", "init", "--bare", "-b", "master", bareDir)
	run(dir, "git", "clone", bareDir, workDir)
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	sha := run(workDir, "git", "rev-parse", "HEAD")
	run(workDir, "git", "tag", "v1.0.0")
	run(workDir, "git", "push", "origin", "HEAD", "--tags")

	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        fileURL(bareDir),
		Ref:        "v1.0.0",
		Commit:     sha,
		Files:      []string{"SKILL.md"},
		Name:       "my-skill",
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	_, err := b.Inspect(context.Background(), dep)
	if err == nil {
		t.Fatal("Inspect() should error for malformed artifact.json, NOT fall back to dep.Files")
	}
	if !strings.Contains(err.Error(), "parse artifact.json") {
		t.Errorf("error should mention parse artifact.json: %v", err)
	}
}

func TestBackend_Inspect_NoManifest_AtSubPath_ManifestAtRoot(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "bare.git")

	run := func(wd string, args ...string) string {
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
		return strings.TrimSpace(string(out))
	}

	run(dir, "git", "init", "--bare", "-b", "master", bareDir)
	run(dir, "git", "clone", bareDir, workDir)

	manifest := `{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"root","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md"]}}`
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("# Root"), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(workDir, "extras", "review")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "REVIEW.md"), []byte("# Review"), 0o644); err != nil {
		t.Fatal(err)
	}

	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	sha := run(workDir, "git", "rev-parse", "HEAD")
	run(workDir, "git", "tag", "v1.0.0")
	run(workDir, "git", "push", "origin", "HEAD", "--tags")

	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        fileURL(bareDir),
		Ref:        "v1.0.0",
		Path:       "extras/review",
		Commit:     sha,
		Files:      []string{"REVIEW.md"},
		Kind:       "Skill",
		Entrypoint: "REVIEW.md",
	}
	m, err := b.Inspect(context.Background(), dep)
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Metadata.Name != "review" {
		t.Errorf("Name = %q, want %q (derived from path)", m.Metadata.Name, "review")
	}
}

// --- Pull: manifestless ---

func TestBackend_Pull_NoManifest_WithFiles(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/my-skill", "v1.0.0", map[string]string{
		"SKILL.md": "# My Skill",
		"lib.md":   "# Lib",
	})
	b := &Backend{}
	outDir := t.TempDir()
	dep := manifestlessDep(url, "v1.0.0", "skills/my-skill", sha, []string{"SKILL.md", "lib.md"})
	err := b.Pull(context.Background(), dep, outDir)
	if err != nil {
		t.Fatalf("Pull() err = %v", err)
	}

	skillDir := filepath.Join(outDir, "my-skill")
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "My Skill") {
		t.Errorf("SKILL.md content = %q", data)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "lib.md")); err != nil {
		t.Errorf("lib.md missing: %v", err)
	}
}

func TestBackend_Pull_NoManifest_WithoutFiles_AllFiles(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/review", "v1.0.0", map[string]string{
		"SKILL.md":        "# Skill",
		"lib/helper.md":   "# Helper",
		"lib/sub/deep.md": "# Deep",
	})
	b := &Backend{}
	outDir := t.TempDir()
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Path:       "skills/review",
		Commit:     sha,
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	err := b.Pull(context.Background(), dep, outDir)
	if err != nil {
		t.Fatalf("Pull() err = %v", err)
	}

	skillDir := filepath.Join(outDir, "review")
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "lib/helper.md")); err != nil {
		t.Errorf("lib/helper.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "lib/sub/deep.md")); err != nil {
		t.Errorf("lib/sub/deep.md missing: %v", err)
	}
}

func TestBackend_Pull_NoManifest_UsesDepName(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/review", "v1.0.0", map[string]string{
		"SKILL.md": "# Skill",
	})
	b := &Backend{}
	outDir := t.TempDir()
	dep := manifestlessDep(url, "v1.0.0", "skills/review", sha, []string{"SKILL.md"})
	dep.Name = "custom-name"
	err := b.Pull(context.Background(), dep, outDir)
	if err != nil {
		t.Fatalf("Pull() err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "custom-name", "SKILL.md")); err != nil {
		t.Errorf("expected output dir named custom-name: %v", err)
	}
}

func TestBackend_Pull_NoManifest_RejectsPathTraversal(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/s", "v1.0.0", map[string]string{
		"SKILL.md": "# Skill",
	})
	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Path:       "skills/s",
		Commit:     sha,
		Files:      []string{"../../../etc/passwd"},
		Kind:       "Skill",
		Entrypoint: "../../../etc/passwd",
	}
	err := b.Pull(context.Background(), dep, t.TempDir())
	if err == nil {
		t.Fatal("Pull() should reject path traversal in dep.Files")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Errorf("error should mention unsafe: %v", err)
	}
}

func TestBackend_Pull_NoManifest_MissingFile_Fails(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/s", "v1.0.0", map[string]string{
		"SKILL.md": "# Skill",
	})
	b := &Backend{}
	dep := manifestlessDep(url, "v1.0.0", "skills/s", sha, []string{"SKILL.md", "nonexistent.md"})
	err := b.Pull(context.Background(), dep, t.TempDir())
	if err == nil {
		t.Fatal("Pull() should fail when dep.Files references a nonexistent file")
	}
	if !strings.Contains(err.Error(), "nonexistent.md") {
		t.Errorf("error should mention the missing file: %v", err)
	}
}

func TestBackend_Pull_HasManifest_IgnoresDepFiles(t *testing.T) {
	url := setupLocalRepo(t, "", "v1.0.0")
	b := &Backend{}
	outDir := t.TempDir()
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Files:      []string{"nonexistent.md"},
		Name:       "override-name",
		Kind:       "Prompt",
		Entrypoint: "nonexistent.md",
	}
	err := b.Pull(context.Background(), dep, outDir)
	if err != nil {
		t.Fatalf("Pull() err = %v (artifact.json should take precedence, ignoring dep.Files)", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "test-skill", "SKILL.md")); err != nil {
		t.Errorf("expected test-skill/SKILL.md from artifact.json: %v", err)
	}
}

func TestBackend_Pull_NoManifest_RootPath_AllFiles(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "", "v1.0.0", map[string]string{
		"SKILL.md": "# Root Skill",
		"extra.md": "# Extra",
	})
	b := &Backend{}
	outDir := t.TempDir()
	dep := &artifact.GitDependency{
		URL:        url,
		Ref:        "v1.0.0",
		Commit:     sha,
		Name:       "root-skill",
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	err := b.Pull(context.Background(), dep, outDir)
	if err != nil {
		t.Fatalf("Pull() err = %v", err)
	}
	skillDir := filepath.Join(outDir, "root-skill")
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "extra.md")); err != nil {
		t.Errorf("extra.md missing: %v", err)
	}
}

func TestBackend_Pull_NoManifest_MalformedManifest_WithFiles_StillErrors(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "bare.git")

	run := func(wd string, args ...string) string {
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
		return strings.TrimSpace(string(out))
	}

	run(dir, "git", "init", "--bare", "-b", "master", bareDir)
	run(dir, "git", "clone", bareDir, workDir)
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(workDir, "git", "add", "-A")
	run(workDir, "git", "commit", "-m", "init")
	sha := run(workDir, "git", "rev-parse", "HEAD")
	run(workDir, "git", "tag", "v1.0.0")
	run(workDir, "git", "push", "origin", "HEAD", "--tags")

	b := &Backend{}
	dep := &artifact.GitDependency{
		URL:        fileURL(bareDir),
		Ref:        "v1.0.0",
		Commit:     sha,
		Files:      []string{"SKILL.md"},
		Name:       "my-skill",
		Kind:       "Skill",
		Entrypoint: "SKILL.md",
	}
	err := b.Pull(context.Background(), dep, t.TempDir())
	if err == nil {
		t.Fatal("Pull() should error for malformed artifact.json, NOT fall back")
	}
	if !strings.Contains(err.Error(), "parse artifact.json") {
		t.Errorf("error should mention parse artifact.json: %v", err)
	}
}

func TestBackend_Pull_NoManifest_WritesSyntheticManifest(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/review", "v1.0.0", map[string]string{
		"SKILL.md": "# Review Skill",
	})
	b := &Backend{}
	outDir := t.TempDir()
	dep := manifestlessDep(url, "v1.0.0", "skills/review", sha, []string{"SKILL.md"})
	err := b.Pull(context.Background(), dep, outDir)
	if err != nil {
		t.Fatalf("Pull() err = %v", err)
	}

	syntheticPath := filepath.Join(outDir, "review", "artifact.json")
	m, err := artifact.Load(syntheticPath)
	if err != nil {
		t.Fatalf("failed to load synthetic artifact.json: %v", err)
	}
	if m.APIVersion != artifact.SupportedAPIVersion {
		t.Errorf("APIVersion = %q", m.APIVersion)
	}
	if m.Kind != "Skill" {
		t.Errorf("Kind = %q", m.Kind)
	}
	if m.Metadata.Name != "review" {
		t.Errorf("Name = %q", m.Metadata.Name)
	}
	if m.Metadata.Version != sha {
		t.Errorf("Version = %q, want %q", m.Metadata.Version, sha)
	}
	if m.Spec.Entrypoint != "SKILL.md" {
		t.Errorf("Entrypoint = %q", m.Spec.Entrypoint)
	}
	if len(m.Spec.Files) != 1 || m.Spec.Files[0] != "SKILL.md" {
		t.Errorf("Files = %v", m.Spec.Files)
	}
	if err := artifact.Validate(m); err != nil {
		t.Errorf("synthetic manifest should pass Validate(), got: %v", err)
	}
}

// --- Manifestless with inferred entrypoint and commit (top-level CLI install path) ---
// Kind comes from the CLI --kind flag; entrypoint and commit are derived when unset.

// kindOnlyDep builds a GitDependency with kind set (from --kind) but entrypoint,
// commit, files, and name unset, as produced by parsing a CLI ref like
// git:URL@ref#path plus the --kind flag.
func kindOnlyDep(url, ref, path, kind string) *artifact.GitDependency {
	return &artifact.GitDependency{URL: url, Ref: ref, Path: path, Kind: kind}
}

func TestBackend_Pull_NoManifest_KindOnly_InferredEntrypointAndCommit(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/handoff", "main", map[string]string{
		"SKILL.md":   "# Handoff",
		"notes.md":   "# Notes",
		"sub/lib.md": "# Lib",
	})
	b := &Backend{}
	outDir := t.TempDir()
	dep := kindOnlyDep(url, "main", "skills/handoff", "Skill")
	if err := b.Pull(context.Background(), dep, outDir); err != nil {
		t.Fatalf("Pull() err = %v", err)
	}

	m, err := artifact.Load(filepath.Join(outDir, "handoff", "artifact.json"))
	if err != nil {
		t.Fatalf("load synthetic artifact.json: %v", err)
	}
	if m.Kind != "Skill" {
		t.Errorf("Kind = %q, want Skill", m.Kind)
	}
	if m.Spec.Entrypoint != "SKILL.md" {
		t.Errorf("Entrypoint = %q, want SKILL.md (inferred for Skill kind)", m.Spec.Entrypoint)
	}
	if m.Metadata.Name != "handoff" {
		t.Errorf("Name = %q, want handoff (path basename)", m.Metadata.Name)
	}
	wantVersion := sha[:7]
	if m.Metadata.Version != wantVersion {
		t.Errorf("Version = %q, want short commit SHA %q", m.Metadata.Version, wantVersion)
	}
	if len(m.Spec.Files) != 3 {
		t.Errorf("Files = %v, want 3 files", m.Spec.Files)
	}
	if err := artifact.Validate(m); err != nil {
		t.Errorf("synthetic manifest should pass Validate(): %v", err)
	}
	for _, f := range []string{"SKILL.md", "notes.md", "sub/lib.md"} {
		if _, err := os.Stat(filepath.Join(outDir, "handoff", filepath.FromSlash(f))); err != nil {
			t.Errorf("file %s not extracted: %v", f, err)
		}
	}
}

func TestBackend_Inspect_NoManifest_KindOnly_InferredEntrypointAndCommit(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/handoff", "main", map[string]string{
		"SKILL.md": "# Handoff",
	})
	b := &Backend{}
	m, err := b.Inspect(context.Background(), kindOnlyDep(url, "main", "skills/handoff", "Skill"))
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Kind != "Skill" {
		t.Errorf("Kind = %q, want Skill", m.Kind)
	}
	if m.Spec.Entrypoint != "SKILL.md" {
		t.Errorf("Entrypoint = %q, want SKILL.md", m.Spec.Entrypoint)
	}
	if m.Metadata.Name != "handoff" {
		t.Errorf("Name = %q, want handoff", m.Metadata.Name)
	}
	wantVersion := sha[:7]
	if m.Metadata.Version != wantVersion {
		t.Errorf("Version = %q, want short commit SHA %q", m.Metadata.Version, wantVersion)
	}
}

func TestBackend_Pull_NoManifest_KindOnly_RootPath_NameFromRepo(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "", "main", map[string]string{
		"SKILL.md": "# Root Skill",
	})
	b := &Backend{}
	outDir := t.TempDir()
	if err := b.Pull(context.Background(), kindOnlyDep(url, "main", "", "Skill"), outDir); err != nil {
		t.Fatalf("Pull() err = %v", err)
	}
	m, err := artifact.Load(filepath.Join(outDir, "bare", "artifact.json"))
	if err != nil {
		t.Fatalf("load synthetic artifact.json: %v", err)
	}
	if m.Metadata.Name != "bare" {
		t.Errorf("Name = %q, want repo basename %q", m.Metadata.Name, "bare")
	}
	wantVersion := sha[:7]
	if m.Metadata.Version != wantVersion {
		t.Errorf("Version = %q, want short commit SHA %q", m.Metadata.Version, wantVersion)
	}
}

func TestBackend_Pull_NoManifest_NoKind_InfersSkillFromSkillMd(t *testing.T) {
	url, sha := setupLocalRepoWithoutManifest(t, "skills/handoff", "main", map[string]string{
		"SKILL.md": "# Handoff",
	})
	b := &Backend{}
	outDir := t.TempDir()
	if err := b.Pull(context.Background(), kindOnlyDep(url, "main", "skills/handoff", ""), outDir); err != nil {
		t.Fatalf("Pull() err = %v", err)
	}
	m, err := artifact.Load(filepath.Join(outDir, "handoff", "artifact.json"))
	if err != nil {
		t.Fatalf("load synthetic artifact.json: %v", err)
	}
	if m.Kind != "Skill" {
		t.Errorf("Kind = %q, want Skill (inferred from SKILL.md)", m.Kind)
	}
	if m.Metadata.Version != sha[:7] {
		t.Errorf("Version = %q, want short SHA %q", m.Metadata.Version, sha[:7])
	}
}

func TestBackend_Inspect_NoManifest_NoKind_InfersSkillFromSkillMd(t *testing.T) {
	url, _ := setupLocalRepoWithoutManifest(t, "skills/handoff", "main", map[string]string{
		"SKILL.md": "# Handoff",
	})
	b := &Backend{}
	m, err := b.Inspect(context.Background(), kindOnlyDep(url, "main", "skills/handoff", ""))
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if m.Kind != "Skill" {
		t.Errorf("Kind = %q, want Skill (inferred from SKILL.md)", m.Kind)
	}
}

func TestBackend_Pull_NoManifest_NoKind_Unrecognizable_ErrorsWithKindHint(t *testing.T) {
	url, _ := setupLocalRepoWithoutManifest(t, "docs/random", "main", map[string]string{
		"README.md": "# Not a skill",
	})
	b := &Backend{}
	err := b.Pull(context.Background(), kindOnlyDep(url, "main", "docs/random", ""), t.TempDir())
	if err == nil {
		t.Fatal("Pull() should error when kind is neither given nor inferable")
	}
	if !strings.Contains(err.Error(), "--kind") {
		t.Errorf("error should mention --kind: %v", err)
	}
}

func TestBackend_Pull_NoManifest_KindWithoutEntrypointOrSkillMd_Errors(t *testing.T) {
	url, _ := setupLocalRepoWithoutManifest(t, "docs/random", "main", map[string]string{
		"README.md": "# Not a skill",
	})
	b := &Backend{}
	err := b.Pull(context.Background(), kindOnlyDep(url, "main", "docs/random", "Skill"), t.TempDir())
	if err == nil {
		t.Fatal("Pull() should error when entrypoint cannot be inferred")
	}
	if !strings.Contains(err.Error(), "entrypoint") {
		t.Errorf("error should mention entrypoint: %v", err)
	}
}

func TestBackend_Pull_NoManifest_NestedSkillMd_NotInferred(t *testing.T) {
	// #path points at the parent of the SKILL.md directory: the collected
	// files contain "sub/SKILL.md", not "SKILL.md", so neither kind nor
	// entrypoint can be inferred. Current contract: error with --kind hint.
	url, _ := setupLocalRepoWithoutManifest(t, "skills", "main", map[string]string{
		"handoff/SKILL.md": "# Handoff",
	})
	b := &Backend{}
	err := b.Pull(context.Background(), kindOnlyDep(url, "main", "skills", ""), t.TempDir())
	if err == nil {
		t.Fatal("Pull() should error when SKILL.md is nested under the pulled path")
	}
	if !strings.Contains(err.Error(), "--kind") {
		t.Errorf("error should mention --kind: %v", err)
	}
	// Even with --kind Skill, the entrypoint cannot be inferred for a nested SKILL.md.
	err = b.Pull(context.Background(), kindOnlyDep(url, "main", "skills", "Skill"), t.TempDir())
	if err == nil {
		t.Fatal("Pull() should error when entrypoint cannot be inferred for nested SKILL.md")
	}
	if !strings.Contains(err.Error(), "entrypoint") {
		t.Errorf("error should mention entrypoint: %v", err)
	}
}
