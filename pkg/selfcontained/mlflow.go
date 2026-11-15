package selfcontained

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultMaxLayers matches MLflow's limit of 256 layers per image.
	// Striatum's one-file-per-layer representation reaches it at 257 files.
	DefaultMaxLayers = 256
	// DefaultMaxSizeBytes matches MLflow's default content size budget (25 MiB).
	DefaultMaxSizeBytes = 25 * 1024 * 1024
)

// MLflowOptions bounds the self-contained payload according to the audited MLflow
// Skill Registry constraints. Zero values fall back to the defaults.
type MLflowOptions struct {
	MaxLayers    int
	MaxSizeBytes int64
}

func (o MLflowOptions) maxLayers() int {
	if o.MaxLayers <= 0 {
		return DefaultMaxLayers
	}
	return o.MaxLayers
}

func (o MLflowOptions) maxSizeBytes() int64 {
	if o.MaxSizeBytes <= 0 {
		return DefaultMaxSizeBytes
	}
	return o.MaxSizeBytes
}

// CheckMLflowCompat validates the prepared export tree at dir (with the given
// image-relative slash paths) against the audited MLflow constraints: root
// SKILL.md with valid UTF-8 and a frontmatter name, portable paths without
// case or Unicode collisions, layer count, and total content size. It belongs
// to export preparation and must not be applied to ordinary pack/push.
func CheckMLflowCompat(dir string, files []string, opts MLflowOptions) error {
	if !containsExact(files, "SKILL.md") {
		return fmt.Errorf("MLflow requires a root SKILL.md; the self-contained image has none")
	}
	if err := checkFrontmatter(filepath.Join(dir, "SKILL.md")); err != nil {
		return err
	}
	seen := map[string]string{}
	var total int64
	for _, f := range files {
		if err := checkPortablePath(f); err != nil {
			return err
		}
		key := strings.ToLower(norm.NFC.String(f))
		if prev, dup := seen[key]; dup {
			return fmt.Errorf("path collision after case/Unicode folding: %q and %q resolve to the same MLflow path", prev, f)
		}
		seen[key] = f
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			return fmt.Errorf("stat %q: %w", f, err)
		}
		total += info.Size()
	}
	if len(files) > opts.maxLayers() {
		return fmt.Errorf("export has %d files but MLflow accepts at most %d layers; reduce the number of files", len(files), opts.maxLayers())
	}
	if total > opts.maxSizeBytes() {
		return fmt.Errorf("export content is %d bytes, over the %d byte budget", total, opts.maxSizeBytes())
	}
	return nil
}

// checkFrontmatter validates the root SKILL.md: UTF-8, YAML frontmatter
// delimited by --- lines, and a non-empty scalar name field.
func checkFrontmatter(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read SKILL.md: %w", err)
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("SKILL.md is not valid UTF-8")
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return fmt.Errorf("SKILL.md has no YAML frontmatter; MLflow requires a frontmatter block starting with \"---\"")
	}
	rest := text[len("---\n"):]
	before, _, ok0 := strings.Cut(rest, "\n---")
	if !ok0 {
		return fmt.Errorf("SKILL.md frontmatter is not closed by a \"---\" line")
	}
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(before), &fm); err != nil {
		return fmt.Errorf("SKILL.md frontmatter is not valid YAML: %w", err)
	}
	name, ok := fm["name"]
	if !ok {
		return fmt.Errorf("SKILL.md frontmatter has no name; MLflow requires a frontmatter name")
	}
	nameStr, ok := name.(string)
	if !ok || strings.TrimSpace(nameStr) == "" {
		return fmt.Errorf("SKILL.md frontmatter name must be a non-empty string")
	}
	return nil
}

// checkPortablePath rejects paths MLflow cannot store portably.
func checkPortablePath(f string) error {
	if f == "" {
		return fmt.Errorf("empty path in export")
	}
	if strings.Contains(f, "\\") {
		return fmt.Errorf("path %q: backslashes are not portable", f)
	}
	if strings.HasPrefix(f, "/") {
		return fmt.Errorf("path %q: absolute paths are not portable", f)
	}
	if strings.HasPrefix(f, "../") || f == ".." || strings.Contains(f, "/../") {
		return fmt.Errorf("path %q: parent references are not portable", f)
	}
	if !utf8.ValidString(f) {
		return fmt.Errorf("path %q is not valid UTF-8", f)
	}
	for _, r := range f {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("path %q: control characters are not portable", f)
		}
	}
	return nil
}

func containsExact(list []string, s string) bool {
	return slices.Contains(list, s)
}
