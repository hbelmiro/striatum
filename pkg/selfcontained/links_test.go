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
