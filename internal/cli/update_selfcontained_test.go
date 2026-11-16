package cli

import (
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/installer"
)

// Exported installs are immutable by design (pinned by digest); update must
// skip them explicitly instead of mangling the digest-only source ref into a
// bogus repository and emitting tag-listing warnings.
func TestUpdate_SkipsSelfContainedEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)

	entries := []installer.InstalledEntry{{
		Name:          "pr-review",
		Kind:          "Skill",
		Version:       "1.0.0",
		Registry:      "localhost:5000/skills/pr-review@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		Target:        "claude",
		Status:        "ok",
		UpdatedAt:     "2026-01-01T00:00:00Z",
		SelfContained: true,
	}}
	if err := installer.SaveInstalled(entries); err != nil {
		t.Fatal(err)
	}

	out := &strings.Builder{}
	errOut := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"update", "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --check: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "not auto-updatable") {
		t.Errorf("output should mark exported entry not auto-updatable: %q", output)
	}
	if !strings.Contains(output, "All artifacts are up to date") {
		t.Errorf("output should report up to date: %q", output)
	}
	if strings.Contains(errOut.String(), "could not list tags") {
		t.Errorf("update should not attempt tag listing for exported entries: %q", errOut.String())
	}
}
