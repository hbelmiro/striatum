package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
)

func TestPull_FromOCILayoutSucceeds(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	outDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "cli-pull", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(manifest, baseDir, layoutDir); err != nil {
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
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := oci.Pack(manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"pull", "oci:" + layoutDir + ":root:1.0.0"})
	err := root.Execute()
	if err == nil {
		t.Error("pull oci: with deps and no --registry: expected error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "registry") {
		t.Errorf("error should mention registry: %v", err)
	}
}
