package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	ocistore "oras.land/oras-go/v2/content/oci"
)

func TestPack_CreatesOCILayout(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	// Write artifact.json and one file
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
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

func TestPack_WithDepFiles_AddsExtraLayers(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "my-wf", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "review.js", Files: []string{"review.js"}},
	}
	if err := os.WriteFile(filepath.Join(baseDir, "review.js"), []byte("// script"), 0o600); err != nil {
		t.Fatal(err)
	}

	depDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(depDir, "rubric.md"), []byte("# Rubric"), 0o600); err != nil {
		t.Fatal(err)
	}

	depFiles := []DepFile{
		{AnnotationPath: "deps/severity-rubric/rubric.md", DiskPath: filepath.Join(depDir, "rubric.md")},
	}

	if err := Pack(context.Background(), manifest, baseDir, layoutDir, depFiles...); err != nil {
		t.Fatalf("Pack() err = %v", err)
	}

	// Verify the packed artifact can be pulled and includes the dep file
	pulledDir := t.TempDir()
	store, err := ociStore(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Pull(context.Background(), store, "my-wf:1.0.0", pulledDir); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	depPath := filepath.Join(pulledDir, "my-wf", "deps", "severity-rubric", "rubric.md")
	data, err := os.ReadFile(depPath)
	if err != nil {
		t.Fatalf("dep file not extracted: %v", err)
	}
	if string(data) != "# Rubric" {
		t.Errorf("dep file content = %q, want %q", string(data), "# Rubric")
	}
}

func TestPack_NilDepFiles_NoChange(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "my-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# Skill"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatalf("Pack() err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(layoutDir, "index.json")); err != nil {
		t.Errorf("index.json missing: %v", err)
	}
}

func ociStore(layoutDir string) (*ocistore.Store, error) {
	return ocistore.New(layoutDir)
}

func TestPack_MissingFileReturnsError(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
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
