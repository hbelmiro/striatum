package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

func TestPush_ToOCILayoutSucceeds(t *testing.T) {
	dir := t.TempDir()
	layoutDir := t.TempDir()
	t.Chdir(dir)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "cli-push", Version: "1.0.0"},
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

	ref := "oci:" + layoutDir + ":1.0.0"
	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"push", ref})
	if err := root.Execute(); err != nil {
		t.Fatalf("push: %v", err)
	}
	if !strings.Contains(out.String(), "Pushed") {
		t.Errorf("output %q does not contain Pushed", out.String())
	}
}

func TestPush_WithManifestFlagFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	layoutDir := t.TempDir()
	cwd := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "cli-push-remote", Version: "1.0.0"},
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

	ref := "oci:" + layoutDir + ":1.0.0"
	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"push", "-f", projectDir, ref})
	if err := root.Execute(); err != nil {
		t.Fatalf("push -f from other dir: %v", err)
	}
	if !strings.Contains(out.String(), "Pushed") {
		t.Errorf("output %q does not contain Pushed", out.String())
	}
}
