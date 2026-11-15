package selfcontained

import "testing"

func TestRewriteLinks_ReferenceDefinitionAndInlineLinkOnSameLine(t *testing.T) {
	// A reference definition whose trailing text contains an inline link:
	// both destinations must rewrite regardless of edit insertion order.
	content := "[policy]: ../../prompts/review-policy/PROMPT.md see [x](../../prompts/review-policy/PROMPT.md)\n"
	got := rewrite(t, rootSkill(), "SKILL.md", content)
	want := "[policy]: deps/prompts/review-policy/PROMPT.md see [x](deps/prompts/review-policy/PROMPT.md)\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
