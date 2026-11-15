package selfcontained

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stageFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var validSKILL = "---\nname: my-skill\ndescription: x\n---\n\n# My skill\n"

func TestCheckMLflowCompat_ValidRoot(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": validSKILL, "helpers/rubric.md": "x"})
	if err := CheckMLflowCompat(dir, []string{"SKILL.md", "helpers/rubric.md"}, MLflowOptions{}); err != nil {
		t.Fatalf("valid export: %v", err)
	}
}

func TestCheckMLflowCompat_MissingRootSkillMD(t *testing.T) {
	dir := stageFiles(t, map[string]string{"PROMPT.md": "x"})
	err := CheckMLflowCompat(dir, []string{"PROMPT.md"}, MLflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "SKILL.md") {
		t.Fatalf("err = %v, want missing root SKILL.md", err)
	}
}

func TestCheckMLflowCompat_MissingFrontmatter(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": "# No frontmatter\n"})
	err := CheckMLflowCompat(dir, []string{"SKILL.md"}, MLflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("err = %v, want frontmatter error", err)
	}
}

func TestCheckMLflowCompat_FrontmatterWithoutName(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": "---\ndescription: x\n---\nbody\n"})
	err := CheckMLflowCompat(dir, []string{"SKILL.md"}, MLflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err = %v, want frontmatter name error", err)
	}
}

func TestCheckMLflowCompat_EmptyFrontmatterName(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": "---\nname: \"\"\n---\nbody\n"})
	err := CheckMLflowCompat(dir, []string{"SKILL.md"}, MLflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err = %v, want frontmatter name error", err)
	}
}

func TestCheckMLflowCompat_InvalidFrontmatterYAML(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": "---\nname: [unclosed\n---\nbody\n"})
	err := CheckMLflowCompat(dir, []string{"SKILL.md"}, MLflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("err = %v, want frontmatter YAML error", err)
	}
}

func TestCheckMLflowCompat_InvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: x\n---\n\xff\xfe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := CheckMLflowCompat(dir, []string{"SKILL.md"}, MLflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("err = %v, want UTF-8 error", err)
	}
}

func TestCheckMLflowCompat_UnportablePaths(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": validSKILL})
	for _, bad := range []string{
		`a\b.md`, "../up.md", "a/../b.md", "/abs.md",
		"a\nctrl.md",        // embedded newline
		"a\x01ctrl.md",      // control byte
		string([]byte{'a', 0xff, '.', 'm', 'd'}), // invalid UTF-8
	} {
		err := CheckMLflowCompat(dir, []string{"SKILL.md", bad}, MLflowOptions{})
		if err == nil {
			t.Errorf("path %q: expected error", bad)
		}
	}
}

func TestCheckMLflowCompat_CaseCollision(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": validSKILL, "README.md": "a", "readme.md": "b"})
	err := CheckMLflowCompat(dir, []string{"SKILL.md", "README.md", "readme.md"}, MLflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("err = %v, want case collision error", err)
	}
}

func TestCheckMLflowCompat_UnicodeNFCCollision(t *testing.T) {
	// "é" as a single NFC codepoint vs "e" + combining acute (NFD).
	dir := stageFiles(t, map[string]string{"SKILL.md": validSKILL, "café.md": "a", "café.md": "b"})
	err := CheckMLflowCompat(dir, []string{"SKILL.md", "café.md", "café.md"}, MLflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("err = %v, want Unicode collision error", err)
	}
}

func TestCheckMLflowCompat_LayerLimit(t *testing.T) {
	makeList := func(n int) []string {
		l := []string{"SKILL.md"}
		for i := 0; i < n; i++ {
			l = append(l, fmt.Sprintf("f%04d.md", i))
		}
		return l
	}
	m := map[string]string{"SKILL.md": validSKILL}
	for _, f := range makeList(256)[1:] {
		m[f] = "x"
	}
	dir := stageFiles(t, m)

	if err := CheckMLflowCompat(dir, makeList(255), MLflowOptions{}); err != nil {
		t.Fatalf("256 files: unexpected error %v", err)
	}
	if err := CheckMLflowCompat(dir, makeList(256), MLflowOptions{}); err == nil || !strings.Contains(err.Error(), "256") {
		t.Fatalf("257 files: err = %v, want layer limit error", err)
	}
}

// TestMLflowDefaults pins the audited MLflow consumer limits the
// self-contained image enforces by default, asserted through the zero-value
// fallback behavior against the literal limits (comparing the constants
// directly would fold at compile time and could never fail). If these change
// upstream, update them here and in mlflow.go together.
func TestMLflowDefaults(t *testing.T) {
	var zero MLflowOptions
	if got := zero.maxLayers(); got != 256 {
		t.Errorf("default layer limit = %d, want 256 (MLflow layer limit)", got)
	}
	if got := zero.maxSizeBytes(); got != 25*1024*1024 {
		t.Errorf("default size budget = %d, want %d (MLflow 25 MiB budget)", got, 25*1024*1024)
	}
	// Explicit options are respected, not replaced by the defaults.
	custom := MLflowOptions{MaxLayers: 10, MaxSizeBytes: 42}
	if custom.maxLayers() != 10 || custom.maxSizeBytes() != 42 {
		t.Errorf("custom MLflowOptions = (%d, %d), want (10, 42)", custom.maxLayers(), custom.maxSizeBytes())
	}
}

func TestCheckMLflowCompat_SizeBudget(t *testing.T) {
	dir := stageFiles(t, map[string]string{
		"SKILL.md": validSKILL,
		"big.bin":  strings.Repeat("x", 100),
	})
	files := []string{"SKILL.md", "big.bin"}
	skSize := int64(len(validSKILL))
	if err := CheckMLflowCompat(dir, files, MLflowOptions{MaxSizeBytes: skSize + 100}); err != nil {
		t.Fatalf("exactly at budget: unexpected error %v", err)
	}
	if err := CheckMLflowCompat(dir, files, MLflowOptions{MaxSizeBytes: skSize + 99}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("over budget: err = %v, want size budget error", err)
	}
}
