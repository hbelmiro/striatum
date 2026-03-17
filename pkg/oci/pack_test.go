package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

func TestPack_CreatesOCILayout(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	// Write artifact.json and one file
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "my-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# Skill"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatalf("Pack() err = %v", err)
	}

	// OCI layout must have index.json and blobs/
	if _, err := os.Stat(filepath.Join(layoutDir, "index.json")); err != nil {
		t.Errorf("index.json missing: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(layoutDir, "blobs")); err != nil || !fi.IsDir() {
		t.Errorf("blobs/ missing or not dir: %v", err)
	}
}

func TestPack_RequiresValidManifest(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	if err := Pack(context.Background(), nil, baseDir, layoutDir); err == nil {
		t.Error("Pack(nil) err = nil, want error")
	}
}

func TestPack_MissingFileReturnsError(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "x", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "missing.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Pack(context.Background(), manifest, baseDir, layoutDir); err == nil {
		t.Error("Pack with missing file: err = nil, want error")
	}
}
