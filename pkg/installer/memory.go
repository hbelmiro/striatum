package installer

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

const maxSlugLen = 128

// ProjectPathToSlug converts an absolute path to a Claude Code project slug
// by replacing all non-alphanumeric characters with "-". Long paths are
// truncated and a hash is appended.
func ProjectPathToSlug(absPath string) string {
	slug := nonAlphanumeric.ReplaceAllString(absPath, "-")
	if len(slug) > maxSlugLen {
		h := sha256.Sum256([]byte(absPath))
		slug = slug[:maxSlugLen-17] + "-" + fmt.Sprintf("%x", h[:8])
	}
	return slug
}

// MemoryTargetDir resolves a project path to the Claude Code memory directory:
// ~/.claude/projects/<slug>/memory/
func MemoryTargetDir(projectPath string) (string, error) {
	if strings.TrimSpace(projectPath) == "" {
		return "", errors.New("project path is required for Memory artifacts")
	}
	abs, err := filepath.Abs(strings.TrimSpace(projectPath))
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	root := findGitRoot(abs)
	slug := ProjectPathToSlug(root)

	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(home, ".claude", "projects", slug, "memory"), nil
}

// findGitRoot walks up from dir looking for a .git directory. Returns dir
// itself if no git root is found.
func findGitRoot(dir string) string {
	cur := dir
	for {
		info, err := os.Stat(filepath.Join(cur, ".git"))
		if err == nil && info.IsDir() {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return dir
		}
		cur = parent
	}
}

// IsWorktree reports whether the directory at path is a git worktree
// (has a .git file rather than a .git directory).
func IsWorktree(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// SlugToPath reconstructs the original filesystem path from a Claude Code
// project slug by greedily matching directory segments against the filesystem.
// Each "-" in the slug could represent "/" (path separator) or a literal
// non-alphanumeric character (-, _, .) within a directory name. The algorithm
// uses DFS with filesystem checks as oracle to find the correct reconstruction.
func SlugToPath(slug string) (string, error) {
	if slug == "" || slug[0] != '-' {
		return "", fmt.Errorf("invalid slug %q: must start with '-'", slug)
	}
	return slugDFS("/", slug[1:], 0)
}

func slugDFS(prefix, remaining string, depth int) (string, error) {
	if depth > 200 {
		return "", fmt.Errorf("max recursion depth exceeded")
	}
	if remaining == "" {
		if isDir(prefix) {
			return filepath.Clean(prefix), nil
		}
		return "", fmt.Errorf("path %q does not exist", prefix)
	}

	seg := ""
	for i := 0; i < len(remaining); i++ {
		if remaining[i] != '-' {
			seg += string(remaining[i])
			continue
		}

		afterDash := remaining[i+1:]

		if seg == "" {
			// Empty segment from "--": the "-" was a non-"/" char (most commonly "." for hidden dirs)
			for _, c := range []string{".", "_"} {
				result, err := slugDFS(prefix, c+afterDash, depth+1)
				if err == nil {
					return result, nil
				}
			}
			return "", fmt.Errorf("cannot reconstruct at empty segment (position %d)", i)
		}

		// Try "/" interpretation: seg is a complete directory name
		tryDir := filepath.Join(prefix, seg)
		if isDir(tryDir) {
			result, err := slugDFS(tryDir, afterDash, depth+1)
			if err == nil {
				return result, nil
			}
		}

		// Try "_" and "." as literal replacements for this "-"
		for _, c := range []string{"_", "."} {
			result, err := slugDFS(prefix, seg+c+afterDash, depth+1)
			if err == nil {
				return result, nil
			}
		}

		// Treat "-" as a literal hyphen in the directory name and continue
		seg += "-"
	}

	// End of remaining: try seg as the final directory name
	if seg != "" {
		tryDir := filepath.Join(prefix, seg)
		if isDir(tryDir) {
			return filepath.Clean(tryDir), nil
		}
	}
	return "", fmt.Errorf("cannot reconstruct path: %q does not exist", filepath.Join(prefix, seg))
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// AllMemoryTargetDirs scans ~/.claude/projects/*/, reconstructs each slug to a
// filesystem path, skips worktrees, and returns memory target directories for
// the remaining projects.
func AllMemoryTargetDirs() ([]string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read projects dir: %w", err)
	}

	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		realPath, err := SlugToPath(slug)
		if err != nil {
			continue
		}
		if IsWorktree(realPath) {
			continue
		}
		dirs = append(dirs, filepath.Join(projectsDir, slug, "memory"))
	}
	return dirs, nil
}

type memoryFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// ParseMemoryFrontmatter extracts the name and description from a memory
// file's YAML frontmatter (delimited by ---).
func ParseMemoryFrontmatter(data []byte) (name, description string, err error) {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return "", "", errors.New("no frontmatter found")
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return "", "", errors.New("unclosed frontmatter")
	}
	yamlBlock := content[3 : 3+end]

	var fm memoryFrontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return "", "", fmt.Errorf("parse frontmatter: %w", err)
	}
	if strings.TrimSpace(fm.Name) == "" {
		return "", "", errors.New("frontmatter: name is required")
	}
	return fm.Name, fm.Description, nil
}

// KebabToTitleCase converts a kebab-case string to Title Case.
// "feedback-testing" becomes "Feedback Testing".
func KebabToTitleCase(s string) string {
	parts := strings.Split(strings.ToLower(s), "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// InstallMemoryToTarget copies only spec.files (not artifact.json) from
// cacheDir to memoryDir/<artifactName>/.
func InstallMemoryToTarget(cacheDir, memoryDir, artifactName string, specFiles []string) error {
	dest := filepath.Join(memoryDir, artifactName)
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("clean memory target: %w", err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create memory target: %w", err)
	}
	for _, f := range specFiles {
		src := filepath.Join(cacheDir, filepath.FromSlash(f))
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		info, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("stat %s: %w", f, err)
		}
		dstPath := filepath.Join(dest, filepath.FromSlash(f))
		if dir := filepath.Dir(dstPath); dir != dest {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create dir for %s: %w", f, err)
			}
		}
		if err := os.WriteFile(dstPath, data, info.Mode()); err != nil {
			return fmt.Errorf("write %s: %w", f, err)
		}
	}
	return nil
}
