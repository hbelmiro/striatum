package cli

import (
	"bytes"
	"strings"
	"testing"
)

var subcommands = []string{"init", "validate", "pack", "push", "pull", "install", "uninstall", "inspect", "skill"}

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

var commandsRequiringArg = []string{"install", "uninstall"}

func TestCommandsRequiringArg_ErrorWithoutArg(t *testing.T) {
	for _, name := range commandsRequiringArg {
		root := NewRootCommand()
		root.SetArgs([]string{name})
		err := root.Execute()
		if err == nil {
			t.Errorf("striatum %s (no arg): expected error, got nil", name)
		}
	}
}

func TestCommandsRequiringArg_ErrorWithTooManyArgs(t *testing.T) {
	for _, name := range commandsRequiringArg {
		root := NewRootCommand()
		root.SetArgs([]string{name, "first", "second"})
		err := root.Execute()
		if err == nil {
			t.Errorf("striatum %s (two args): expected error, got nil", name)
		}
	}
}

func TestCommandsRequiringArg_AcceptOneArg(t *testing.T) {
	argsByCmd := map[string][]string{
		"install":   {"install", "some-ref-or-name", "--target", "cursor"},
		"uninstall": {"uninstall", "some-name", "--target", "cursor"},
	}
	for _, name := range commandsRequiringArg {
		root := NewRootCommand()
		out := &bytes.Buffer{}
		root.SetOut(out)
		root.SetArgs(argsByCmd[name])
		err := root.Execute()
		// Should not fail with "accepts 1 arg(s)" (wrong arg count); other errors (e.g. invalid ref, not installed) are ok
		if err != nil && strings.Contains(err.Error(), "accepts ") {
			t.Errorf("striatum %s: unexpected arg-count error: %v", name, err)
		}
	}
}

func TestSkillList_HelpExitsZero(t *testing.T) {
	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"skill", "list", "--help"})
	if err := root.Execute(); err != nil {
		t.Errorf("striatum skill list --help: err = %v", err)
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
