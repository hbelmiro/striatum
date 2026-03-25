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
	gotOut := out.String()
	if !strings.Contains(gotOut, "Packed") {
		t.Errorf("output %q does not contain Packed", gotOut)
	}
	wantLayout := filepath.Join(dir, "build")
	if !strings.Contains(gotOut, wantLayout) {
		t.Errorf("output %q should mention layout path %q", gotOut, wantLayout)
	}
	if _, err := os.Stat(filepath.Join(wantLayout, "index.json")); err != nil {
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
	gotOut := out.String()
	if !strings.Contains(gotOut, "Packed") {
		t.Errorf("output %q does not contain Packed", gotOut)
	}
	wantLayout := filepath.Join(projectDir, "build")
	if !strings.Contains(gotOut, wantLayout) {
		t.Errorf("output %q should mention layout path %q", gotOut, wantLayout)
	}
	idx := filepath.Join(wantLayout, "index.json")
	if _, err := os.Stat(idx); err != nil {
		t.Errorf("expected layout under project dir %s: %v", idx, err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "build", "index.json")); err == nil {
		t.Error("did not expect OCI layout under unrelated cwd")
	}
}

func TestPack_CustomOutputAbsoluteLayout(t *testing.T) {
	projectDir := t.TempDir()
	customLayout := t.TempDir()
	t.Chdir(projectDir)

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "custom-out", Version: "1.0.0"},
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

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-o", customLayout})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -o: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, customLayout) {
		t.Errorf("output %q should mention layout path %q", gotOut, customLayout)
	}
	if _, err := os.Stat(filepath.Join(customLayout, "index.json")); err != nil {
		t.Errorf("expected index.json under custom output %s: %v", customLayout, err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "build", "index.json")); err == nil {
		t.Error("did not expect default <project>/build when -o is set")
	}
}

func TestPack_CustomOutputRelativeToCwd(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	relOut := "rel-oci-layout"
	wantLayout, err := filepath.Abs(relOut)
	if err != nil {
		t.Fatal(err)
	}

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "rel-out", Version: "1.0.0"},
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

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-o", relOut})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -o relative: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, wantLayout) {
		t.Errorf("output %q should mention resolved layout path %q", gotOut, wantLayout)
	}
	if _, err := os.Stat(filepath.Join(wantLayout, "index.json")); err != nil {
		t.Errorf("expected index.json under resolved output %s: %v", wantLayout, err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "build", "index.json")); err == nil {
		t.Error("did not expect default <project>/build when -o is set")
	}
}

func TestPack_CustomOutputWithManifestFlagFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	cwd := t.TempDir()
	customLayout := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "custom-f", Version: "1.0.0"},
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
	root.SetArgs([]string{"pack", "-f", projectDir, "-o", customLayout})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -f -o: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, customLayout) {
		t.Errorf("output %q should mention layout path %q", gotOut, customLayout)
	}
	if _, err := os.Stat(filepath.Join(customLayout, "index.json")); err != nil {
		t.Errorf("expected index.json under custom output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "build", "index.json")); err == nil {
		t.Error("did not expect <project>/build when -o is set")
	}
	if _, err := os.Stat(filepath.Join(cwd, "build", "index.json")); err == nil {
		t.Error("did not expect OCI layout under unrelated cwd")
	}
}

func TestPack_CustomOutputRelativeWithManifestFlagFromOtherDir(t *testing.T) {
	projectDir := t.TempDir()
	cwd := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "rel-f", Version: "1.0.0"},
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

	relOut := "out-layout"
	wantLayout, err := filepath.Abs(relOut)
	if err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-f", projectDir, "-o", relOut})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack -f -o relative: %v", err)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, wantLayout) {
		t.Errorf("output %q should mention resolved layout path %q", gotOut, wantLayout)
	}
	if _, err := os.Stat(filepath.Join(wantLayout, "index.json")); err != nil {
		t.Errorf("expected index.json under cwd-relative output %s: %v", wantLayout, err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "build", "index.json")); err == nil {
		t.Error("did not expect <project>/build when -o is set")
	}
	if _, err := os.Stat(filepath.Join(projectDir, relOut, "index.json")); err == nil {
		t.Error("did not expect layout under project dir for cwd-relative -o")
	}
	if _, err := os.Stat(filepath.Join(cwd, "build", "index.json")); err == nil {
		t.Error("did not expect OCI layout under cwd/build when -o is set")
	}
}
