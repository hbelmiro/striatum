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
		"a\nctrl.md",                             // embedded newline
		"a\x01ctrl.md",                           // control byte
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
		for i := range n {
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

// MLflow applies full Unicode case folding, so "Straße.md" and "STRASSE.md"
// collide, and directory names participate in collisions.
func TestCheckMLflowCompat_FullUnicodeFoldingCollisions(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": validSKILL})
	// "ß" folds to "ss".
	if err := CheckMLflowCompat(dir, []string{"SKILL.md", "Straße.md", "STRASSE.md"}, MLflowOptions{}); err == nil {
		t.Error("expected collision between Straße.md and STRASSE.md under full case folding")
	}
	// Directory collisions: files differ, but their parent directories fold
	// to the same name.
	dir2 := stageFiles(t, map[string]string{"SKILL.md": validSKILL})
	if err := CheckMLflowCompat(dir2, []string{"SKILL.md", "Assets/a.md", "assets/b.md"}, MLflowOptions{}); err == nil {
		t.Error("expected collision between Assets/ and assets/ directories")
	}
	// A file and a directory folding to the same name collide.
	dir3 := stageFiles(t, map[string]string{"SKILL.md": validSKILL})
	if err := CheckMLflowCompat(dir3, []string{"SKILL.md", "assets.md", "Assets/a.md"}, MLflowOptions{}); err == nil {
		t.Error("expected collision between file assets.md and directory Assets/")
	}
}

// The frontmatter closing delimiter must be a complete "---" line;
// "---not-a-delimiter" does not close it. CRLF frontmatter is accepted.
func TestCheckMLflowCompat_FrontmatterDelimiterStrictness(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": "---\nname: x\n---not-a-delimiter\nbody\n"})
	if err := CheckMLflowCompat(dir, []string{"SKILL.md"}, MLflowOptions{}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("err = %v, want unclosed frontmatter error", err)
	}
	dir2 := stageFiles(t, map[string]string{"SKILL.md": "---\r\nname: x\r\n---\r\nbody\r\n"})
	if err := CheckMLflowCompat(dir2, []string{"SKILL.md"}, MLflowOptions{}); err != nil {
		t.Fatalf("CRLF frontmatter rejected: %v", err)
	}
}

// Paths the consumer rejects on other platforms are refused before
// publication: reserved device names, illegal characters, and trailing
// dots or spaces.
func TestCheckMLflowCompat_ReservedAndIllegalSegments(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": validSKILL})
	for _, bad := range []string{"CON.md", "com1.md", "assets/file:stream", "report?.md", "notes.md.", "trailing space .md", `a<b.md`, "a|b.md", "CON/x.md", "assets/LPT1/y.md"} {
		if err := CheckMLflowCompat(dir, []string{"SKILL.md", bad}, MLflowOptions{}); err == nil {
			t.Errorf("path %q: expected rejection", bad)
		}
	}
}

// A closing frontmatter delimiter with trailing whitespace is accepted.
func TestCheckMLflowCompat_FrontmatterClosingTrailingSpace(t *testing.T) {
	dir := stageFiles(t, map[string]string{"SKILL.md": "---\nname: x\n--- \nbody\n"})
	if err := CheckMLflowCompat(dir, []string{"SKILL.md"}, MLflowOptions{}); err != nil {
		t.Fatalf("frontmatter with trailing space on closing delimiter rejected: %v", err)
	}
}
