package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

func TestPack_CreatesLayoutAndPrintsMessage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "cli-pack", Version: "1.0.0"},
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

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	if !strings.Contains(out.String(), "Packed") {
		t.Errorf("output %q does not contain Packed", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".striatum", "oci-layout", "index.json")); err != nil {
		t.Errorf("layout index.json missing: %v", err)
	}
}

func TestPack_WithManifestFlagFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	cwd := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "remote-pack", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-f", projectDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -f from other dir: %v", err)
	}
	if !strings.Contains(out.String(), "Packed") {
		t.Errorf("output %q does not contain Packed", out.String())
	}
	idx := filepath.Join(projectDir, ".striatum", "oci-layout", "index.json")
	if _, err := os.Stat(idx); err != nil {
		t.Errorf("expected layout under project dir %s: %v", idx, err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".striatum", "oci-layout", "index.json")); err == nil {
		t.Error("did not expect OCI layout under unrelated cwd")
	}
}
