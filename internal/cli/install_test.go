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
		APIVersion: "striatum.dev/v1alpha1",
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

// TestRefToCacheCandidate verifies name@version derivation from references.
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

// TestInstall_FromCache_WhenRefMapsToCachedSkill_SucceedsWithoutInspect ensures install
// uses the local cache when the reference maps to a cached name@version (no Inspect/pull for root).
func TestInstall_FromCache_WhenRefMapsToCachedSkill_SucceedsWithoutInspect(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	// Pre-populate cache with foo@1.0.0 (no registry needed)
	cacheDir := installer.CacheDir("foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
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

	// Install using a registry-style ref that maps to foo@1.0.0 (no server running)
	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "localhost:5000/skills/foo:1.0.0"})
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

// TestInstall_ShortRefWithDepsRequiresRegistry ensures short ref with dependencies fails without --registry.
func TestInstall_ShortRefWithDepsRequiresRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	// Cache has root with deps
	cacheDir := installer.CacheDir("example-skill", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha1",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: "example-skill", Version: "1.0.0"},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{{Name: "example-helper-a", Version: "1.0.0"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "example-skill:1.0.0"})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected error for short ref with deps and no --registry")
	}
	if !strings.Contains(err.Error(), "short ref with dependencies requires --registry") {
		t.Errorf("error should mention --registry: %v", err)
	}
}

// TestInstall_FromCache_WithDependencies ensures install uses cache for root and deps when all are cached.
func TestInstall_FromCache_WithDependencies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	// Pre-populate cache: root "example-skill@1.0.0" and deps "example-helper-a@1.0.0", "example-helper-b@1.0.0"
	for _, ent := range []struct {
		name, version string
		deps          []artifact.Dependency
	}{
		{"example-helper-a", "1.0.0", nil},
		{"example-helper-b", "1.0.0", nil},
		{"example-skill", "1.0.0", []artifact.Dependency{
			{Name: "example-helper-a", Version: "1.0.0"},
			{Name: "example-helper-b", Version: "1.0.0"},
		}},
	} {
		cacheDir := installer.CacheDir(ent.name, ent.version)
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := &artifact.Manifest{
			APIVersion:   "striatum.dev/v1alpha1",
			Kind:         "Skill",
			Metadata:     artifact.Metadata{Name: ent.name, Version: ent.version},
			Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
			Dependencies: ent.deps,
		}
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("# "+ent.name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Install with short ref + --registry (all from cache; registry required for short ref with deps)
	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"skill", "install", "--target", "cursor", "--registry", "example-registry", "example-skill:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install from cache with deps: %v", err)
	}
	if !strings.Contains(out.String(), "Installed") {
		t.Errorf("output %q", out.String())
	}
	// All three should be installed
	for _, name := range []string{"example-skill", "example-helper-a", "example-helper-b"} {
		dir := filepath.Join(home, ".cursor", "skills", name)
		if _, err := os.Stat(filepath.Join(dir, "artifact.json")); err != nil {
			t.Errorf("artifact %s not installed: %v", name, err)
		}
	}
}
