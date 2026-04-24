package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
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
