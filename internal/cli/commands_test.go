package cli

import (
	"bytes"
	"strings"
	"testing"
)

var subcommands = []string{"init", "validate", "pack", "push", "pull", "inspect", "install", "uninstall", "list", "update"}

func TestSubcommands_Registered(t *testing.T) {
	root := NewRootCommand()
	for _, name := range subcommands {
		cmd, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatalf("Find(%q): %v", name, err)
		}
		if cmd == nil {
			t.Errorf("subcommand %q not registered", name)
		}
	}
}

func TestSubcommand_HelpExitsZero(t *testing.T) {
	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	for _, name := range subcommands {
		root.SetArgs([]string{name, "--help"})
		if err := root.Execute(); err != nil {
			t.Errorf("striatum %s --help: err = %v", name, err)
		}
	}
}

var commandsRequiringArg = []struct {
	args    []string
	desc    string
	tooMany []string
}{
	{
		args:    []string{"install"},
		desc:    "install",
		tooMany: []string{"install", "first", "second"},
	},
	{
		args:    []string{"uninstall"},
		desc:    "uninstall",
		tooMany: []string{"uninstall", "first", "second"},
	},
}

func TestCommands_ErrorWithoutArg(t *testing.T) {
	for _, tc := range commandsRequiringArg {
		root := NewRootCommand()
		root.SetArgs(tc.args)
		err := root.Execute()
		if err == nil {
			t.Errorf("striatum %s (no arg): expected error, got nil", tc.desc)
		}
	}
}

func TestCommands_ErrorWithTooManyArgs(t *testing.T) {
	for _, tc := range commandsRequiringArg {
		root := NewRootCommand()
		root.SetArgs(tc.tooMany)
		err := root.Execute()
		if err == nil {
			t.Errorf("striatum %s (two args): expected error, got nil", tc.desc)
		}
	}
}

func TestCommands_AcceptOneArg(t *testing.T) {
	argsByCmd := map[string][]string{
		"install":   {"install", "some-ref-or-name", "--target", "cursor"},
		"uninstall": {"uninstall", "some-name", "--target", "cursor"},
	}
	for _, tc := range commandsRequiringArg {
		root := NewRootCommand()
		out := &bytes.Buffer{}
		root.SetOut(out)
		root.SetArgs(argsByCmd[tc.desc])
		err := root.Execute()
		if err != nil && strings.Contains(err.Error(), "accepts ") {
			t.Errorf("striatum %s: unexpected arg-count error: %v", tc.desc, err)
		}
	}
}

func TestList_HelpExitsZero(t *testing.T) {
	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"list", "--help"})
	if err := root.Execute(); err != nil {
		t.Errorf("striatum list --help: err = %v", err)
	}
}

func TestSkillSubcommand_Removed(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"skill"})
	err := root.Execute()
	if err == nil {
		t.Error("striatum skill: expected error (removed), got nil")
	}
}

func TestUnknownSubcommand_ReturnsError(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"unknown-subcommand"})
	err := root.Execute()
	if err == nil {
		t.Error("striatum unknown-subcommand: expected error, got nil")
	}
}

// TestExecute_SilenceUsage_NoUsageDumpOnRunEError asserts that with root
// silenceRootPresentation, RunE failures do not print a full Cobra usage dump.
func TestExecute_SilenceUsage_NoUsageDumpOnRunEError(t *testing.T) {
	root := NewRootCommand()
	silenceRootPresentation(root)
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"inspect", "missing-colon-so-invalid"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid reference")
	}
	combined := out.String() + errBuf.String()
	for _, needle := range []string{"Usage:", "Flags:", "Examples:"} {
		if strings.Contains(combined, needle) {
			t.Errorf("output must not contain Cobra usage marker %q; stdout+stderr:\n%s", needle, combined)
		}
	}
}
