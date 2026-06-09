package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

func TestListCachedArtifacts_EmptyOrMissingCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	// No cache/ dir at all
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (missing cache dir)", len(got))
	}
}

func TestListCachedArtifacts_EmptyCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheRoot := filepath.Join(dir, cacheDirName)
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (empty cache)", len(got))
	}
}

func TestListCachedArtifacts_OneValidEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("foo", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactManifest(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "foo", Version: "1.0.0", Description: "A test skill"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
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

func TestListCachedArtifacts_MultipleEntriesSorted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	for _, name := range []string{"b", "a"} {
		cacheDir := CacheDir(name, "1.0.0")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeArtifactManifest(t, cacheDir, &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       "Skill",
			Metadata:   artifact.Metadata{Name: name, Version: "1.0.0"},
			Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		})
	}
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Name != "a" || got[1].Name != "b" {
		t.Errorf("want sorted by name: got %q, %q", got[0].Name, got[1].Name)
	}
}

func TestListCachedArtifacts_SkipsUnsupportedKind(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("other-type", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Valid manifest but unsupported kind
	writeArtifactManifest(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "VectorIndex",
		Metadata:   artifact.Metadata{Name: "other-type", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "index.json", Files: []string{"index.json"}},
	})
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (unsupported kind should be skipped)", len(got))
	}
}

func TestListCachedArtifacts_SkipsCorruptManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("corrupt", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (dir with corrupt artifact.json skipped)", len(got))
	}
}

func TestListCachedArtifacts_SkipsDirWithoutArtifactJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("partial", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No artifact.json
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (dir without artifact.json skipped)", len(got))
	}
}

func TestListCachedArtifacts_SkipsDirNameWithoutAt(t *testing.T) {
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
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (dir name without @ skipped)", len(got))
	}
}

func TestListCachedArtifacts_SkipsEmptySkillName(t *testing.T) {
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
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (dir @1.0.0 has empty skill name, skipped)", len(got))
	}
}

func TestListCachedArtifacts_NameWithMultipleAt_SplitOnLast(t *testing.T) {
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
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "a@b", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	})
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Name != "a@b" || got[0].Version != "1.0.0" {
		t.Errorf("got[0] = Name=%q Version=%q, want a@b and 1.0.0", got[0].Name, got[0].Version)
	}
}

func TestListCachedArtifacts_IncludesWorkflow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("my-wf", "2.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactManifest(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Workflow",
		Metadata:   artifact.Metadata{Name: "my-wf", Version: "2.0.0", Description: "A workflow"},
		Spec:       artifact.Spec{Entrypoint: "review.js", Files: []string{"review.js"}},
	})
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Name != "my-wf" || got[0].Kind != "Workflow" {
		t.Errorf("got[0] = %+v, want Name=my-wf Kind=Workflow", got[0])
	}
}

func TestListCachedArtifacts_IncludesPrompt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	cacheDir := CacheDir("severity-rubric", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArtifactManifest(t, cacheDir, &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Prompt",
		Metadata:   artifact.Metadata{Name: "severity-rubric", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "rubric.md", Files: []string{"rubric.md"}},
	})
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Kind != "Prompt" {
		t.Errorf("got[0].Kind = %q, want Prompt", got[0].Kind)
	}
}

func TestListCachedArtifacts_MixedKinds_AllShown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRIATUM_HOME", dir)
	for _, tc := range []struct {
		name, kind, entry string
	}{
		{"skill-a", "Skill", "SKILL.md"},
		{"wf-b", "Workflow", "script.js"},
		{"prompt-c", "Prompt", "prompt.md"},
	} {
		cacheDir := CacheDir(tc.name, "1.0.0")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeArtifactManifest(t, cacheDir, &artifact.Manifest{
			APIVersion: "striatum.dev/v1alpha2",
			Kind:       tc.kind,
			Metadata:   artifact.Metadata{Name: tc.name, Version: "1.0.0"},
			Spec:       artifact.Spec{Entrypoint: tc.entry, Files: []string{tc.entry}},
		})
	}
	got, err := ListCachedArtifacts()
	if err != nil {
		t.Fatalf("ListCachedArtifacts(): err = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
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
