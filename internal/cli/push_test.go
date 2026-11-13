package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

func TestPush_OCIReferenceRejected(t *testing.T) {
	dir := t.TempDir()
	layoutDir := t.TempDir()
	t.Chdir(dir)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
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
	stderr := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetErr(stderr)
	root.SetArgs([]string{"push", ref})
	err = root.Execute()
	if err == nil {
		t.Fatal("push oci: ref: expected error")
	}
	if !strings.Contains(stderr.String(), "pack -o") {
		t.Errorf("stderr %q does not point to \"striatum pack -o\"", stderr.String())
	}
	entries, readErr := os.ReadDir(layoutDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Errorf("layout dir has %d entries after rejected push, want 0", len(entries))
	}
}

func TestPush_NoArtifactJSON_Errors(t *testing.T) {
	t.Chdir(t.TempDir())
	root := NewRootCommand()
	root.SetArgs([]string{"push", "oci:/tmp/layout:1.0.0"})
	if err := root.Execute(); err == nil {
		t.Error("push with no artifact.json: expected error")
	}
}

func TestPush_InvalidManifest_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), []byte(`{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	root.SetArgs([]string{"push", "oci:/tmp/layout:1.0.0"})
	if err := root.Execute(); err == nil {
		t.Error("push with empty name: expected error")
	}
}

func TestPush_MissingSpecFile_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), []byte(`{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md","no-such.md"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	root.SetArgs([]string{"push", "oci:/tmp/layout:1.0.0"})
	if err := root.Execute(); err == nil {
		t.Error("push with missing spec file: expected error")
	}
}

func TestPush_NoArgs_Errors(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"push"})
	if err := root.Execute(); err == nil {
		t.Error("push with no args: expected error")
	}
}

func TestPush_WithManifestFlagFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	layoutDir := t.TempDir()
	cwd := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
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
	stderr := &strings.Builder{}
	root := NewRootCommand()
	root.SetErr(stderr)
	root.SetArgs([]string{"push", "-f", projectDir, ref})
	err = root.Execute()
	if err == nil {
		t.Fatal("push -f with oci: ref: expected error")
	}
	// The manifest must load fine from projectDir; the failure must be the
	// registry-only rejection, not a manifest error.
	if !strings.Contains(stderr.String(), "pack -o") {
		t.Errorf("stderr %q does not point to \"striatum pack -o\"", stderr.String())
	}
}
