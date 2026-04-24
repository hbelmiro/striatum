package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"oras.land/oras-go/v2/content/memory"
	orasoci "oras.land/oras-go/v2/content/oci"
)

func TestInspect_ReturnsManifestFromPackedLayout(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "inspect-skill", Version: "2.0.0", Description: "For inspect"},
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

	if err := Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
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

func TestResolveDigest_ReturnsDigest(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "digest-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# test"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatal(err)
	}

	store, err := orasoci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	ref := "digest-skill:1.0.0"
	digest, err := ResolveDigest(context.Background(), store, ref)
	if err != nil {
		t.Fatalf("ResolveDigest() err = %v", err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("digest %q does not start with sha256:", digest)
	}
	if len(digest) != 71 {
		t.Errorf("digest %q length = %d, want 71 (sha256: + 64 hex chars)", digest, len(digest))
	}
}

func TestResolveDigest_UnknownRef(t *testing.T) {
	store := memory.New()
	_, err := ResolveDigest(context.Background(), store, "nonexistent:1.0.0")
	if err == nil {
		t.Error("ResolveDigest(unknown ref) err = nil, want error")
	}
}

func TestResolveDigest_DifferentContentDifferentDigest(t *testing.T) {
	layoutDir := t.TempDir()

	createAndPack := func(tag, content string) error {
		baseDir := t.TempDir()
		manifest := &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       "Skill",
			Metadata:   artifact.Metadata{Name: "multi-skill", Version: tag},
			Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		}
		data, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte(content), 0o600); err != nil {
			return err
		}
		return Pack(context.Background(), manifest, baseDir, layoutDir)
	}

	if err := createAndPack("1.0.0", "# v1"); err != nil {
		t.Fatal(err)
	}
	if err := createAndPack("2.0.0", "# v2"); err != nil {
		t.Fatal(err)
	}

	store, err := orasoci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}

	digest1, err := ResolveDigest(context.Background(), store, "multi-skill:1.0.0")
	if err != nil {
		t.Fatalf("ResolveDigest(1.0.0) err = %v", err)
	}

	digest2, err := ResolveDigest(context.Background(), store, "multi-skill:2.0.0")
	if err != nil {
		t.Fatalf("ResolveDigest(2.0.0) err = %v", err)
	}

	if digest1 == digest2 {
		t.Errorf("digests should differ for different content, both = %q", digest1)
	}
}
