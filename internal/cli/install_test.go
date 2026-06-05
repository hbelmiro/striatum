package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/resolver"
)

func TestInstall_MissingTargetErrors(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "localhost:5000/skills/foo:1.0.0"})
	err := root.Execute()
	if err == nil {
		t.Error("install without --target: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "target") {
		t.Errorf("error should mention target: %v", err)
	}
}

func TestInstall_InvalidTargetErrors(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "all", "localhost:5000/skills/foo:1.0.0"})
	err := root.Execute()
	if err == nil {
		t.Error("install --target all: expected error")
	}
}

func TestInstall_HappyPathNoDeps(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "install-test", Version: "1.0.0"},
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
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "oci:" + layoutDir + ":install-test:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("output %q", out.String())
	}
	cursorSkills := filepath.Join(home, ".cursor", "skills", "install-test")
	if _, err := os.Stat(filepath.Join(cursorSkills, "artifact.json")); err != nil {
		t.Errorf("artifact not installed: %v", err)
	}
}

func TestRefToCacheCandidate(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		wantName  string
		wantVer   string
		wantOK    bool
	}{
		{"registry ref", "localhost:5000/skills/foo:1.0.0", "foo", "1.0.0", true},
		{"registry ref path", "host/a/b/c:2.0.0", "c", "2.0.0", true},
		{"dep ref (registry/name:version)", "example-skill/example-helper-a:1.0.0", "example-helper-a", "1.0.0", true},
		{"oci ref name:version", "oci:/path/layout:my-skill:1.0.0", "my-skill", "1.0.0", true},
		{"oci ref tag only", "oci:/path:1.0.0", "", "", false},
		{"oci ref empty name", "oci:/path: :1.0.0", "", "", false},
		{"oci ref empty version", "oci:/path:foo: ", "", "", false},
		{"invalid no colon", "no-colon-at-all", "", "", false},
		{"git ref rejected", "git:https://github.com/org/repo@v1.0.0", "", "", false},
		{"git ref with path rejected", "git:https://github.com/org/repo@main#skills/foo", "", "", false},
		{"registry ref with digest", "localhost:5000/skills/foo:1.0.0@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "foo", "1.0.0", true},
		{"registry ref deep path with digest", "host/a/b/c:2.0.0@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", "c", "2.0.0", true},
		{"git ref with commit rejected", "git:https://github.com/org/repo@main!abcdef0123456789abcdef0123456789abcdef01", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVer, gotOK := refToCacheCandidate(tt.reference)
			if gotOK != tt.wantOK || gotName != tt.wantName || gotVer != tt.wantVer {
				t.Errorf("refToCacheCandidate(%q) = %q, %q, %v; want %q, %q, %v",
					tt.reference, gotName, gotVer, gotOK, tt.wantName, tt.wantVer, tt.wantOK)
			}
		})
	}
}

func TestAtomicReplaceCacheDir_Success(t *testing.T) {
	baseDir := t.TempDir()
	created := filepath.Join(baseDir, "source")
	cacheDir := filepath.Join(baseDir, "dest")

	if err := os.Mkdir(created, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(created, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := atomicReplaceCacheDir(created, cacheDir); err != nil {
		t.Fatalf("atomicReplaceCacheDir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cacheDir, "file.txt")); err != nil {
		t.Errorf("file should exist in dest: %v", err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Error("source should no longer exist after rename")
	}
}

func TestAtomicReplaceCacheDir_OverwritesExisting(t *testing.T) {
	baseDir := t.TempDir()
	created := filepath.Join(baseDir, "source")
	cacheDir := filepath.Join(baseDir, "dest")

	if err := os.Mkdir(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(created, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(created, "new.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := atomicReplaceCacheDir(created, cacheDir); err != nil {
		t.Fatalf("atomicReplaceCacheDir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cacheDir, "new.txt")); err != nil {
		t.Errorf("new file should exist in dest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "old.txt")); !os.IsNotExist(err) {
		t.Error("old file should not exist after replace")
	}
}

func TestAtomicReplaceCacheDir_SourceMissing_ClearError(t *testing.T) {
	baseDir := t.TempDir()
	created := filepath.Join(baseDir, "nonexistent-source")
	cacheDir := filepath.Join(baseDir, "dest")

	err := atomicReplaceCacheDir(created, cacheDir)
	if err == nil {
		t.Fatal("atomicReplaceCacheDir with missing source: expected error")
	}
	if !strings.Contains(err.Error(), "nonexistent-source") {
		t.Errorf("error should mention the missing source path: %v", err)
	}
}

func TestPullDependency_NilDep(t *testing.T) {
	err := pullDependency(context.Background(), nil, t.TempDir())
	if err == nil {
		t.Fatal("pullDependency(nil) should error")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil: %v", err)
	}
}

func TestPullDependency_UnsupportedType(t *testing.T) {
	err := pullDependency(context.Background(), &customDepType{}, t.TempDir())
	if err == nil {
		t.Fatal("pullDependency(unsupported) should error")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error should mention unsupported: %v", err)
	}
}

type customDepType struct{}

func (d *customDepType) Source() string       { return "custom" }
func (d *customDepType) CanonicalRef() string { return "custom:x" }
func (d *customDepType) Validate() error      { return nil }

func TestInstall_FromCache_WhenRefMapsToCachedSkill_SucceedsWithoutInspect(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	cacheDir := installer.CacheDir("foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "foo", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("# foo"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "foo:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install from cache: %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("output %q", out.String())
	}
	cursorSkills := filepath.Join(home, ".cursor", "skills", "foo")
	if _, err := os.Stat(filepath.Join(cursorSkills, "artifact.json")); err != nil {
		t.Errorf("artifact not installed: %v", err)
	}
}

func TestMergeInstalledWith(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		root     string
		want     string
	}{
		{"empty existing", "", "skill-a", "skill-a"},
		{"add new root", "skill-a", "skill-b", "skill-a skill-b"},
		{"duplicate root", "skill-a skill-b", "skill-a", "skill-a skill-b"},
		{"third root", "skill-a skill-b", "skill-c", "skill-a skill-b skill-c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeInstalledWith(tt.existing, tt.root)
			if got != tt.want {
				t.Errorf("mergeInstalledWith(%q, %q) = %q, want %q", tt.existing, tt.root, got, tt.want)
			}
		})
	}
}

func TestInstall_ShortRefNotInCache_Errors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "not-cached:1.0.0"})
	err := root.Execute()
	if err == nil {
		t.Fatal("install short ref not in cache: expected error")
	}
	if !strings.Contains(err.Error(), "cache-only") {
		t.Errorf("error should mention cache-only: %v", err)
	}
}

func TestInstall_ConflictWithoutForce_Errors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	for _, ent := range []struct{ name, version string }{
		{"conflict-skill", "1.0.0"},
		{"conflict-skill", "2.0.0"},
	} {
		cacheDir := installer.CacheDir(ent.name, ent.version)
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		m := &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       "Skill",
			Metadata:   artifact.Metadata{Name: ent.name, Version: ent.version},
			Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Install v1 first
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "conflict-skill:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first install: %v", err)
	}

	// Try to install v2 without --force
	root2 := NewRootCommand()
	root2.SetArgs([]string{"skill", "install", "--target", "cursor", "conflict-skill:2.0.0"})
	err := root2.Execute()
	if err == nil {
		t.Fatal("install conflicting version without --force: expected error")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("error should mention conflicts: %v", err)
	}
}

func TestInstall_ConflictWithForce_Succeeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	for _, ent := range []struct{ name, version string }{
		{"force-skill", "1.0.0"},
		{"force-skill", "2.0.0"},
	} {
		cacheDir := installer.CacheDir(ent.name, ent.version)
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		m := &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       "Skill",
			Metadata:   artifact.Metadata{Name: ent.name, Version: ent.version},
			Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "force-skill:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first install: %v", err)
	}

	out := &strings.Builder{}
	root2 := NewRootCommand()
	root2.SetOut(out)
	root2.SetArgs([]string{"skill", "install", "--target", "cursor", "--force", "force-skill:2.0.0"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("install with --force: %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("output %q missing Installed", out.String())
	}
}

func TestInstall_ReinstallAll_EmptyRegistry_Errors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	targetDir, _ := installer.Targets("cursor", "")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installer.SaveInstalled([]installer.InstalledEntry{
		{Skill: "orphan", Version: "1.0.0", Registry: "", Target: "cursor", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--reinstall-all"})
	err := root.Execute()
	if err == nil {
		t.Fatal("reinstall-all with empty registry: expected error")
	}
	if !strings.Contains(err.Error(), "no source ref stored") {
		t.Errorf("error should mention no source ref: %v", err)
	}

	entries, _ := installer.LoadInstalled()
	for _, e := range entries {
		if e.Skill == "orphan" && e.Status != "error" {
			t.Errorf("orphan entry should be marked error, got %q", e.Status)
		}
	}
}

func TestInstall_OCI_DigestCacheHit_SkipsPull(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "digest-cache-hit", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "oci:" + layoutDir + ":digest-cache-hit:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first install: %v", err)
	}

	cacheDir := installer.CacheDir("digest-cache-hit", "1.0.0")
	digestPath := filepath.Join(cacheDir, ".oci-digest")
	digestBytes, err := os.ReadFile(digestPath)
	if err != nil {
		t.Fatalf("read .oci-digest: %v", err)
	}
	digest := strings.TrimSpace(string(digestBytes))
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("digest %q should start with sha256:", digest)
	}

	out := &strings.Builder{}
	root2 := NewRootCommand()
	root2.SetOut(out)
	root2.SetArgs([]string{"skill", "install", "--target", "cursor", "oci:" + layoutDir + ":digest-cache-hit:1.0.0"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("second install (cache hit): %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("output %q missing Installed", out.String())
	}
}

func TestInstall_OCI_DigestMismatch_Repulls(t *testing.T) {
	layoutDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifest1 := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "digest-mismatch", Version: "1.0.0", Description: "v1"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}

	baseDir1 := t.TempDir()
	data1, err := json.Marshal(manifest1)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir1, "artifact.json"), data1, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir1, "SKILL.md"), []byte("# v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest1, baseDir1, layoutDir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "oci:" + layoutDir + ":digest-mismatch:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first install: %v", err)
	}

	manifest2 := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "digest-mismatch", Version: "1.0.0", Description: "v2"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	baseDir2 := t.TempDir()
	data2, err := json.Marshal(manifest2)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir2, "artifact.json"), data2, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir2, "SKILL.md"), []byte("# v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest2, baseDir2, layoutDir); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root2 := NewRootCommand()
	root2.SetOut(out)
	root2.SetArgs([]string{"skill", "install", "--target", "cursor", "oci:" + layoutDir + ":digest-mismatch:1.0.0"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("second install (digest mismatch): %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("output %q missing Installed", out.String())
	}

	installedSkill := filepath.Join(home, ".cursor", "skills", "digest-mismatch", "SKILL.md")
	content, err := os.ReadFile(installedSkill)
	if err != nil {
		t.Fatalf("read installed SKILL.md: %v", err)
	}
	if got := string(content); got != "# v2" {
		t.Errorf("installed content = %q, want \"# v2\"", got)
	}
}

func TestBuildRequired_FiltersToCurrentScope(t *testing.T) {
	entries := []installer.InstalledEntry{
		{Skill: "skill-a", Version: "1.0.0", Target: "cursor", ProjectPath: ""},
		{Skill: "skill-a", Version: "2.0.0", Target: "cursor", ProjectPath: "/proj"},
		{Skill: "skill-b", Version: "1.0.0", Target: "cursor", ProjectPath: ""},
	}

	// Global scope (ProjectPath = "")
	got := buildRequired(entries, "")
	if got["skill-a"] != "1.0.0" {
		t.Errorf("global scope: skill-a = %q, want 1.0.0", got["skill-a"])
	}
	if got["skill-b"] != "1.0.0" {
		t.Errorf("global scope: skill-b = %q, want 1.0.0", got["skill-b"])
	}
	if len(got) != 2 {
		t.Errorf("global scope: len(required) = %d, want 2", len(got))
	}
}

func TestBuildRequired_ProjectScope(t *testing.T) {
	entries := []installer.InstalledEntry{
		{Skill: "skill-a", Version: "1.0.0", Target: "cursor", ProjectPath: ""},
		{Skill: "skill-a", Version: "2.0.0", Target: "cursor", ProjectPath: "/proj/a"},
		{Skill: "skill-b", Version: "1.0.0", Target: "cursor", ProjectPath: "/proj/a"},
		{Skill: "skill-c", Version: "1.0.0", Target: "cursor", ProjectPath: "/proj/b"},
	}

	// Project scope /proj/a
	got := buildRequired(entries, "/proj/a")
	if got["skill-a"] != "2.0.0" {
		t.Errorf("project /proj/a: skill-a = %q, want 2.0.0", got["skill-a"])
	}
	if got["skill-b"] != "1.0.0" {
		t.Errorf("project /proj/a: skill-b = %q, want 1.0.0", got["skill-b"])
	}
	if _, hasC := got["skill-c"]; hasC {
		t.Errorf("project /proj/a: should NOT have skill-c (it's in /proj/b)")
	}
	if len(got) != 2 {
		t.Errorf("project /proj/a: len(required) = %d, want 2", len(got))
	}
}

func TestBuildRequired_EmptyEntries(t *testing.T) {
	got := buildRequired(nil, "")
	if len(got) != 0 {
		t.Errorf("buildRequired(nil) = %v, want empty map", got)
	}

	got2 := buildRequired([]installer.InstalledEntry{}, "/proj")
	if len(got2) != 0 {
		t.Errorf("buildRequired(empty slice) = %v, want empty map", got2)
	}
}

func TestInstall_CrossScopeNoConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	// Setup: cache skill-a@1.0.0 and skill-a@2.0.0
	for _, ent := range []struct{ name, version string }{
		{"skill-a", "1.0.0"},
		{"skill-a", "2.0.0"},
	} {
		cacheDir := installer.CacheDir(ent.name, ent.version)
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		m := &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       "Skill",
			Metadata:   artifact.Metadata{Name: ent.name, Version: ent.version},
			Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Install skill-a@1.0.0 globally
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "skill-a:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install global: %v", err)
	}

	// Setup project directory
	projectDir := t.TempDir()

	// Install skill-a@2.0.0 for project (different scope)
	// This should succeed WITHOUT --force because it's a different scope
	root2 := NewRootCommand()
	root2.SetArgs([]string{"skill", "install", "--target", "cursor", "--project", projectDir, "skill-a:2.0.0"})
	err := root2.Execute()
	if err != nil {
		t.Fatalf("install project (cross-scope): expected success, got error: %v", err)
	}

	// Verify both entries exist in DB
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}

	var globalEntry, projectEntry *installer.InstalledEntry
	for i := range entries {
		e := &entries[i]
		if e.Skill == "skill-a" && e.ProjectPath == "" {
			globalEntry = e
		} else if e.Skill == "skill-a" && e.ProjectPath == projectDir {
			projectEntry = e
		}
	}

	if globalEntry == nil {
		t.Error("global entry for skill-a not found")
	} else if globalEntry.Version != "1.0.0" {
		t.Errorf("global entry version = %q, want 1.0.0", globalEntry.Version)
	}

	if projectEntry == nil {
		t.Error("project entry for skill-a not found")
	} else if projectEntry.Version != "2.0.0" {
		t.Errorf("project entry version = %q, want 2.0.0", projectEntry.Version)
	}
}

func TestInstall_SameScopeConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	// Setup: cache skill-a@1.0.0 and skill-a@2.0.0
	for _, ent := range []struct{ name, version string }{
		{"skill-a", "1.0.0"},
		{"skill-a", "2.0.0"},
	} {
		cacheDir := installer.CacheDir(ent.name, ent.version)
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		m := &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       "Skill",
			Metadata:   artifact.Metadata{Name: ent.name, Version: ent.version},
			Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Install skill-a@1.0.0 globally
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "skill-a:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install global v1.0.0: %v", err)
	}

	// Try to install skill-a@2.0.0 globally (same scope)
	// This should FAIL without --force
	root2 := NewRootCommand()
	root2.SetArgs([]string{"skill", "install", "--target", "cursor", "skill-a:2.0.0"})
	err := root2.Execute()
	if err == nil {
		t.Fatal("install same scope different version: expected error without --force")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("error should mention conflicts: %v", err)
	}
}

// --- Local directory install tests ---

func writeLocalSkill(t *testing.T, dir, name, version, entrypoint string, files map[string]string) *artifact.Manifest {
	t.Helper()
	fileNames := make([]string, 0, len(files))
	for f := range files {
		fileNames = append(fileNames, f)
	}
	sort.Strings(fileNames)
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: name, Version: version},
		Spec:       artifact.Spec{Entrypoint: entrypoint, Files: fileNames},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for f, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return manifest
}

func TestInstall_LocalDir_HappyPath(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	writeLocalSkill(t, skillDir, "local-skill", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "# Local Skill",
	})

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("install from local dir: %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("expected Installed in output, got %q", out.String())
	}
	installed := filepath.Join(home, ".cursor", "skills", "local-skill")
	if _, err := os.Stat(filepath.Join(installed, "artifact.json")); err != nil {
		t.Errorf("artifact.json not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installed, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not installed: %v", err)
	}
}

func TestInstall_LocalDir_DotReference(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	t.Chdir(skillDir)

	writeLocalSkill(t, skillDir, "dot-skill", "0.1.0", "SKILL.md", map[string]string{
		"SKILL.md": "# Dot",
	})

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "claude", "."})
	if err := root.Execute(); err != nil {
		t.Fatalf("install from '.': %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("expected Installed in output, got %q", out.String())
	}
	installed := filepath.Join(home, ".claude", "skills", "dot-skill")
	if _, err := os.Stat(filepath.Join(installed, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not installed: %v", err)
	}
}

func TestInstall_LocalDir_RejectsPromptKind(t *testing.T) {
	promptDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: "severity-rubric", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "severity-rubric.md", Files: []string{"severity-rubric.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptDir, "severity-rubric.md"), []byte("# Severity Rubric"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", promptDir})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error when installing a Prompt artifact")
	}
	if !strings.Contains(err.Error(), "prompt") || !strings.Contains(err.Error(), "cannot be installed") {
		t.Errorf("error should explain Prompt cannot be installed, got: %v", err)
	}
}

func TestInstall_OCI_RejectsPromptKind(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: "oci-prompt", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "rubric.md", Files: []string{"rubric.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "rubric.md"), []byte("# Rubric"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "oci:" + layoutDir + ":oci-prompt:1.0.0"})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error when installing a Prompt artifact via OCI")
	}
	if !strings.Contains(err.Error(), "prompt") || !strings.Contains(err.Error(), "cannot be installed") {
		t.Errorf("error should explain Prompt cannot be installed, got: %v", err)
	}
}

func TestInstall_LocalDir_InvalidManifest(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	bad := &artifact.Manifest{
		APIVersion: "wrong/v1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "bad", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid manifest")
	}
	if !strings.Contains(err.Error(), "apiVersion") {
		t.Errorf("error should mention apiVersion, got: %v", err)
	}
}

func TestInstall_LocalDir_MissingSpecFile(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "missing-file", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "missing.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error for missing spec file")
	}
	if !strings.Contains(err.Error(), "missing.md") {
		t.Errorf("error should mention missing file, got: %v", err)
	}
}

func TestInstall_LocalDir_CacheCorrectness(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	writeLocalSkill(t, skillDir, "cache-test", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "# Cache Test",
	})
	if err := os.WriteFile(filepath.Join(skillDir, "README.md"), []byte("# Ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".git", "config"), []byte("[core]"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}

	cacheDir := installer.CacheDir("cache-test", "1.0.0")
	if _, err := os.Stat(filepath.Join(cacheDir, "artifact.json")); err != nil {
		t.Errorf("artifact.json missing from cache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing from cache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "README.md")); !os.IsNotExist(err) {
		t.Errorf("README.md should not be in cache (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, ".git")); !os.IsNotExist(err) {
		t.Errorf(".git should not be in cache (err=%v)", err)
	}
}

func TestInstall_LocalDir_DBEntry(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	writeLocalSkill(t, skillDir, "db-test", "2.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "# DB",
	})

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"skill", "install", "--target", "claude", skillDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatalf("load installed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Skill != "db-test" {
		t.Errorf("Skill = %q, want db-test", e.Skill)
	}
	if e.Version != "2.0.0" {
		t.Errorf("Version = %q, want 2.0.0", e.Version)
	}
	if e.Target != "claude" {
		t.Errorf("Target = %q, want claude", e.Target)
	}
	if e.Registry != "" {
		t.Errorf("Registry = %q, want empty for local install", e.Registry)
	}
	if e.InstalledWith != "" {
		t.Errorf("InstalledWith = %q, want empty (root install)", e.InstalledWith)
	}
	if e.Status != "ok" {
		t.Errorf("Status = %q, want ok", e.Status)
	}
}

func TestInstall_LocalDir_MultipleFiles(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	writeLocalSkill(t, skillDir, "multi-file", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md":          "# Main",
		"lib/helper.md":     "# Helper",
		"prompts/system.md": "# System Prompt",
	})

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}

	cacheDir := installer.CacheDir("multi-file", "1.0.0")
	for _, f := range []string{"SKILL.md", "lib/helper.md", "prompts/system.md"} {
		if _, err := os.Stat(filepath.Join(cacheDir, filepath.FromSlash(f))); err != nil {
			t.Errorf("%s missing from cache: %v", f, err)
		}
	}

	targetDir := filepath.Join(home, ".cursor", "skills", "multi-file")
	for _, f := range []string{"SKILL.md", "lib/helper.md", "prompts/system.md"} {
		if _, err := os.Stat(filepath.Join(targetDir, filepath.FromSlash(f))); err != nil {
			t.Errorf("%s missing from target: %v", f, err)
		}
	}
}

func TestInstall_LocalDir_ProjectPath(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	writeLocalSkill(t, skillDir, "proj-skill", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "# Project",
	})

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "--project", projectDir, skillDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}

	installed := filepath.Join(projectDir, ".cursor", "skills", "proj-skill")
	if _, err := os.Stat(filepath.Join(installed, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not installed to project path: %v", err)
	}
}

func TestInstall_LocalDir_AlwaysCopiesFresh(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	writeLocalSkill(t, skillDir, "fresh-test", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "# Version 1",
	})

	root := NewRootCommand()
	root.SetOut(&strings.Builder{})
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("first install: %v", err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Version 2"), 0o644); err != nil {
		t.Fatal(err)
	}

	root2 := NewRootCommand()
	root2.SetOut(&strings.Builder{})
	root2.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	if err := root2.Execute(); err != nil {
		t.Fatalf("second install: %v", err)
	}

	targetFile := filepath.Join(home, ".cursor", "skills", "fresh-test", "SKILL.md")
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read installed file: %v", err)
	}
	if !strings.Contains(string(data), "Version 2") {
		t.Errorf("installed file should have updated content, got: %s", data)
	}
}

func TestValidateResolvedPaths(t *testing.T) {
	tests := []struct {
		name    string
		arts    []resolver.ResolvedArtifact
		wantErr string
	}{
		{"safe", []resolver.ResolvedArtifact{{Name: "my-skill", Version: "1.0.0"}}, ""},
		{"slash in name", []resolver.ResolvedArtifact{{Name: "a/b", Version: "1.0.0"}}, "unsafe"},
		{"backslash in version", []resolver.ResolvedArtifact{{Name: "ok", Version: "1\\2"}}, "unsafe"},
		{"dotdot in name", []resolver.ResolvedArtifact{{Name: "..", Version: "1.0.0"}}, "unsafe"},
		{"dotdot in version", []resolver.ResolvedArtifact{{Name: "ok", Version: "../../x"}}, "unsafe"},
		{"second artifact unsafe", []resolver.ResolvedArtifact{
			{Name: "safe", Version: "1.0.0"},
			{Name: "../evil", Version: "1.0.0"},
		}, "unsafe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResolvedPaths(tt.arts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestInstall_LocalDir_PathTraversalName(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "../escape", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# X"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error for path traversal in name")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Errorf("error should mention unsafe: %v", err)
	}
}

func TestInstall_LocalDir_PathTraversalVersion(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "ok-name", Version: "../../etc"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# X"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error for path traversal in version")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Errorf("error should mention unsafe: %v", err)
	}
}

func TestInstall_LocalDir_SymlinkEscape(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	secretDir := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	if err := os.WriteFile(filepath.Join(secretDir, "secret.txt"), []byte("sensitive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(secretDir, "secret.txt"), filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "symlink-test", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error for symlink escape")
	}
	if !strings.Contains(err.Error(), "symlink") || !strings.Contains(err.Error(), "outside") {
		t.Errorf("error should mention symlink escape: %v", err)
	}
}

func TestInstall_LocalDir_PartialCopyCleanup(t *testing.T) {
	skillDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	// Create a spec file that passes ValidateLocal (os.Stat succeeds for dirs)
	// but fails during copyLocalToCache (os.ReadFile fails on a directory).
	if err := os.MkdirAll(filepath.Join(skillDir, "bad-file.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "partial-copy", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "bad-file.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# OK"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error for unreadable spec file during copy")
	}

	cacheDir := installer.CacheDir("partial-copy", "1.0.0")
	if _, statErr := os.Stat(cacheDir); !os.IsNotExist(statErr) {
		t.Errorf("cache dir should be removed after partial copy failure, stat err: %v", statErr)
	}
}

func TestInstall_LocalDir_DirWithoutManifest(t *testing.T) {
	emptyDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", emptyDir})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for directory without artifact.json")
	}
	if !strings.Contains(err.Error(), "artifact.json") {
		t.Errorf("error should mention artifact.json, got: %v", err)
	}
}

func TestInstall_LocalDir_WithDeps(t *testing.T) {
	home := t.TempDir()
	skillDir := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	const depDigest = "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	depName := "dep-a"
	depVersion := "1.0.0"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Set("Content-Length", "512")
			w.Header().Set("Docker-Content-Digest", depDigest)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	srvHost := strings.TrimPrefix(srv.URL, "http://")

	dockerDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dockerDir, "config.json"), []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dockerDir)

	rootManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "root-with-dep", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{
				RegistryHost: srvHost,
				Repository:   depName,
				Tag:          depVersion,
			},
		},
	}
	data, err := json.Marshal(rootManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Root"), 0o644); err != nil {
		t.Fatal(err)
	}

	depCacheDir := installer.CacheDir(depName, depVersion)
	if err := os.MkdirAll(depCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	depManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: depName, Version: depVersion},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	depData, err := json.Marshal(depManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCacheDir, "artifact.json"), depData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCacheDir, "SKILL.md"), []byte("# Dep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installer.WriteDigest(depCacheDir, depDigest); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.String(), "Installed 2") {
		t.Errorf("expected 2 artifacts installed, got %q", out.String())
	}

	rootTarget := filepath.Join(home, ".cursor", "skills", "root-with-dep")
	if _, err := os.Stat(filepath.Join(rootTarget, "SKILL.md")); err != nil {
		t.Errorf("root SKILL.md not installed: %v", err)
	}
	depTarget := filepath.Join(home, ".cursor", "skills", depName)
	if _, err := os.Stat(filepath.Join(depTarget, "SKILL.md")); err != nil {
		t.Errorf("dep SKILL.md not installed: %v", err)
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatalf("load installed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 DB entries, got %d", len(entries))
	}
}

func TestInstall_LocalDir_SkillWithPromptDep(t *testing.T) {
	home := t.TempDir()
	skillDir := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	const depDigest = "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	depName := "severity-rubric"
	depVersion := "1.0.0"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Set("Content-Length", "512")
			w.Header().Set("Docker-Content-Digest", depDigest)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	srvHost := strings.TrimPrefix(srv.URL, "http://")

	dockerDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dockerDir, "config.json"), []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dockerDir)

	rootManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "skill-with-prompt", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{
				RegistryHost: srvHost,
				Repository:   depName,
				Tag:          depVersion,
			},
		},
	}
	data, err := json.Marshal(rootManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill with Prompt dep"), 0o644); err != nil {
		t.Fatal(err)
	}

	depCacheDir := installer.CacheDir(depName, depVersion)
	if err := os.MkdirAll(depCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	depManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: depName, Version: depVersion},
		Spec:       artifact.Spec{Entrypoint: "severity-rubric.md", Files: []string{"severity-rubric.md"}},
	}
	depData, err := json.Marshal(depManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCacheDir, "artifact.json"), depData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCacheDir, "severity-rubric.md"), []byte("# Severity Rubric"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installer.WriteDigest(depCacheDir, depDigest); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("install skill with prompt dep: %v", err)
	}
	if !strings.Contains(out.String(), "Installed 1") {
		t.Errorf("expected 1 artifact installed (Prompt dep excluded), got %q", out.String())
	}

	rootTarget := filepath.Join(home, ".cursor", "skills", "skill-with-prompt")
	if _, err := os.Stat(filepath.Join(rootTarget, "SKILL.md")); err != nil {
		t.Errorf("root SKILL.md not installed: %v", err)
	}
	depTarget := filepath.Join(home, ".cursor", "skills", depName)
	if _, err := os.Stat(depTarget); err == nil {
		t.Errorf("prompt dep should NOT be installed to target dir, but %s exists", depTarget)
	}
}

func TestInstall_LocalDir_PromptDepSkipsConflictCheck(t *testing.T) {
	home := t.TempDir()
	skillDir := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	const depDigest = "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	depName := "shared-name"
	depVersion := "2.0.0"

	if err := installer.SaveInstalled([]installer.InstalledEntry{{
		Skill:   depName,
		Version: "1.0.0",
		Target:  "cursor",
		Status:  "ok",
	}}); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Set("Content-Length", "512")
			w.Header().Set("Docker-Content-Digest", depDigest)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	srvHost := strings.TrimPrefix(srv.URL, "http://")

	dockerDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dockerDir, "config.json"), []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dockerDir)

	rootManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "my-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{
				RegistryHost: srvHost,
				Repository:   depName,
				Tag:          depVersion,
			},
		},
	}
	data, err := json.Marshal(rootManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	depCacheDir := installer.CacheDir(depName, depVersion)
	if err := os.MkdirAll(depCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	depManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: depName, Version: depVersion},
		Spec:       artifact.Spec{Entrypoint: "prompt.md", Files: []string{"prompt.md"}},
	}
	depData, err := json.Marshal(depManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCacheDir, "artifact.json"), depData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCacheDir, "prompt.md"), []byte("# Prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installer.WriteDigest(depCacheDir, depDigest); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "cursor", skillDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("prompt dep should not trigger conflict check, got: %v", err)
	}
}

func TestPullToStagingDir_CreatesIsolatedDir(t *testing.T) {
	parentDir := t.TempDir()

	var capturedStagingDir string
	artifactDir, cleanup, err := pullToStagingDir(parentDir, "my-artifact", func(stagingDir string) error {
		capturedStagingDir = stagingDir

		if !strings.HasPrefix(filepath.Base(stagingDir), ".staging-my-artifact-") {
			t.Errorf("staging dir name should start with .staging-my-artifact-, got %q", filepath.Base(stagingDir))
		}

		rel, relErr := filepath.Rel(parentDir, stagingDir)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			t.Errorf("staging dir should be under parentDir, got %q", stagingDir)
		}

		entries, readErr := os.ReadDir(stagingDir)
		if readErr != nil {
			t.Fatalf("read staging dir: %v", readErr)
		}
		if len(entries) != 0 {
			t.Errorf("staging dir should start empty, got %d entries", len(entries))
		}

		if err := os.MkdirAll(filepath.Join(stagingDir, "my-artifact"), 0o755); err != nil {
			t.Fatal(err)
		}
		return os.WriteFile(filepath.Join(stagingDir, "my-artifact", "test.txt"), []byte("data"), 0o600)
	})
	if err != nil {
		t.Fatalf("pullToStagingDir: %v", err)
	}
	defer cleanup()

	expectedArtifactDir := filepath.Join(capturedStagingDir, "my-artifact")
	if artifactDir != expectedArtifactDir {
		t.Errorf("artifactDir = %q, want %q", artifactDir, expectedArtifactDir)
	}
	if _, err := os.Stat(filepath.Join(artifactDir, "test.txt")); err != nil {
		t.Errorf("test.txt should exist in artifactDir: %v", err)
	}
}

func TestPullToStagingDir_CleanupRemovesTempDir(t *testing.T) {
	parentDir := t.TempDir()

	var capturedStagingDir string
	_, cleanup, err := pullToStagingDir(parentDir, "cleanup-test", func(stagingDir string) error {
		capturedStagingDir = stagingDir
		return os.MkdirAll(filepath.Join(stagingDir, "cleanup-test"), 0o755)
	})
	if err != nil {
		t.Fatalf("pullToStagingDir: %v", err)
	}

	cleanup()

	if _, err := os.Stat(capturedStagingDir); !os.IsNotExist(err) {
		t.Errorf("staging dir should be removed after cleanup, stat err = %v", err)
	}
}

func TestPullToStagingDir_CleanupOnPullFailure(t *testing.T) {
	parentDir := t.TempDir()

	_, cleanup, err := pullToStagingDir(parentDir, "fail-test", func(stagingDir string) error {
		if err := os.WriteFile(filepath.Join(stagingDir, "partial.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return fmt.Errorf("simulated pull failure")
	})
	if err == nil {
		t.Fatal("expected error from failing pullFn")
	}
	if !strings.Contains(err.Error(), "simulated pull failure") {
		t.Errorf("error should propagate: %v", err)
	}

	cleanup()

	entries, readErr := os.ReadDir(parentDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Errorf("leftover staging dir after cleanup: %s", e.Name())
		}
	}
}

func TestPullToStagingDir_StaleFilesNotCarriedForward(t *testing.T) {
	parentDir := t.TempDir()

	staleDir := filepath.Join(parentDir, "my-artifact")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "stale.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	artifactDir, cleanup, err := pullToStagingDir(parentDir, "my-artifact", func(stagingDir string) error {
		dest := filepath.Join(stagingDir, "my-artifact")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dest, "fresh.txt"), []byte("new"), 0o600)
	})
	if err != nil {
		t.Fatalf("pullToStagingDir: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(artifactDir, "fresh.txt")); err != nil {
		t.Errorf("fresh.txt should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifactDir, "stale.txt")); !os.IsNotExist(err) {
		t.Errorf("stale.txt should NOT exist in staging dir, stat err = %v", err)
	}
}

func TestPullToStagingDir_CreatesParentDirIfMissing(t *testing.T) {
	parentDir := filepath.Join(t.TempDir(), "nested", "cache")

	artifactDir, cleanup, err := pullToStagingDir(parentDir, "new-dir-test", func(stagingDir string) error {
		dest := filepath.Join(stagingDir, "new-dir-test")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dest, "file.txt"), []byte("ok"), 0o600)
	})
	if err != nil {
		t.Fatalf("pullToStagingDir with missing parent: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(artifactDir, "file.txt")); err != nil {
		t.Errorf("file.txt should exist: %v", err)
	}
}

func TestPullToStagingDir_ConcurrentPullsGetSeparateDirs(t *testing.T) {
	parentDir := t.TempDir()

	type result struct {
		artifactDir string
		cleanup     func()
		err         error
	}

	ch := make(chan result, 2)
	for i := 0; i < 2; i++ {
		marker := fmt.Sprintf("marker-%d", i)
		go func() {
			ad, cl, err := pullToStagingDir(parentDir, "concurrent", func(stagingDir string) error {
				dest := filepath.Join(stagingDir, "concurrent")
				if err := os.MkdirAll(dest, 0o755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dest, marker+".txt"), []byte(marker), 0o600)
			})
			ch <- result{ad, cl, err}
		}()
	}

	var results []result
	for i := 0; i < 2; i++ {
		results = append(results, <-ch)
	}
	for _, r := range results {
		if r.err != nil {
			t.Fatalf("concurrent pullToStagingDir failed: %v", r.err)
		}
		defer r.cleanup()
	}

	if results[0].artifactDir == results[1].artifactDir {
		t.Error("concurrent pulls should use different staging directories")
	}

	for i, r := range results {
		entries, err := os.ReadDir(r.artifactDir)
		if err != nil {
			t.Fatalf("read artifactDir %d: %v", i, err)
		}
		if len(entries) != 1 {
			t.Errorf("artifactDir %d should have exactly 1 file, got %d", i, len(entries))
		}
	}
}

func TestPullToStagingDir_RejectsUnsafeArtifactNames(t *testing.T) {
	parentDir := t.TempDir()
	noop := func(string) error { return nil }

	cases := []string{
		"../escape",
		"foo/bar",
		"foo\\bar",
		"..\\escape",
		"a/../b",
	}
	for _, name := range cases {
		_, cleanup, err := pullToStagingDir(parentDir, name, noop)
		cleanup()
		if err == nil {
			t.Errorf("expected error for artifact name %q, got nil", name)
		}
		if err != nil && !strings.Contains(err.Error(), "unsafe artifact name") {
			t.Errorf("error for %q should mention unsafe artifact name: %v", name, err)
		}
	}
}

func setupGitRepo(t *testing.T) string {
	return setupLocalGitRepo(t, "", "v1.0.0")
}

func TestInstall_GitRef_HappyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	repoURL := setupGitRepo(t)

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "git:" + repoURL + "@v1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install git ref: %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("output %q", out.String())
	}

	cursorSkills := filepath.Join(home, ".cursor", "skills", "git-skill")
	if _, err := os.Stat(filepath.Join(cursorSkills, "artifact.json")); err != nil {
		t.Errorf("artifact not installed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cursorSkills, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "Git Skill") {
		t.Errorf("SKILL.md = %q, want 'Git Skill'", string(data))
	}
}

func TestInstall_GitRef_WithCommit_HappyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	repoURL := setupGitRepo(t)

	// Extract the bare repo path from the file:// URL to get the commit SHA.
	barePath := strings.TrimPrefix(repoURL, "file://")
	commitOut, err := exec.Command("git", "-C", barePath, "rev-parse", "v1.0.0^{commit}").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	commitSHA := strings.TrimSpace(string(commitOut))

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "git:" + repoURL + "@v1.0.0!" + commitSHA})
	if err := root.Execute(); err != nil {
		t.Fatalf("install git ref with commit: %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("output %q", out.String())
	}

	cursorSkills := filepath.Join(home, ".cursor", "skills", "git-skill")
	if _, err := os.Stat(filepath.Join(cursorSkills, "artifact.json")); err != nil {
		t.Errorf("artifact not installed: %v", err)
	}
}
