package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	orasoci "oras.land/oras-go/v2/content/oci"
)

func TestPush_ToOCILayout_Roundtrip(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "push-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# Pushed"), 0o600); err != nil {
		t.Fatal(err)
	}

	ref := "oci:" + layoutDir + ":1.0.0"
	desc, err := Push(context.Background(), manifest, baseDir, ref)
	if err != nil {
		t.Fatalf("Push() err = %v", err)
	}

	digestStr := desc.Digest.String()
	if !strings.HasPrefix(digestStr, "sha256:") {
		t.Errorf("descriptor digest %q does not start with sha256:", digestStr)
	}
	if len(digestStr) != 71 {
		t.Errorf("descriptor digest length = %d, want 71", len(digestStr))
	}
	if desc.Size <= 0 {
		t.Errorf("descriptor size = %d, want > 0", desc.Size)
	}

	store, err := orasoci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Inspect(context.Background(), store, "1.0.0")
	if err != nil {
		t.Fatalf("Inspect after push: %v", err)
	}
	if m.Metadata.Name != "push-skill" || m.Metadata.Version != "1.0.0" {
		t.Errorf("got name=%q version=%q", m.Metadata.Name, m.Metadata.Version)
	}
}

func TestPush_InvalidReferenceReturnsError(t *testing.T) {
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "x", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Push(context.Background(), manifest, baseDir, "no-colon")
	if err == nil {
		t.Error("Push(invalid ref) err = nil, want error")
	}
}

func TestPush_ReturnsDescriptorMatchingResolvedDigest(t *testing.T) {
	baseDir := t.TempDir()
	layoutDir := t.TempDir()

	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "digest-check", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# Digest check"), 0o600); err != nil {
		t.Fatal(err)
	}

	ref := "oci:" + layoutDir + ":1.0.0"
	desc, err := Push(context.Background(), manifest, baseDir, ref)
	if err != nil {
		t.Fatalf("Push() err = %v", err)
	}

	store, err := orasoci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	resolvedDigest, err := ResolveDigest(context.Background(), store, "1.0.0")
	if err != nil {
		t.Fatalf("ResolveDigest() err = %v", err)
	}

	if desc.Digest.String() != resolvedDigest {
		t.Errorf("Push descriptor digest = %q, ResolveDigest = %q; want equal", desc.Digest.String(), resolvedDigest)
	}
}
