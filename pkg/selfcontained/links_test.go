package selfcontained

import (
	"strings"
	"testing"
)

// testTree mirrors a small ai-skills-like closure:
// root Skill my-skill -> Skill dep other-skill -> Prompt dep review-policy.
func testTree() []TreeArtifact {
	return []TreeArtifact{
		{Kind: "Skill", Name: "my-skill", Files: []string{"SKILL.md", "helpers/rubric.md"}, IsRoot: true},
		{Kind: "Skill", Name: "other-skill", Files: []string{"SKILL.md", "helpers/notes.md"}},
		{Kind: "Prompt", Name: "review-policy", Files: []string{"PROMPT.md", "examples/case.md"}},
	}
}

func rootSkill() TreeArtifact { return testTree()[0] }

func rewrite(t *testing.T, owner TreeArtifact, ownerFile, content string) string {
	t.Helper()
	out, err := RewriteLinks([]byte(content), owner, ownerFile, testTree())
	if err != nil {
		t.Fatalf("RewriteLinks: %v", err)
	}
	return string(out)
}

func TestRewriteLinks_RootSkillToPromptDep(t *testing.T) {
	// Authored in install coordinates: .claude/skills/my-skill/ -> ../../prompts/review-policy/
	got := rewrite(t, rootSkill(), "SKILL.md", "See [the policy](../../prompts/review-policy/PROMPT.md).\n")
	want := "See [the policy](deps/prompts/review-policy/PROMPT.md).\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_RootSkillToHelperFile(t *testing.T) {
	got := rewrite(t, rootSkill(), "SKILL.md", "Use [rubric](helpers/rubric.md).\n")
	want := "Use [rubric](helpers/rubric.md).\n" // same-artifact, already correct
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_RootSkillToSiblingSkillDep(t *testing.T) {
	got := rewrite(t, rootSkill(), "SKILL.md", "See [other](../other-skill/SKILL.md).\n")
	want := "See [other](deps/skills/other-skill/SKILL.md).\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_DepSkillToDepSkill(t *testing.T) {
	owner := testTree()[1] // other-skill
	got := rewrite(t, owner, "SKILL.md", "See [other](../my-skill/SKILL.md) and [policy](../../prompts/review-policy/PROMPT.md).\n")
	// From deps/skills/other-skill/: root file is three levels up at the image
	// root; the prompt dep is two levels up under deps/prompts/.
	want := "See [other](../../../SKILL.md) and [policy](../../prompts/review-policy/PROMPT.md).\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_DepFileInSubdirectory(t *testing.T) {
	owner := testTree()[2] // review-policy, file examples/case.md
	got := rewrite(t, owner, "examples/case.md", "Back to [prompt](../PROMPT.md).\n")
	want := "Back to [prompt](../PROMPT.md).\n" // already correct from the new location
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_ExternalReferencesUntouched(t *testing.T) {
	content := "URL [docs](https://example.com/a), anchor [sec](#section), absolute [proj](/etc/hosts), " +
		"mailto [me](mailto:x@y.z), protocol-relative [s](//cdn.example.com/x), empty [e]().\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	if got != content {
		t.Fatalf("got %q, want unchanged %q", got, content)
	}
}

func TestRewriteLinks_UserRelativeReferenceOutsideClosureUntouched(t *testing.T) {
	// Resolves to skills/unknown-skill/SKILL.md: outside the closure, not artifact-owned.
	got := rewrite(t, rootSkill(), "SKILL.md", "See [x](../unknown-skill/SKILL.md).\n")
	want := "See [x](../unknown-skill/SKILL.md).\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_CodeFencesUntouched(t *testing.T) {
	content := "Intro\n\n```markdown\n[policy](../../prompts/review-policy/PROMPT.md)\n```\n\nAfter\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	if got != content {
		t.Fatalf("got %q, want unchanged", got)
	}
}

func TestRewriteLinks_InlineCodeUntouched(t *testing.T) {
	content := "Use `[policy](../../prompts/review-policy/PROMPT.md)` inline.\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	if got != content {
		t.Fatalf("got %q, want unchanged", got)
	}
}

func TestRewriteLinks_ReferenceDefinition(t *testing.T) {
	got := rewrite(t, rootSkill(), "SKILL.md", "[policy]: ../../prompts/review-policy/PROMPT.md\nSee [policy].\n")
	want := "[policy]: deps/prompts/review-policy/PROMPT.md\nSee [policy].\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_AngleBracketDestination(t *testing.T) {
	got := rewrite(t, rootSkill(), "SKILL.md", "See [x](<../../prompts/review-policy/PROMPT.md>).\n")
	want := "See [x](<deps/prompts/review-policy/PROMPT.md>).\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_TitleAndFragmentPreserved(t *testing.T) {
	got := rewrite(t, rootSkill(), "SKILL.md", "See [x](../../prompts/review-policy/PROMPT.md#rules \"Title here\").\n")
	want := "See [x](deps/prompts/review-policy/PROMPT.md#rules \"Title here\").\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_ImageLinks(t *testing.T) {
	got := rewrite(t, rootSkill(), "SKILL.md", "![logo](../../prompts/review-policy/PROMPT.md)\n")
	want := "![logo](deps/prompts/review-policy/PROMPT.md)\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_NonMarkdownUntouched(t *testing.T) {
	content := []byte("[x](../../prompts/review-policy/PROMPT.md)\n")
	out, err := RewriteLinks(content, rootSkill(), "helpers/data.txt", testTree())
	if err != nil {
		t.Fatalf("RewriteLinks: %v", err)
	}
	if string(out) != string(content) {
		t.Fatalf("got %q, want unchanged", out)
	}
}

func TestRewriteLinks_UndeclaredArtifactOwnedFileErrors(t *testing.T) {
	_, err := RewriteLinks([]byte("See [x](../../prompts/review-policy/SECRET.md).\n"), rootSkill(), "SKILL.md", testTree())
	if err == nil {
		t.Fatal("expected error for undeclared artifact-owned reference")
	}
	for _, want := range []string{"SKILL.md", "SECRET.md", "review-policy"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want containing %q", err, want)
		}
	}
}

func TestRewriteLinks_ArtifactDirectoryReferenceErrors(t *testing.T) {
	_, err := RewriteLinks([]byte("See [x](../../prompts/review-policy).\n"), rootSkill(), "SKILL.md", testTree())
	if err == nil {
		t.Fatal("expected error for artifact-directory reference")
	}
}

func TestRewriteLinks_MultipleLinksPerLine(t *testing.T) {
	got := rewrite(t, rootSkill(), "SKILL.md",
		"See [a](https://x.com) and [b](../../prompts/review-policy/PROMPT.md) and [c](helpers/rubric.md).\n")
	want := "See [a](https://x.com) and [b](deps/prompts/review-policy/PROMPT.md) and [c](helpers/rubric.md).\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A four-backtick fence contains three-backtick example fences; links inside
// the block (including inside the example fence) must stay untouched, and
// links after the real closing fence must be rewritten.
func TestRewriteLinks_FenceLengthTracking(t *testing.T) {
	content := "before [p](../../prompts/review-policy/PROMPT.md)\n" +
		"````markdown\n" +
		"inside [p](../../prompts/review-policy/PROMPT.md)\n" +
		"```\n" +
		"still inside [p](../../prompts/review-policy/PROMPT.md)\n" +
		"````\n" +
		"after [p](../../prompts/review-policy/PROMPT.md)\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	want := "before [p](deps/prompts/review-policy/PROMPT.md)\n" +
		"````markdown\n" +
		"inside [p](../../prompts/review-policy/PROMPT.md)\n" +
		"```\n" +
		"still inside [p](../../prompts/review-policy/PROMPT.md)\n" +
		"````\n" +
		"after [p](deps/prompts/review-policy/PROMPT.md)\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Destinations may contain balanced parentheses.
func TestRewriteLinks_BalancedParentheses(t *testing.T) {
	tree := []TreeArtifact{
		{Kind: "Skill", Name: "root", Files: []string{"SKILL.md"}, IsRoot: true},
		{Kind: "Prompt", Name: "review-policy", Files: []string{"PROMPT.md", "helpers/review(policy).md"}},
	}
	content := "See [x](../../prompts/review-policy/helpers/review(policy).md).\n"
	out, err := RewriteLinks([]byte(content), tree[0], "SKILL.md", tree)
	if err != nil {
		t.Fatalf("RewriteLinks: %v", err)
	}
	want := "See [x](deps/prompts/review-policy/helpers/review(policy).md).\n"
	if string(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// Quoted HTML src and href attributes referencing artifact-owned files are
// rewritten; external URLs are untouched.
func TestRewriteLinks_HTMLAttributes(t *testing.T) {
	content := "<img src=\"../../prompts/review-policy/PROMPT.md\" alt=\"x\"> " +
		"<a href='../../prompts/review-policy/PROMPT.md'>policy</a> " +
		"<img src=\"https://example.com/logo.png\">\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	want := "<img src=\"deps/prompts/review-policy/PROMPT.md\" alt=\"x\"> " +
		"<a href='deps/prompts/review-policy/PROMPT.md'>policy</a> " +
		"<img src=\"https://example.com/logo.png\">\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// An HTML attribute pointing at an undeclared artifact-owned file fails
// clearly instead of silently leaving the image broken.
func TestRewriteLinks_HTMLAttributeUndeclaredErrors(t *testing.T) {
	_, err := RewriteLinks([]byte("<img src=\"../../prompts/review-policy/SECRET.png\">\n"), rootSkill(), "SKILL.md", testTree())
	if err == nil {
		t.Fatal("expected error for undeclared artifact-owned HTML reference")
	}
}

// A line ending exactly with "src=" must not panic the scanner.
func TestRewriteLinks_LineEndingWithAttrName(t *testing.T) {
	content := "Use the attribute src=\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	if got != content {
		t.Fatalf("got %q, want unchanged", got)
	}
}

// Prose quoting an attribute value must be left alone; only attributes
// inside actual HTML tags are rewritten.
func TestRewriteLinks_ProseAttributeUntouched(t *testing.T) {
	content := "Type src=\"../../prompts/review-policy/PROMPT.md\" in your editor.\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	if got != content {
		t.Fatalf("prose attribute rewritten: %q", got)
	}
	// Prose quoting an undeclared artifact path must not fail the build.
	if _, err := RewriteLinks([]byte("See src=\"../../prompts/review-policy/SECRET.png\" docs.\n"), rootSkill(), "SKILL.md", testTree()); err != nil {
		t.Fatalf("prose undeclared path errored: %v", err)
	}
}

// A closing fence with trailing whitespace still closes the block.
func TestRewriteLinks_ClosingFenceTrailingWhitespace(t *testing.T) {
	content := "```go\n[x](../../prompts/review-policy/PROMPT.md)\n``` \nafter [p](../../prompts/review-policy/PROMPT.md)\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	want := "```go\n[x](../../prompts/review-policy/PROMPT.md)\n``` \nafter [p](deps/prompts/review-policy/PROMPT.md)\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Prose attribute values appearing AFTER a closed HTML tag must be left
// alone: the backwards scan hits the tag's '>' first.
func TestRewriteLinks_ProseAfterClosedTag(t *testing.T) {
	content := "Click <a href='https://example.com'>here</a> then type src=\"../../prompts/review-policy/PROMPT.md\" in prose.\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	if got != content {
		t.Fatalf("prose after closed tag rewritten: %q", got)
	}
}

// Escaped parentheses in destinations are unescaped for resolution.
func TestRewriteLinks_EscapedParentheses(t *testing.T) {
	tree := []TreeArtifact{
		{Kind: "Skill", Name: "root", Files: []string{"SKILL.md"}, IsRoot: true},
		{Kind: "Prompt", Name: "review-policy", Files: []string{"PROMPT.md", "helpers/review(1).md"}},
	}
	content := "See [x](../../prompts/review-policy/helpers/review\\(1\\).md).\n"
	out, err := RewriteLinks([]byte(content), tree[0], "SKILL.md", tree)
	if err != nil {
		t.Fatalf("RewriteLinks: %v", err)
	}
	want := "See [x](deps/prompts/review-policy/helpers/review(1).md).\n"
	if string(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
