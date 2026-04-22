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

func TestInspect_WithDependencies_PrintsDepSection(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "root-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "reg.io", Repository: "skills/dep-a", Tag: "2.0.0"},
			&artifact.GitDependency{URL: "https://github.com/org/repo.git", Ref: "v3.0.0", Path: "sub"},
		},
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
	root.SetArgs([]string{"inspect", "oci:" + layoutDir + ":root-skill:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Dependencies:") {
		t.Errorf("output should contain Dependencies: section\n%s", got)
	}
	if !strings.Contains(got, "reg.io/skills/dep-a:2.0.0") {
		t.Errorf("output should contain OCI dep canonical ref\n%s", got)
	}
	if !strings.Contains(got, "[oci]") {
		t.Errorf("output should contain [oci] source\n%s", got)
	}
	if !strings.Contains(got, "github.com/org/repo.git") {
		t.Errorf("output should contain Git dep URL\n%s", got)
	}
	if !strings.Contains(got, "[git]") {
		t.Errorf("output should contain [git] source\n%s", got)
	}
}

func TestInspect_NoDeps_OmitsDepsSection(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "no-deps", Version: "1.0.0"},
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
	root.SetArgs([]string{"inspect", "oci:" + layoutDir + ":no-deps:1.0.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if strings.Contains(out.String(), "Dependencies:") {
		t.Errorf("output should NOT contain Dependencies: for zero-dep artifact\n%s", out.String())
	}
}

func TestInspect_InvalidRef_Errors(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"inspect", "oci:./nonexistent-layout:tag"})
	if err := root.Execute(); err == nil {
		t.Error("inspect with bad layout: expected error")
	}
}

func TestInspect_FromOCILayoutPrintsManifest(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
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
