package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveManifestAndProjectRoot_Default(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	gotManifest, gotRoot, err := resolveManifestAndProjectRoot("")
	if err != nil {
		t.Fatalf("resolveManifestAndProjectRoot: %v", err)
	}
	wantManifest, err := filepath.Abs(filepath.Join(tmp, defaultManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if gotManifest != wantManifest {
		t.Errorf("manifest path = %q, want %q", gotManifest, wantManifest)
	}
	if gotRoot != filepath.Dir(wantManifest) {
		t.Errorf("project root = %q, want %q", gotRoot, filepath.Dir(wantManifest))
	}
}

func TestResolveManifestAndProjectRoot_RelativeFile(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "packages", "myskill")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(sub, "artifact.json")
	if err := os.WriteFile(manifest, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)

	rel := filepath.Join("packages", "myskill", "artifact.json")
	gotManifest, gotRoot, err := resolveManifestAndProjectRoot(rel)
	if err != nil {
		t.Fatalf("resolveManifestAndProjectRoot: %v", err)
	}
	wantManifest, err := filepath.Abs(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if gotManifest != wantManifest {
		t.Errorf("manifest path = %q, want %q", gotManifest, wantManifest)
	}
	wantRoot, err := filepath.Abs(sub)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Errorf("project root = %q, want %q", gotRoot, wantRoot)
	}
}

func TestResolveManifestAndProjectRoot_AbsoluteFile(t *testing.T) {
	tmp := t.TempDir()
	manifest := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(manifest, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(tmp, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(other)

	absManifest, err := filepath.Abs(manifest)
	if err != nil {
		t.Fatal(err)
	}
	gotManifest, gotRoot, err := resolveManifestAndProjectRoot(absManifest)
	if err != nil {
		t.Fatalf("resolveManifestAndProjectRoot: %v", err)
	}
	if gotManifest != absManifest {
		t.Errorf("manifest path = %q, want %q", gotManifest, absManifest)
	}
	if gotRoot != tmp {
		t.Errorf("project root = %q, want %q", gotRoot, tmp)
	}
}

func TestResolveManifestAndProjectRoot_ManifestBasenameCaseInsensitive(t *testing.T) {
	tmp := t.TempDir()
	// Use non-canonical casing; resolution must still treat this as the manifest file path.
	manifest := filepath.Join(tmp, "Artifact.json")
	if err := os.WriteFile(manifest, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)

	gotManifest, gotRoot, err := resolveManifestAndProjectRoot("Artifact.json")
	if err != nil {
		t.Fatalf("resolveManifestAndProjectRoot: %v", err)
	}
	wantManifest, err := filepath.Abs(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if gotManifest != wantManifest {
		t.Errorf("manifest path = %q, want %q (must not append second artifact.json)", gotManifest, wantManifest)
	}
	if gotRoot != tmp {
		t.Errorf("project root = %q, want %q", gotRoot, tmp)
	}
}

func TestResolveManifestAndProjectRoot_TrimsWhitespace(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)

	_, _, err := resolveManifestAndProjectRoot("  artifact.json  ")
	if err != nil {
		t.Fatalf("expected whitespace-trimmed path to resolve: %v", err)
	}
}

func TestResolveManifestAndProjectRoot_DirectoryContainsDefaultManifest(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)

	gotManifest, gotRoot, err := resolveManifestAndProjectRoot("pkg")
	if err != nil {
		t.Fatalf("resolveManifestAndProjectRoot: %v", err)
	}
	wantManifest, err := filepath.Abs(filepath.Join(sub, "artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	if gotManifest != wantManifest {
		t.Errorf("manifest path = %q, want %q", gotManifest, wantManifest)
	}
	wantRoot, err := filepath.Abs(sub)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Errorf("project root = %q, want %q", gotRoot, wantRoot)
	}
}

func TestResolveManifestAndProjectRoot_NonexistentProjectDir(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	missingDir := filepath.Join(tmp, "typo-proj")
	wantManifest, err := filepath.Abs(filepath.Join(missingDir, defaultManifestName))
	if err != nil {
		t.Fatal(err)
	}
	gotManifest, gotRoot, err := resolveManifestAndProjectRoot("typo-proj")
	if err != nil {
		t.Fatalf("resolveManifestAndProjectRoot: %v", err)
	}
	if gotManifest != wantManifest {
		t.Errorf("manifest path = %q, want %q (missing dir should still append %s)", gotManifest, wantManifest, defaultManifestName)
	}
	wantRoot, err := filepath.Abs(missingDir)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Errorf("project root = %q, want %q", gotRoot, wantRoot)
	}
}
