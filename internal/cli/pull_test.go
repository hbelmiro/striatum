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

func TestPull_FromOCILayoutSucceeds(t *testing.T) {
	t.Setenv("STRIATUM_HOME", t.TempDir())
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	outDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "cli-pull", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pull", "oci:" + layoutDir + ":cli-pull:1.0.0", "--output", outDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if !strings.Contains(out.String(), "Pulled") {
		t.Errorf("output %q does not contain Pulled", out.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "cli-pull", "artifact.json")); err != nil {
		t.Errorf("pulled artifact.json missing: %v", err)
	}
	cacheArtifact := filepath.Join(installer.CacheDir("cli-pull", "1.0.0"), "artifact.json")
	if _, err := os.Stat(cacheArtifact); err != nil {
		t.Errorf("expected default pull to populate Striatum cache at %s: %v", cacheArtifact, err)
	}
}

func TestPull_OCILayoutWithDepsRequiresRegistry(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha1",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: "root", Version: "1.0.0"},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{{Name: "dep", Version: "1.0.0"}},
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

	root := NewRootCommand()
	root.SetArgs([]string{"pull", "oci:" + layoutDir + ":root:1.0.0"})
	err = root.Execute()
	if err == nil {
		t.Error("pull oci: with deps and no --registry: expected error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "registry") {
		t.Errorf("error should mention registry: %v", err)
	}
}

func TestPull_WhitespaceOnlyOutputFallsBackToDefault(t *testing.T) {
	t.Setenv("STRIATUM_HOME", t.TempDir())
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "ws-pull", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# ws"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pull", "oci:" + layoutDir + ":ws-pull:1.0.0", "--output", "   "})
	if err := root.Execute(); err != nil {
		t.Fatalf("pull with whitespace-only output: %v", err)
	}
	defaultOut := filepath.Join(cwd, "ws-pull")
	if _, err := os.Stat(filepath.Join(defaultOut, "ws-pull", "artifact.json")); err != nil {
		t.Errorf("expected artifact under default dir %s: %v", defaultOut, err)
	}
}

// TestPull_NoCache_OutputOnly specifies --no-cache must not populate ~/.striatum/cache (STRIATUM_HOME).
func TestPull_NoCache_OutputOnly(t *testing.T) {
	t.Setenv("STRIATUM_HOME", t.TempDir())
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	outDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "no-cache-pull", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"pull", "--no-cache", "oci:" + layoutDir + ":no-cache-pull:1.0.0", "--output", outDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("pull --no-cache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "no-cache-pull", "artifact.json")); err != nil {
		t.Fatalf("output artifact.json missing: %v", err)
	}
	cacheArtifact := filepath.Join(installer.CacheDir("no-cache-pull", "1.0.0"), "artifact.json")
	if _, err := os.Stat(cacheArtifact); !os.IsNotExist(err) {
		if err == nil {
			t.Fatalf("expected no cache entry at %s with --no-cache", cacheArtifact)
		}
		t.Fatalf("stat cache artifact: %v", err)
	}
}
