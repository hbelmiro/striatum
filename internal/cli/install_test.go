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

func TestInstall_MissingTargetErrors(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"install", "localhost:5000/skills/foo:1.0.0"})
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
	root.SetArgs([]string{"install", "--target", "all", "localhost:5000/skills/foo:1.0.0"})
	err := root.Execute()
	if err == nil {
		t.Error("install --target all: expected error")
	}
}

func TestInstall_HappyPathNoDeps(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	home := t.TempDir()
	_ = os.Setenv("STRIATUM_HOME", home)
	_ = os.Setenv("HOME", home)
	defer func() {
		_, _ = os.Unsetenv("STRIATUM_HOME"), os.Unsetenv("HOME")
	}()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "install-test", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
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

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"install", "--target", "cursor", "oci:" + layoutDir + ":install-test:1.0.0"})
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
