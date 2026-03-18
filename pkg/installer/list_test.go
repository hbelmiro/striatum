package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

func TestListCachedSkills_EmptyOrMissingCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	// No cache/ dir at all
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (missing cache dir)", len(got))
	}
}

func TestListCachedSkills_EmptyCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheRoot := filepath.Join(dir, cacheDirName)
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (empty cache)", len(got))
	}
}

func TestListCachedSkills_OneValidEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactManifest(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "foo", Version: "1.0.0", Description: "A test skill"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Name != "foo" || got[0].Version != "1.0.0" {
		t.Errorf("got[0] = %+v, want Name=foo Version=1.0.0", got[0])
	}
	if got[0].Description != "A test skill" {
		t.Errorf("got[0].Description = %q, want %q", got[0].Description, "A test skill")
	}
}

func TestListCachedSkills_MultipleEntriesSorted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	for _, name := range []string{"b", "a"} {
		cacheDir := CacheDir(name, "1.0.0")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeArtifactManifest(t, cacheDir, &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha1",
			Kind:       "Skill",
			Metadata:   artifact.Metadata{Name: name, Version: "1.0.0"},
			Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		})
	}
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Name != "a" || got[1].Name != "b" {
		t.Errorf("want sorted by name: got %q, %q", got[0].Name, got[1].Name)
	}
}

func TestListCachedSkills_SkipsNonSkillKind(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("other-type", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Valid manifest but kind is not Skill — should be omitted from list
	writeArtifactManifest(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "VectorIndex",
		Metadata:   artifact.Metadata{Name: "other-type", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "index.json", Files: []string{"index.json"}},
	})
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (non-Skill kind should be skipped)", len(got))
	}
}

func TestListCachedSkills_SkipsCorruptManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("corrupt", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (dir with corrupt artifact.json skipped)", len(got))
	}
}

func TestListCachedSkills_SkipsDirWithoutArtifactJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("partial", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No artifact.json
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (dir without artifact.json skipped)", len(got))
	}
}

func TestListCachedSkills_SkipsDirNameWithoutAt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheRoot := filepath.Join(dir, cacheDirName)
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	invalidDir := filepath.Join(cacheRoot, "invalid")
	if err := os.MkdirAll(invalidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (dir name without @ skipped)", len(got))
	}
}

func TestListCachedSkills_SkipsEmptySkillName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheRoot := filepath.Join(dir, cacheDirName)
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	emptyNameDir := filepath.Join(cacheRoot, "@1.0.0")
	if err := os.MkdirAll(emptyNameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactManifest(t, emptyNameDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (dir @1.0.0 has empty skill name, skipped)", len(got))
	}
}

func TestListCachedSkills_NameWithMultipleAt_SplitOnLast(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	// Dir name "a@b@1.0.0" -> name "a@b", version "1.0.0"
	cacheRoot := filepath.Join(dir, cacheDirName)
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	multiAtDir := filepath.Join(cacheRoot, "a@b@1.0.0")
	if err := os.MkdirAll(multiAtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactManifest(t, multiAtDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "a@b", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})
	got, err := ListCachedSkills()
	if err != nil {
		t.Fatalf("ListCachedSkills(): err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Name != "a@b" || got[0].Version != "1.0.0" {
		t.Errorf("got[0] = Name=%q Version=%q, want a@b and 1.0.0", got[0].Name, got[0].Version)
	}
}

func writeArtifactManifest(t *testing.T, dir string, m *artifact.Manifest) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
