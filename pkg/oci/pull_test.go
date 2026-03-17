package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	orasoci "oras.land/oras-go/v2/content/oci"
)

func TestPull_ExtractsArtifactToOutputDir(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	outputDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "pull-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "extra.md"}},
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# Skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "extra.md"), []byte("extra"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Pack(manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	store, err := orasoci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}

	if err := Pull(context.Background(), store, "pull-skill:1.0.0", outputDir); err != nil {
		t.Fatalf("Pull() err = %v", err)
	}

	artifactDir := filepath.Join(outputDir, "pull-skill")
	if _, err := os.Stat(filepath.Join(artifactDir, "artifact.json")); err != nil {
		t.Errorf("artifact.json missing: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(artifactDir, "SKILL.md")); err != nil || string(b) != "# Skill" {
		t.Errorf("SKILL.md: err=%v content=%q", err, string(b))
	}
	if b, err := os.ReadFile(filepath.Join(artifactDir, "extra.md")); err != nil || string(b) != "extra" {
		t.Errorf("extra.md: err=%v content=%q", err, string(b))
	}
}

func TestPull_UnknownRefReturnsError(t *testing.T) {
	store, _ := orasoci.New(t.TempDir())
	err := Pull(context.Background(), store, "nonexistent:1.0.0", t.TempDir())
	if err == nil {
		t.Error("Pull(unknown ref) err = nil, want error")
	}
}
