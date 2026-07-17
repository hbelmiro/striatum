package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMemoryLinks_NoLinks(t *testing.T) {
	memoryDir := t.TempDir()
	artDir := filepath.Join(memoryDir, "my-art")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "test.md"), []byte("no links here"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := ValidateMemoryLinks(memoryDir, "my-art")
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestValidateMemoryLinks_AllValid(t *testing.T) {
	memoryDir := t.TempDir()
	artDir := filepath.Join(memoryDir, "my-art")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "a.md"), []byte("see [[other-memory]]"), 0o644); err != nil {
		t.Fatal(err)
	}

	otherDir := filepath.Join(memoryDir, "existing")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "other.md"), []byte("---\nname: other-memory\ndescription: test\n---\ncontent"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := ValidateMemoryLinks(memoryDir, "my-art")
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestValidateMemoryLinks_BrokenLinks(t *testing.T) {
	memoryDir := t.TempDir()
	artDir := filepath.Join(memoryDir, "my-art")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "a.md"), []byte("see [[nonexistent]]"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := ValidateMemoryLinks(memoryDir, "my-art")
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateMemoryLinks_MixedLinks(t *testing.T) {
	memoryDir := t.TempDir()
	artDir := filepath.Join(memoryDir, "my-art")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "a.md"), []byte("see [[valid-one]] and [[broken-one]]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "valid.md"), []byte("---\nname: valid-one\ndescription: test\n---\ncontent"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := ValidateMemoryLinks(memoryDir, "my-art")
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning (broken-one), got %d: %v", len(warnings), warnings)
	}
}

func TestValidateMemoryLinks_NonexistentArtDir(t *testing.T) {
	memoryDir := t.TempDir()
	warnings := ValidateMemoryLinks(memoryDir, "does-not-exist")
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for nonexistent dir, got %v", warnings)
	}
}

func TestValidateMemoryLinks_IgnoresNonMdFiles(t *testing.T) {
	memoryDir := t.TempDir()
	artDir := filepath.Join(memoryDir, "my-art")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "notes.txt"), []byte("see [[broken-link]]"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := ValidateMemoryLinks(memoryDir, "my-art")
	if len(warnings) != 0 {
		t.Errorf("expected no warnings (.txt should be ignored), got %v", warnings)
	}
}

func TestValidateMemoryLinks_NestedSubdirectories(t *testing.T) {
	memoryDir := t.TempDir()
	artDir := filepath.Join(memoryDir, "my-art", "sub")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "deep.md"), []byte("see [[missing-ref]]"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := ValidateMemoryLinks(memoryDir, "my-art")
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for nested link, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateMemoryLinks_MultipleBrokenLinks(t *testing.T) {
	memoryDir := t.TempDir()
	artDir := filepath.Join(memoryDir, "my-art")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "a.md"), []byte("see [[broken-a]] and [[broken-b]] and [[broken-c]]"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := ValidateMemoryLinks(memoryDir, "my-art")
	if len(warnings) != 3 {
		t.Fatalf("expected 3 warnings, got %d: %v", len(warnings), warnings)
	}
}
