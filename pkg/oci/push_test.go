package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"oras.land/oras-go/v2"
	orasoci "oras.land/oras-go/v2/content/oci"
)

func writePushFixture(t *testing.T, name string) (*artifact.Manifest, string) {
	t.Helper()
	baseDir := t.TempDir()
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: name, Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# "+name), 0o600); err != nil {
		t.Fatal(err)
	}
	return manifest, baseDir
}

func mustOpenLayout(t *testing.T, layoutDir string) oras.ReadOnlyTarget {
	t.Helper()
	store, err := orasoci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestPush_OCIReferenceRejected(t *testing.T) {
	layoutDir := t.TempDir()
	refs := []string{
		"oci:" + layoutDir + ":1.0.0",
		"oci:" + layoutDir,
	}
	for _, ref := range refs {
		manifest, baseDir := writePushFixture(t, "push-reject")
		_, err := Push(context.Background(), manifest, baseDir, ref)
		if err == nil {
			t.Errorf("Push(%q) err = nil, want error", ref)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, "registry-only") {
			t.Errorf("Push(%q) error %q does not mention registry-only", ref, msg)
		}
		if !strings.Contains(msg, "pack -o") {
			t.Errorf("Push(%q) error %q does not point to \"striatum pack -o\"", ref, msg)
		}
	}
}

func TestPush_OCIReferenceRejectedWithoutWritingLayout(t *testing.T) {
	layoutDir := t.TempDir()
	manifest, baseDir := writePushFixture(t, "push-reject-clean")
	_, err := Push(context.Background(), manifest, baseDir, "oci:"+layoutDir+":1.0.0")
	if err == nil {
		t.Fatal("Push(oci:) err = nil, want error")
	}
	entries, readErr := os.ReadDir(layoutDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Errorf("layout dir has %d entries after rejected push, want 0", len(entries))
	}
}

func TestPush_InvalidReferenceReturnsError(t *testing.T) {
	manifest, baseDir := writePushFixture(t, "x")
	_, err := Push(context.Background(), manifest, baseDir, "no-colon")
	if err == nil {
		t.Error("Push(invalid ref) err = nil, want error")
	}
}

func TestPush_UnreachableRegistryReturnsError(t *testing.T) {
	manifest, baseDir := writePushFixture(t, "x")
	// Port 1 has no listener; the connection is refused during copy.
	_, err := Push(context.Background(), manifest, baseDir, "localhost:1/unreachable/my-skill:1.0.0")
	if err == nil {
		t.Fatal("Push(unreachable registry) err = nil, want error")
	}
	if !strings.Contains(err.Error(), "push to registry") {
		t.Errorf("error %q does not come from the registry copy step", err)
	}
}

func TestPack_SameArtifactTwiceIsIdempotent(t *testing.T) {
	manifest, baseDir := writePushFixture(t, "repack")
	layoutDir := t.TempDir()

	if err := Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatalf("first Pack() err = %v", err)
	}
	if err := Pack(context.Background(), manifest, baseDir, layoutDir); err != nil {
		t.Fatalf("second Pack() err = %v", err)
	}

	m, err := Inspect(context.Background(), mustOpenLayout(t, layoutDir), "repack:1.0.0")
	if err != nil {
		t.Fatalf("Inspect after repack: %v", err)
	}
	if m.Metadata.Name != "repack" || m.Metadata.Version != "1.0.0" {
		t.Errorf("got name=%q version=%q", m.Metadata.Name, m.Metadata.Version)
	}
}
