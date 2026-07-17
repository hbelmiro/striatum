package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MemoryIndexEntry represents a single line in MEMORY.md.
type MemoryIndexEntry struct {
	Title       string
	RelPath     string
	Description string
}

func (e MemoryIndexEntry) String() string {
	s := fmt.Sprintf("- [%s](%s)", e.Title, e.RelPath)
	if e.Description != "" {
		s += " — " + e.Description
	}
	return s
}

// AddMemoryIndexEntries reads MEMORY.md, removes any existing lines whose link
// path starts with artifactName/, appends the new entries, and writes back.
// Creates the file if it doesn't exist.
func AddMemoryIndexEntries(memoryMDPath, artifactName string, entries []MemoryIndexEntry) error {
	existing, err := readLines(memoryMDPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read MEMORY.md: %w", err)
	}

	kept := filterOutArtifact(existing, artifactName)
	for _, e := range entries {
		kept = append(kept, e.String())
	}

	content := strings.Join(kept, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if err := os.MkdirAll(filepath.Dir(memoryMDPath), 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	return os.WriteFile(memoryMDPath, []byte(content), 0o644)
}

// RemoveMemoryIndexEntries removes lines whose link path starts with
// artifactName/ from MEMORY.md. No-op if the file doesn't exist.
func RemoveMemoryIndexEntries(memoryMDPath, artifactName string) error {
	existing, err := readLines(memoryMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read MEMORY.md: %w", err)
	}

	kept := filterOutArtifact(existing, artifactName)
	content := strings.Join(kept, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(memoryMDPath, []byte(content), 0o644)
}

// BuildMemoryIndexEntries reads spec files from cacheDir, parses their
// frontmatter, and returns MEMORY.md index entries for files that have valid
// frontmatter. Files without frontmatter are silently skipped.
func BuildMemoryIndexEntries(cacheDir, artifactName string, specFiles []string) []MemoryIndexEntry {
	var entries []MemoryIndexEntry
	for _, f := range specFiles {
		data, err := os.ReadFile(filepath.Join(cacheDir, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		name, desc, fmErr := ParseMemoryFrontmatter(data)
		if fmErr != nil {
			continue
		}
		entries = append(entries, MemoryIndexEntry{
			Title:       KebabToTitleCase(name),
			RelPath:     artifactName + "/" + f,
			Description: desc,
		})
	}
	return entries
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := strings.TrimRight(string(data), "\n")
	if content == "" {
		return nil, nil
	}
	return strings.Split(content, "\n"), nil
}

// filterOutArtifact removes lines that contain a markdown link with a path
// starting with artifactName/.
func filterOutArtifact(lines []string, artifactName string) []string {
	prefix := "(" + artifactName + "/"
	var kept []string
	for _, line := range lines {
		if strings.Contains(line, prefix) {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}
