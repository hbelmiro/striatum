package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"oras.land/oras-go/v2/content/memory"
	orasoci "oras.land/oras-go/v2/content/oci"
)

func TestInspect_ReturnsManifestFromPackedLayout(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "inspect-skill", Version: "2.0.0", Description: "For inspect"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Pack(manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	store, err := orasoci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	ref := "inspect-skill:2.0.0"
	got, err := Inspect(context.Background(), store, ref)
	if err != nil {
		t.Fatalf("Inspect() err = %v", err)
	}
	if got.Metadata.Name != "inspect-skill" || got.Metadata.Version != "2.0.0" {
		t.Errorf("got name=%q version=%q", got.Metadata.Name, got.Metadata.Version)
	}
	if got.Metadata.Description != "For inspect" {
		t.Errorf("got description=%q", got.Metadata.Description)
	}
}

func TestInspect_UnknownRefReturnsError(t *testing.T) {
	store := memory.New()
	_, err := Inspect(context.Background(), store, "nonexistent:1.0.0")
	if err == nil {
		t.Error("Inspect(unknown ref) err = nil, want error")
	}
}
