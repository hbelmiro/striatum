package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddMemoryIndexEntries_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")

	entries := []MemoryIndexEntry{
		{Title: "Feedback Testing", RelPath: "team-conv/feedback_testing.md", Description: "Integration tests must hit a real database"},
	}
	if err := AddMemoryIndexEntries(mdPath, "team-conv", entries); err != nil {
		t.Fatalf("AddMemoryIndexEntries: %v", err)
	}

	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "- [Feedback Testing](team-conv/feedback_testing.md)") {
		t.Errorf("MEMORY.md missing entry, got:\n%s", content)
	}
	if !strings.Contains(content, "Integration tests must hit a real database") {
		t.Errorf("MEMORY.md missing description, got:\n%s", content)
	}
}

func TestAddMemoryIndexEntries_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")
	existing := "- [Existing](existing/file.md) — Some existing memory\n"
	if err := os.WriteFile(mdPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []MemoryIndexEntry{
		{Title: "New Entry", RelPath: "new-pkg/entry.md", Description: "A new memory"},
	}
	if err := AddMemoryIndexEntries(mdPath, "new-pkg", entries); err != nil {
		t.Fatalf("AddMemoryIndexEntries: %v", err)
	}

	data, _ := os.ReadFile(mdPath)
	content := string(data)
	if !strings.Contains(content, "Existing") {
		t.Error("existing entry should be preserved")
	}
	if !strings.Contains(content, "New Entry") {
		t.Error("new entry should be appended")
	}
}

func TestAddMemoryIndexEntries_ReplacesExistingArtifact(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")
	existing := "- [Old Title](team-conv/old_file.md) — Old description\n- [Other](other/file.md) — Keep this\n"
	if err := os.WriteFile(mdPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []MemoryIndexEntry{
		{Title: "New Title", RelPath: "team-conv/new_file.md", Description: "New description"},
	}
	if err := AddMemoryIndexEntries(mdPath, "team-conv", entries); err != nil {
		t.Fatalf("AddMemoryIndexEntries: %v", err)
	}

	data, _ := os.ReadFile(mdPath)
	content := string(data)
	if strings.Contains(content, "Old Title") {
		t.Error("old entry for same artifact should be removed")
	}
	if !strings.Contains(content, "New Title") {
		t.Error("new entry should be added")
	}
	if !strings.Contains(content, "Other") {
		t.Error("entries from other artifacts should be preserved")
	}
}

func TestRemoveMemoryIndexEntries_RemovesMatching(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")
	content := "- [Entry A](team-conv/a.md) — Desc A\n- [Entry B](other/b.md) — Desc B\n- [Entry C](team-conv/c.md) — Desc C\n"
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveMemoryIndexEntries(mdPath, "team-conv"); err != nil {
		t.Fatalf("RemoveMemoryIndexEntries: %v", err)
	}

	data, _ := os.ReadFile(mdPath)
	result := string(data)
	if strings.Contains(result, "Entry A") || strings.Contains(result, "Entry C") {
		t.Error("matching entries should be removed")
	}
	if !strings.Contains(result, "Entry B") {
		t.Error("non-matching entries should be preserved")
	}
}

func TestRemoveMemoryIndexEntries_PreservesOther(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")
	content := "- [Entry B](other/b.md) — Desc B\n"
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveMemoryIndexEntries(mdPath, "team-conv"); err != nil {
		t.Fatalf("RemoveMemoryIndexEntries: %v", err)
	}

	data, _ := os.ReadFile(mdPath)
	result := string(data)
	if !strings.Contains(result, "Entry B") {
		t.Error("non-matching entries should be preserved")
	}
}

func TestRemoveMemoryIndexEntries_NoOpWhenNoMatch(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")
	content := "- [Entry](other/file.md) — Desc\n"
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveMemoryIndexEntries(mdPath, "nonexistent"); err != nil {
		t.Fatalf("RemoveMemoryIndexEntries: %v", err)
	}

	data, _ := os.ReadFile(mdPath)
	if string(data) != content {
		t.Errorf("content should be unchanged, got %q", string(data))
	}
}

func TestRemoveMemoryIndexEntries_NoOpWhenFileAbsent(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")

	if err := RemoveMemoryIndexEntries(mdPath, "anything"); err != nil {
		t.Fatalf("RemoveMemoryIndexEntries on absent file: %v", err)
	}
}

func TestAddMemoryIndexEntries_EmptyEntriesRemovesExisting(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")
	existing := "- [Old](team-conv/old.md) — Old entry\n- [Keep](other/keep.md) — Keep me\n"
	if err := os.WriteFile(mdPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AddMemoryIndexEntries(mdPath, "team-conv", nil); err != nil {
		t.Fatalf("AddMemoryIndexEntries: %v", err)
	}

	data, _ := os.ReadFile(mdPath)
	content := string(data)
	if strings.Contains(content, "Old") {
		t.Error("old entry for team-conv should be removed")
	}
	if !strings.Contains(content, "Keep") {
		t.Error("entries from other artifacts should be preserved")
	}
}

func TestAddMemoryIndexEntries_NoDescription(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")

	entries := []MemoryIndexEntry{
		{Title: "No Desc", RelPath: "art/file.md"},
	}
	if err := AddMemoryIndexEntries(mdPath, "art", entries); err != nil {
		t.Fatalf("AddMemoryIndexEntries: %v", err)
	}

	data, _ := os.ReadFile(mdPath)
	content := string(data)
	expected := "- [No Desc](art/file.md)"
	if !strings.Contains(content, expected) {
		t.Errorf("expected %q in output, got:\n%s", expected, content)
	}
	if strings.Contains(content, "—") {
		t.Error("should not contain em-dash separator when description is empty")
	}
}

func TestRemoveMemoryIndexEntries_FileBecomesEmpty(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "MEMORY.md")
	if err := os.WriteFile(mdPath, []byte("- [Only](art/only.md) — The only entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveMemoryIndexEntries(mdPath, "art"); err != nil {
		t.Fatalf("RemoveMemoryIndexEntries: %v", err)
	}

	data, _ := os.ReadFile(mdPath)
	if len(data) != 0 {
		t.Errorf("file should be empty after removing all entries, got %q", string(data))
	}
}

func TestMemoryIndexEntry_String(t *testing.T) {
	tests := []struct {
		entry MemoryIndexEntry
		want  string
	}{
		{
			MemoryIndexEntry{Title: "Feedback", RelPath: "art/fb.md", Description: "A desc"},
			"- [Feedback](art/fb.md) — A desc",
		},
		{
			MemoryIndexEntry{Title: "No Desc", RelPath: "art/nd.md"},
			"- [No Desc](art/nd.md)",
		},
	}
	for _, tt := range tests {
		got := tt.entry.String()
		if got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

func TestBuildMemoryIndexEntries(t *testing.T) {
	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, "good.md"), []byte("---\nname: fb-testing\ndescription: Use real DB\n---\nContent."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "no-fm.md"), []byte("plain text"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := BuildMemoryIndexEntries(cacheDir, "my-art", []string{"good.md", "no-fm.md", "missing.md"})
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (only good.md), got %d", len(entries))
	}
	if entries[0].Title != "Fb Testing" {
		t.Errorf("Title = %q, want %q", entries[0].Title, "Fb Testing")
	}
	if entries[0].RelPath != "my-art/good.md" {
		t.Errorf("RelPath = %q, want %q", entries[0].RelPath, "my-art/good.md")
	}
	if entries[0].Description != "Use real DB" {
		t.Errorf("Description = %q, want %q", entries[0].Description, "Use real DB")
	}
}
