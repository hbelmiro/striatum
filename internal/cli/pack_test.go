package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/resolver"
	ocistore "oras.land/oras-go/v2/content/oci"
)

func setupTestProject(t *testing.T, dir, name string) {
	t.Helper()
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: name, Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPack_CreatesLayoutAndPrintsMessage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	setupTestProject(t, dir, "cli-pack")

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, "Packed") {
		t.Errorf("output %q does not contain Packed", gotOut)
	}
	wantLayout := filepath.Join(dir, "build")
	if !strings.Contains(gotOut, wantLayout) {
		t.Errorf("output %q should mention layout path %q", gotOut, wantLayout)
	}
	if _, err := os.Stat(filepath.Join(wantLayout, "index.json")); err != nil {
		t.Errorf("layout index.json missing: %v", err)
	}
}

func TestPack_WithManifestFlagFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	cwd := t.TempDir()

	setupTestProject(t, projectDir, "remote-pack")
	t.Chdir(cwd)

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-f", projectDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -f from other dir: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, "Packed") {
		t.Errorf("output %q does not contain Packed", gotOut)
	}
	wantLayout := filepath.Join(projectDir, "build")
	if !strings.Contains(gotOut, wantLayout) {
		t.Errorf("output %q should mention layout path %q", gotOut, wantLayout)
	}
	idx := filepath.Join(wantLayout, "index.json")
	if _, err := os.Stat(idx); err != nil {
		t.Errorf("expected layout under project dir %s: %v", idx, err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "build", "index.json")); err == nil {
		t.Error("did not expect OCI layout under unrelated cwd")
	}
}

func TestPack_CustomOutputAbsoluteLayout(t *testing.T) {
	projectDir := t.TempDir()
	customLayout := t.TempDir()
	t.Chdir(projectDir)

	setupTestProject(t, projectDir, "custom-out")

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-o", customLayout})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -o: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, customLayout) {
		t.Errorf("output %q should mention layout path %q", gotOut, customLayout)
	}
	if _, err := os.Stat(filepath.Join(customLayout, "index.json")); err != nil {
		t.Errorf("expected index.json under custom output %s: %v", customLayout, err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "build", "index.json")); err == nil {
		t.Error("did not expect default <project>/build when -o is set")
	}
}

func TestPack_CustomOutputRelativeToCwd(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	relOut := "rel-oci-layout"
	wantLayout, err := filepath.Abs(relOut)
	if err != nil {
		t.Fatal(err)
	}

	setupTestProject(t, projectDir, "rel-out")

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-o", relOut})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -o relative: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, wantLayout) {
		t.Errorf("output %q should mention resolved layout path %q", gotOut, wantLayout)
	}
	if _, err := os.Stat(filepath.Join(wantLayout, "index.json")); err != nil {
		t.Errorf("expected index.json under resolved output %s: %v", wantLayout, err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "build", "index.json")); err == nil {
		t.Error("did not expect default <project>/build when -o is set")
	}
}

func TestPack_CustomOutputWithManifestFlagFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	cwd := t.TempDir()
	customLayout := t.TempDir()

	setupTestProject(t, projectDir, "custom-f")
	t.Chdir(cwd)

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-f", projectDir, "-o", customLayout})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -f -o: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, customLayout) {
		t.Errorf("output %q should mention layout path %q", gotOut, customLayout)
	}
	if _, err := os.Stat(filepath.Join(customLayout, "index.json")); err != nil {
		t.Errorf("expected index.json under custom output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "build", "index.json")); err == nil {
		t.Error("did not expect <project>/build when -o is set")
	}
	if _, err := os.Stat(filepath.Join(cwd, "build", "index.json")); err == nil {
		t.Error("did not expect OCI layout under unrelated cwd")
	}
}

func TestPack_NoArtifactJSON_Errors(t *testing.T) {
	t.Chdir(t.TempDir())
	root := NewRootCommand()
	root.SetArgs([]string{"pack"})
	if err := root.Execute(); err == nil {
		t.Error("pack with no artifact.json: expected error")
	}
}

func TestPack_InvalidManifest_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	root.SetArgs([]string{"pack"})
	if err := root.Execute(); err == nil {
		t.Error("pack with invalid JSON: expected error")
	}
}

func TestPack_MissingSpecFile_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "x", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "missing.md"}},
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	root.SetArgs([]string{"pack"})
	if err := root.Execute(); err == nil {
		t.Error("pack with missing spec file: expected error")
	}
}

func TestPack_CustomOutputRelativeWithManifestFlagFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	cwd := t.TempDir()

	setupTestProject(t, projectDir, "rel-f")
	t.Chdir(cwd)

	relOut := "out-layout"
	wantLayout, err := filepath.Abs(relOut)
	if err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-f", projectDir, "-o", relOut})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -f -o relative: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, wantLayout) {
		t.Errorf("output %q should mention resolved layout path %q", gotOut, wantLayout)
	}
	if _, err := os.Stat(filepath.Join(wantLayout, "index.json")); err != nil {
		t.Errorf("expected index.json under cwd-relative output %s: %v", wantLayout, err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "build", "index.json")); err == nil {
		t.Error("did not expect <project>/build when -o is set")
	}
	if _, err := os.Stat(filepath.Join(projectDir, relOut, "index.json")); err == nil {
		t.Error("did not expect layout under project dir for cwd-relative -o")
	}
	if _, err := os.Stat(filepath.Join(cwd, "build", "index.json")); err == nil {
		t.Error("did not expect OCI layout under cwd/build when -o is set")
	}
}

func TestPack_Workflow_WithPromptDep_InlinesPromptFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	// Set up a Prompt artifact in the cache
	promptCacheDir := installer.CacheDir("Prompt", "severity-rubric", "1.0.0")
	if err := os.MkdirAll(promptCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	promptManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: "severity-rubric", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "severity-rubric.md", Files: []string{"severity-rubric.md"}},
	}
	promptData, _ := json.Marshal(promptManifest)
	if err := os.WriteFile(filepath.Join(promptCacheDir, "artifact.json"), promptData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptCacheDir, "severity-rubric.md"), []byte("# Severity Rubric\nCritical > High > Medium > Low"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set up a Workflow project with a dependency on the prompt
	wfDir := t.TempDir()
	t.Chdir(wfDir)
	wfManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "thorough-review", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "review.js", Files: []string{"review.js"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{
				RegistryHost: "ghcr.io",
				Repository:   "test/severity-rubric",
				Tag:          "1.0.0",
			},
		},
	}
	wfData, _ := json.Marshal(wfManifest)
	if err := os.WriteFile(filepath.Join(wfDir, "artifact.json"), wfData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "review.js"), []byte("// workflow script"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack workflow: %v", err)
	}

	// Pull from the layout and verify dep files are present
	layoutDir := filepath.Join(wfDir, "build")
	pullDir := t.TempDir()
	store, err := ocistore.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := oci.Pull(t.Context(), store, "thorough-review:1.0.0", pullDir); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	depPath := filepath.Join(pullDir, "thorough-review", "deps", "severity-rubric", "severity-rubric.md")
	data, err := os.ReadFile(depPath)
	if err != nil {
		t.Fatalf("dep file not extracted: %v", err)
	}
	if !strings.Contains(string(data), "Severity Rubric") {
		t.Errorf("dep file content = %q, want to contain 'Severity Rubric'", string(data))
	}
}

func TestPack_Workflow_NoDeps_NormalBehavior(t *testing.T) {
	wfDir := t.TempDir()
	t.Chdir(wfDir)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "simple-wf", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "script.js", Files: []string{"script.js"}},
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(wfDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "script.js"), []byte("// simple"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack workflow without deps: %v", err)
	}
	if !strings.Contains(out.String(), "Packed") {
		t.Errorf("output %q does not contain Packed", out.String())
	}
}

func TestPack_Skill_SkipsPromptInlining(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	promptCacheDir := installer.CacheDir("Prompt", "my-prompt", "1.0.0")
	if err := os.MkdirAll(promptCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	promptManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: "my-prompt", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "prompt.md", Files: []string{"prompt.md"}},
	}
	promptData, _ := json.Marshal(promptManifest)
	if err := os.WriteFile(filepath.Join(promptCacheDir, "artifact.json"), promptData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptCacheDir, "prompt.md"), []byte("# Prompt"), 0o644); err != nil {
		t.Fatal(err)
	}

	skillDir := t.TempDir()
	t.Chdir(skillDir)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-with-dep", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{
				RegistryHost: "ghcr.io",
				Repository:   "test/my-prompt",
				Tag:          "1.0.0",
			},
		},
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack skill: %v", err)
	}
	if !strings.Contains(out.String(), "Packed") {
		t.Errorf("output %q does not contain Packed", out.String())
	}

	layoutDir := filepath.Join(skillDir, "build")
	pullDir := t.TempDir()
	store, err := ocistore.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := oci.Pull(t.Context(), store, "skill-with-dep:1.0.0", pullDir); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	depsDir := filepath.Join(pullDir, "skill-with-dep", "deps")
	if _, err := os.Stat(depsDir); !os.IsNotExist(err) {
		t.Errorf("Skill pack should not inline deps, but deps/ directory exists")
	}
}

func TestPack_Workflow_MixedDeps_OnlyInlinesPrompts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	// Set up a Prompt dep in cache
	promptCacheDir := installer.CacheDir("Prompt", "my-prompt", "1.0.0")
	if err := os.MkdirAll(promptCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	promptManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: "my-prompt", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "prompt.md", Files: []string{"prompt.md"}},
	}
	promptData, _ := json.Marshal(promptManifest)
	if err := os.WriteFile(filepath.Join(promptCacheDir, "artifact.json"), promptData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptCacheDir, "prompt.md"), []byte("# My Prompt"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set up a Skill dep in cache
	skillCacheDir := installer.CacheDir("Skill", "helper-skill", "1.0.0")
	if err := os.MkdirAll(skillCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "helper-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	skillData, _ := json.Marshal(skillManifest)
	if err := os.WriteFile(filepath.Join(skillCacheDir, "artifact.json"), skillData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillCacheDir, "SKILL.md"), []byte("# Helper"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set up a Workflow with both deps
	wfDir := t.TempDir()
	t.Chdir(wfDir)
	wfManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "mixed-wf", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "run.js", Files: []string{"run.js"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "ghcr.io", Repository: "test/my-prompt", Tag: "1.0.0"},
			&artifact.OCIDependency{RegistryHost: "ghcr.io", Repository: "test/helper-skill", Tag: "1.0.0"},
		},
	}
	wfData, _ := json.Marshal(wfManifest)
	if err := os.WriteFile(filepath.Join(wfDir, "artifact.json"), wfData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "run.js"), []byte("// mixed"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack workflow with mixed deps: %v", err)
	}

	// Pull and verify: only Prompt dep inlined, not Skill dep
	layoutDir := filepath.Join(wfDir, "build")
	pullDir := t.TempDir()
	store, err := ocistore.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := oci.Pull(t.Context(), store, "mixed-wf:1.0.0", pullDir); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Prompt dep should be inlined
	promptPath := filepath.Join(pullDir, "mixed-wf", "deps", "my-prompt", "prompt.md")
	if _, err := os.Stat(promptPath); err != nil {
		t.Errorf("Prompt dep should be inlined at deps/my-prompt/prompt.md: %v", err)
	}

	// Skill dep should NOT be inlined
	skillPath := filepath.Join(pullDir, "mixed-wf", "deps", "helper-skill")
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Errorf("Skill dep should NOT be inlined, but deps/helper-skill exists")
	}
}

func setupPromptInCache(t *testing.T, name, version, fileName, content string, writeFile bool) {
	t.Helper()
	cacheDir := installer.CacheDir("Prompt", name, version)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: name, Version: version},
		Spec:       artifact.Spec{Entrypoint: fileName, Files: []string{fileName}},
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if writeFile {
		if err := os.WriteFile(filepath.Join(cacheDir, fileName), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolvePromptDeps_AutoPulls_WhenFilesNotInCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	setupPromptInCache(t, "my-prompt", "1.0.0", "rubric.md", "", false)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "my-wf", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "run.js", Files: []string{"run.js"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "ghcr.io", Repository: "test/my-prompt", Tag: "1.0.0"},
		},
	}

	pullCalled := false
	fakePuller := func(ctx context.Context, r resolver.ResolvedArtifact) error {
		pullCalled = true
		cacheDir := installer.CacheDir("Prompt", r.Name, r.Version)
		return os.WriteFile(filepath.Join(cacheDir, "rubric.md"), []byte("# Rubric content"), 0o644)
	}

	deps, err := resolvePromptDeps(context.Background(), m, fakePuller)
	if err != nil {
		t.Fatalf("resolvePromptDeps: %v", err)
	}
	if !pullCalled {
		t.Error("puller was not called when files were missing from cache")
	}
	if len(deps) != 1 {
		t.Fatalf("len(deps) = %d, want 1", len(deps))
	}
	if deps[0].AnnotationPath != "deps/my-prompt/rubric.md" {
		t.Errorf("AnnotationPath = %q, want %q", deps[0].AnnotationPath, "deps/my-prompt/rubric.md")
	}
}

func TestResolvePromptDeps_PullFails_ReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	setupPromptInCache(t, "fail-prompt", "1.0.0", "rubric.md", "", false)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "my-wf", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "run.js", Files: []string{"run.js"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "ghcr.io", Repository: "test/fail-prompt", Tag: "1.0.0"},
		},
	}

	failPuller := func(ctx context.Context, r resolver.ResolvedArtifact) error {
		return fmt.Errorf("registry unavailable")
	}

	_, err := resolvePromptDeps(context.Background(), m, failPuller)
	if err == nil {
		t.Fatal("expected error when puller fails")
	}
	if !strings.Contains(err.Error(), "pull prompt dependency") {
		t.Errorf("error should mention 'pull prompt dependency', got: %v", err)
	}
	if !strings.Contains(err.Error(), "registry unavailable") {
		t.Errorf("error should wrap puller error, got: %v", err)
	}
}

func TestResolvePromptDeps_FilesInCache_SkipsPull(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	setupPromptInCache(t, "cached-prompt", "1.0.0", "rubric.md", "# Already here", true)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "my-wf", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "run.js", Files: []string{"run.js"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "ghcr.io", Repository: "test/cached-prompt", Tag: "1.0.0"},
		},
	}

	pullCalled := false
	fakePuller := func(ctx context.Context, r resolver.ResolvedArtifact) error {
		pullCalled = true
		return nil
	}

	deps, err := resolvePromptDeps(context.Background(), m, fakePuller)
	if err != nil {
		t.Fatalf("resolvePromptDeps: %v", err)
	}
	if pullCalled {
		t.Error("puller should NOT be called when files are already in cache")
	}
	if len(deps) != 1 {
		t.Fatalf("len(deps) = %d, want 1", len(deps))
	}
}

func TestResolvePromptDeps_PathTraversal_ReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	for _, tc := range []struct {
		name     string
		fileName string
	}{
		{"parent traversal", "../evil.txt"},
		{"absolute path", "/etc/passwd"},
		{"nested traversal", "subdir/../../evil.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			promptCacheDir := installer.CacheDir("Prompt", "bad-prompt-"+tc.name, "1.0.0")
			if err := os.MkdirAll(promptCacheDir, 0o755); err != nil {
				t.Fatal(err)
			}
			promptManifest := &artifact.Manifest{
				APIVersion: "striatum.dev/v1alpha2",
				Kind:       "Prompt",
				Metadata:   artifact.Metadata{Name: "bad-prompt-" + tc.name, Version: "1.0.0"},
				Spec:       artifact.Spec{Entrypoint: tc.fileName, Files: []string{tc.fileName}},
			}
			promptData, _ := json.Marshal(promptManifest)
			if err := os.WriteFile(filepath.Join(promptCacheDir, "artifact.json"), promptData, 0o644); err != nil {
				t.Fatal(err)
			}

			m := &artifact.Manifest{
				APIVersion: "striatum.dev/v1alpha2",
				Kind:       "Workflow",
				Metadata:   artifact.Metadata{Name: "wf-" + tc.name, Version: "1.0.0"},
				Spec:       artifact.Spec{Entrypoint: "run.js", Files: []string{"run.js"}},
				Dependencies: []artifact.Dependency{
					&artifact.OCIDependency{RegistryHost: "ghcr.io", Repository: "test/bad-prompt-" + tc.name, Tag: "1.0.0"},
				},
			}

			fakePuller := func(ctx context.Context, r resolver.ResolvedArtifact) error {
				return nil
			}

			_, err := resolvePromptDeps(context.Background(), m, fakePuller)
			if err == nil {
				t.Fatalf("expected error for path %q, got nil", tc.fileName)
			}
			if !strings.Contains(err.Error(), "invalid file path") {
				t.Errorf("error should mention 'invalid file path', got: %v", err)
			}
		})
	}
}

func TestResolvePromptDeps_UnsafeDepName_ReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	for _, tc := range []struct {
		name    string
		depName string
		version string
	}{
		{"name with dot-dot", "..evil", "1.0.0"},
		{"version with dot-dot", "ok-prompt", "..sneaky"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cacheDir := installer.CacheDir("Prompt", tc.depName, tc.version)
			if err := os.MkdirAll(cacheDir, 0o755); err != nil {
				t.Fatal(err)
			}
			promptManifest := &artifact.Manifest{
				APIVersion: "striatum.dev/v1alpha2",
				Kind:       "Prompt",
				Metadata:   artifact.Metadata{Name: tc.depName, Version: tc.version},
				Spec:       artifact.Spec{Entrypoint: "rubric.md", Files: []string{"rubric.md"}},
			}
			data, _ := json.Marshal(promptManifest)
			if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cacheDir, "rubric.md"), []byte("content"), 0o644); err != nil {
				t.Fatal(err)
			}

			m := &artifact.Manifest{
				APIVersion: "striatum.dev/v1alpha2",
				Kind:       "Workflow",
				Metadata:   artifact.Metadata{Name: "wf-" + tc.name, Version: "1.0.0"},
				Spec:       artifact.Spec{Entrypoint: "run.js", Files: []string{"run.js"}},
				Dependencies: []artifact.Dependency{
					&artifact.OCIDependency{RegistryHost: "ghcr.io", Repository: "test/" + tc.depName, Tag: tc.version},
				},
			}

			fakePuller := func(ctx context.Context, r resolver.ResolvedArtifact) error {
				return nil
			}

			_, err := resolvePromptDeps(context.Background(), m, fakePuller)
			if err == nil {
				t.Fatalf("expected error for dep %q@%q, got nil", tc.depName, tc.version)
			}
			if !strings.Contains(err.Error(), "unsafe artifact name") {
				t.Errorf("error should mention 'unsafe artifact name', got: %v", err)
			}
		})
	}
}

func TestResolvePromptDeps_Skill_SkipsResolution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "my-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "ghcr.io", Repository: "test/skill-prompt", Tag: "1.0.0"},
		},
	}

	pullCalled := false
	fakePuller := func(ctx context.Context, r resolver.ResolvedArtifact) error {
		pullCalled = true
		return nil
	}

	deps, err := resolvePromptDeps(context.Background(), m, fakePuller)
	if err != nil {
		t.Fatalf("resolvePromptDeps: %v", err)
	}
	if pullCalled {
		t.Error("puller should NOT be called for Skills during pack")
	}
	if len(deps) != 0 {
		t.Errorf("len(deps) = %d, want 0 (Skills should skip prompt dep resolution)", len(deps))
	}
}
