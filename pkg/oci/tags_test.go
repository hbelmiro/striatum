package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

func pushTestArtifact(t *testing.T, layoutDir, name, version string) {
	t.Helper()
	baseDir := t.TempDir()
	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: name, Version: version},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# "+name), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := "oci:" + layoutDir + ":" + version
	if _, err := Push(context.Background(), m, baseDir, ref); err != nil {
		t.Fatalf("push %s:%s: %v", name, version, err)
	}
}

func TestListTags_ReturnsPushedTags(t *testing.T) {
	layoutDir := t.TempDir()
	pushTestArtifact(t, layoutDir, "my-skill", "1.0.0")
	pushTestArtifact(t, layoutDir, "my-skill", "2.0.0")
	pushTestArtifact(t, layoutDir, "my-skill", "1.5.0")

	tags, err := ListTags(context.Background(), "oci:"+layoutDir)
	if err != nil {
		t.Fatalf("ListTags() err = %v", err)
	}
	want := map[string]bool{"1.0.0": true, "2.0.0": true, "1.5.0": true}
	if len(tags) != len(want) {
		t.Fatalf("ListTags() returned %d tags, want %d: %v", len(tags), len(want), tags)
	}
	for _, tag := range tags {
		if !want[tag] {
			t.Errorf("unexpected tag %q", tag)
		}
	}
}

func TestListTags_EmptyLayout(t *testing.T) {
	layoutDir := t.TempDir()

	tags, err := ListTags(context.Background(), "oci:"+layoutDir)
	if err != nil {
		t.Fatalf("ListTags() err = %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("ListTags(empty) = %v, want empty", tags)
	}
}

func TestListTags_InvalidReference(t *testing.T) {
	_, err := ListTags(context.Background(), "no-scheme-or-colon")
	if err == nil {
		t.Error("ListTags(invalid) err = nil, want error")
	}
}
