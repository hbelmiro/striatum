package installer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestProjectPathToSlug(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "simple absolute path",
			path: "/Users/hbelmiro/dev/hbelmiro/striatum",
			want: "-Users-hbelmiro-dev-hbelmiro-striatum",
		},
		{
			name: "path with dots",
			path: "/Users/hbelmiro/.config/test",
			want: "-Users-hbelmiro--config-test",
		},
		{
			name: "path with underscores",
			path: "/Users/hbelmiro/my_project",
			want: "-Users-hbelmiro-my-project",
		},
		{
			name: "path with hyphens",
			path: "/Users/hbelmiro/my-project",
			want: "-Users-hbelmiro-my-project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectPathToSlug(tt.path)
			if got != tt.want {
				t.Errorf("ProjectPathToSlug(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestProjectPathToSlug_LongPath(t *testing.T) {
	long := "/" + strings.Repeat("a", 300)
	slug := ProjectPathToSlug(long)
	if len(slug) > 200 {
		t.Errorf("slug for long path should be truncated, got length %d", len(slug))
	}
	if !regexp.MustCompile(`-[0-9a-f]+$`).MatchString(slug) {
		t.Errorf("truncated slug should end with a hash, got %q", slug)
	}
}

func TestProjectPathToSlug_OnlyAlphanumericPreserved(t *testing.T) {
	slug := ProjectPathToSlug("/a/b.c_d-e f")
	for i, c := range slug {
		if c != '-' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			t.Errorf("slug[%d] = %c, want alphanumeric or '-'", i, c)
		}
	}
}

func TestMemoryTargetDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	gitDir := filepath.Join(projectDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := MemoryTargetDir(projectDir)
	if err != nil {
		t.Fatalf("MemoryTargetDir: %v", err)
	}

	slug := ProjectPathToSlug(projectDir)
	want := filepath.Join(home, ".claude", "projects", slug, "memory")
	if got != want {
		t.Errorf("MemoryTargetDir(%q) = %q, want %q", projectDir, got, want)
	}
}

func TestMemoryTargetDir_ResolvesRelativePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	gitDir := filepath.Join(projectDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	_ = os.Chdir(projectDir)
	defer func() { _ = os.Chdir(origWd) }()

	got, err := MemoryTargetDir(".")
	if err != nil {
		t.Fatalf("MemoryTargetDir: %v", err)
	}

	// filepath.Abs may resolve symlinks on macOS (/var -> /private/var), so
	// derive the expected slug from the same resolved absolute path.
	resolvedProject, _ := filepath.Abs(".")
	resolvedGitRoot := resolvedProject
	slug := ProjectPathToSlug(resolvedGitRoot)
	want := filepath.Join(home, ".claude", "projects", slug, "memory")
	if got != want {
		t.Errorf("MemoryTargetDir(\".\") = %q, want %q", got, want)
	}
}

func TestMemoryTargetDir_EmptyPath_ReturnsError(t *testing.T) {
	_, err := MemoryTargetDir("")
	if err == nil {
		t.Fatal("MemoryTargetDir(\"\") want error, got nil")
	}
}

func TestMemoryTargetDir_NonGitDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()

	got, err := MemoryTargetDir(projectDir)
	if err != nil {
		t.Fatalf("MemoryTargetDir: %v", err)
	}

	slug := ProjectPathToSlug(projectDir)
	want := filepath.Join(home, ".claude", "projects", slug, "memory")
	if got != want {
		t.Errorf("MemoryTargetDir(%q) = %q, want %q", projectDir, got, want)
	}
}

func TestIsWorktree_MainRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if IsWorktree(dir) {
		t.Error("IsWorktree should return false for main repo (.git is directory)")
	}
}

func TestIsWorktree_Worktree(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /some/path"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsWorktree(dir) {
		t.Error("IsWorktree should return true when .git is a file")
	}
}

func TestIsWorktree_NoGit(t *testing.T) {
	dir := t.TempDir()
	if IsWorktree(dir) {
		t.Error("IsWorktree should return false when no .git exists")
	}
}

func TestSlugToPath_SimpleAbsolute(t *testing.T) {
	dir := t.TempDir()
	slug := ProjectPathToSlug(dir)

	got, err := SlugToPath(slug)
	if err != nil {
		t.Fatalf("SlugToPath(%q): %v", slug, err)
	}
	if got != dir {
		t.Errorf("SlugToPath(%q) = %q, want %q", slug, got, dir)
	}
}

func TestSlugToPath_WithHyphenatedDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "my-project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	slug := ProjectPathToSlug(dir)

	got, err := SlugToPath(slug)
	if err != nil {
		t.Fatalf("SlugToPath(%q): %v", slug, err)
	}
	if got != dir {
		t.Errorf("SlugToPath(%q) = %q, want %q", slug, got, dir)
	}
}

func TestSlugToPath_WithHiddenDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".hidden", "sub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	slug := ProjectPathToSlug(dir)

	got, err := SlugToPath(slug)
	if err != nil {
		t.Fatalf("SlugToPath(%q): %v", slug, err)
	}
	if got != dir {
		t.Errorf("SlugToPath(%q) = %q, want %q", slug, got, dir)
	}
}

func TestSlugToPath_InvalidSlug(t *testing.T) {
	_, err := SlugToPath("-nonexistent-path-that-does-not-exist-anywhere")
	if err == nil {
		t.Fatal("SlugToPath for nonexistent path want error, got nil")
	}
}

func TestAllMemoryTargetDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectsDir := filepath.Join(home, ".claude", "projects")

	repo1 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo1, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	slug1 := ProjectPathToSlug(repo1)
	if err := os.MkdirAll(filepath.Join(projectsDir, slug1), 0o755); err != nil {
		t.Fatal(err)
	}

	repo2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo2, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	slug2 := ProjectPathToSlug(repo2)
	if err := os.MkdirAll(filepath.Join(projectsDir, slug2), 0o755); err != nil {
		t.Fatal(err)
	}

	worktreeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktreeDir, ".git"), []byte("gitdir: /some/path"), 0o644); err != nil {
		t.Fatal(err)
	}
	slugWT := ProjectPathToSlug(worktreeDir)
	if err := os.MkdirAll(filepath.Join(projectsDir, slugWT), 0o755); err != nil {
		t.Fatal(err)
	}

	dirs, err := AllMemoryTargetDirs()
	if err != nil {
		t.Fatalf("AllMemoryTargetDirs: %v", err)
	}

	want1 := filepath.Join(projectsDir, slug1, "memory")
	want2 := filepath.Join(projectsDir, slug2, "memory")
	found1, found2 := false, false
	for _, d := range dirs {
		if d == want1 {
			found1 = true
		}
		if d == want2 {
			found2 = true
		}
		if strings.Contains(d, slugWT) {
			t.Errorf("AllMemoryTargetDirs should skip worktree, but found %q", d)
		}
	}
	if !found1 {
		t.Errorf("AllMemoryTargetDirs missing %q", want1)
	}
	if !found2 {
		t.Errorf("AllMemoryTargetDirs missing %q", want2)
	}
}

func TestAllMemoryTargetDirs_EmptyWhenNoDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dirs, err := AllMemoryTargetDirs()
	if err != nil {
		t.Fatalf("AllMemoryTargetDirs: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("AllMemoryTargetDirs = %v, want empty", dirs)
	}
}

func TestParseMemoryFrontmatter(t *testing.T) {
	data := []byte(`---
name: feedback-testing
description: Integration tests must hit a real database
metadata:
  type: feedback
---

Some content here.
`)
	name, desc, err := ParseMemoryFrontmatter(data)
	if err != nil {
		t.Fatalf("ParseMemoryFrontmatter: %v", err)
	}
	if name != "feedback-testing" {
		t.Errorf("name = %q, want %q", name, "feedback-testing")
	}
	if desc != "Integration tests must hit a real database" {
		t.Errorf("description = %q, want %q", desc, "Integration tests must hit a real database")
	}
}

func TestParseMemoryFrontmatter_MissingFields(t *testing.T) {
	data := []byte(`---
metadata:
  type: feedback
---

Content.
`)
	_, _, err := ParseMemoryFrontmatter(data)
	if err == nil {
		t.Fatal("ParseMemoryFrontmatter with missing name should error")
	}
}

func TestParseMemoryFrontmatter_NoFrontmatter(t *testing.T) {
	data := []byte(`Just plain text without frontmatter.`)
	_, _, err := ParseMemoryFrontmatter(data)
	if err == nil {
		t.Fatal("ParseMemoryFrontmatter without frontmatter should error")
	}
}

func TestKebabToTitleCase(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"feedback-testing", "Feedback Testing"},
		{"single", "Single"},
		{"a-b-c", "A B C"},
		{"already-Title", "Already Title"},
	}
	for _, tt := range tests {
		got := KebabToTitleCase(tt.input)
		if got != tt.want {
			t.Errorf("KebabToTitleCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestInstallMemoryToTarget(t *testing.T) {
	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "feedback_testing.md"), []byte("# test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "user_role.md"), []byte("# role"), 0o600); err != nil {
		t.Fatal(err)
	}

	memoryDir := t.TempDir()
	err := InstallMemoryToTarget(cacheDir, memoryDir, "my-memories", []string{"feedback_testing.md", "user_role.md"})
	if err != nil {
		t.Fatalf("InstallMemoryToTarget: %v", err)
	}

	dest := filepath.Join(memoryDir, "my-memories")
	if _, err := os.Stat(filepath.Join(dest, "feedback_testing.md")); err != nil {
		t.Errorf("feedback_testing.md not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "user_role.md")); err != nil {
		t.Errorf("user_role.md not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "artifact.json")); err == nil {
		t.Error("artifact.json should NOT be copied to memory target")
	}
}

func TestInstallMemoryToTarget_WithSubdirectories(t *testing.T) {
	cacheDir := t.TempDir()
	subDir := filepath.Join(cacheDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.md"), []byte("# nested"), 0o600); err != nil {
		t.Fatal(err)
	}

	memoryDir := t.TempDir()
	err := InstallMemoryToTarget(cacheDir, memoryDir, "art", []string{"sub/nested.md"})
	if err != nil {
		t.Fatalf("InstallMemoryToTarget: %v", err)
	}

	dest := filepath.Join(memoryDir, "art", "sub", "nested.md")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("nested file not copied: %v", err)
	}
}

func TestInstallMemoryToTarget_EmptySpecFiles(t *testing.T) {
	cacheDir := t.TempDir()
	memoryDir := t.TempDir()
	err := InstallMemoryToTarget(cacheDir, memoryDir, "empty", []string{})
	if err != nil {
		t.Fatalf("InstallMemoryToTarget with empty files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(memoryDir, "empty")); err != nil {
		t.Errorf("artifact directory should be created even with no files: %v", err)
	}
}

func TestInstallMemoryToTarget_MissingSourceFile(t *testing.T) {
	cacheDir := t.TempDir()
	memoryDir := t.TempDir()
	err := InstallMemoryToTarget(cacheDir, memoryDir, "art", []string{"nonexistent.md"})
	if err == nil {
		t.Fatal("InstallMemoryToTarget with missing source file should error")
	}
}

func TestSlugToPath_EmptySlug(t *testing.T) {
	_, err := SlugToPath("")
	if err == nil {
		t.Fatal("SlugToPath(\"\") want error, got nil")
	}
}

func TestSlugToPath_NoLeadingDash(t *testing.T) {
	_, err := SlugToPath("no-leading-dash")
	if err == nil {
		t.Fatal("SlugToPath without leading dash want error, got nil")
	}
}

func TestSlugToPath_WithUnderscoreDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "my_project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	slug := ProjectPathToSlug(dir)

	got, err := SlugToPath(slug)
	if err != nil {
		t.Fatalf("SlugToPath(%q): %v", slug, err)
	}
	if got != dir {
		t.Errorf("SlugToPath(%q) = %q, want %q", slug, got, dir)
	}
}

func TestMemoryTargetDir_WhitespaceOnlyPath(t *testing.T) {
	_, err := MemoryTargetDir("   ")
	if err == nil {
		t.Fatal("MemoryTargetDir(\"   \") want error, got nil")
	}
}

func TestFindGitRoot_WalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(root, "src", "pkg")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := findGitRoot(subDir)
	if got != root {
		t.Errorf("findGitRoot(%q) = %q, want %q", subDir, got, root)
	}
}

func TestParseMemoryFrontmatter_UnclosedFrontmatter(t *testing.T) {
	data := []byte("---\nname: test\n")
	_, _, err := ParseMemoryFrontmatter(data)
	if err == nil {
		t.Fatal("ParseMemoryFrontmatter with unclosed frontmatter should error")
	}
}

func TestParseMemoryFrontmatter_InvalidYAML(t *testing.T) {
	data := []byte("---\n: invalid yaml [\n---\n")
	_, _, err := ParseMemoryFrontmatter(data)
	if err == nil {
		t.Fatal("ParseMemoryFrontmatter with invalid YAML should error")
	}
}

func TestKebabToTitleCase_EmptyString(t *testing.T) {
	got := KebabToTitleCase("")
	if got != "" {
		t.Errorf("KebabToTitleCase(\"\") = %q, want \"\"", got)
	}
}

func TestAllMemoryTargetDirs_SkipsNonDirEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectsDir := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectsDir, "a-regular-file"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := AllMemoryTargetDirs()
	if err != nil {
		t.Fatalf("AllMemoryTargetDirs: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("should return no dirs, got %v", dirs)
	}
}

func TestAllMemoryTargetDirs_SkipsUnresolvableSlugs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectsDir := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(filepath.Join(projectsDir, "-nonexistent-path-xyz"), 0o755); err != nil {
		t.Fatal(err)
	}

	dirs, err := AllMemoryTargetDirs()
	if err != nil {
		t.Fatalf("AllMemoryTargetDirs: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("should skip unresolvable slugs, got %v", dirs)
	}
}
