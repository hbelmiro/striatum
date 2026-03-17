package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
)

func TestInspect_FromOCILayoutPrintsManifest(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
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
