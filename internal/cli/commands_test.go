package cli

import (
	"bytes"
	"strings"
	"testing"
)

var subcommands = []string{"init", "validate", "pack", "push", "pull", "inspect", "skill"}

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

var skillSubcommandsRequiringArg = []struct {
	args    []string
	desc    string
	tooMany []string
}{
	{
		args:    []string{"skill", "install"},
		desc:    "skill install",
		tooMany: []string{"skill", "install", "first", "second"},
	},
	{
		args:    []string{"skill", "uninstall"},
		desc:    "skill uninstall",
		tooMany: []string{"skill", "uninstall", "first", "second"},
	},
}

func TestSkillSubcommands_ErrorWithoutArg(t *testing.T) {
	for _, tc := range skillSubcommandsRequiringArg {
		root := NewRootCommand()
		root.SetArgs(tc.args)
		err := root.Execute()
		if err == nil {
			t.Errorf("striatum %s (no arg): expected error, got nil", tc.desc)
		}
	}
}

func TestSkillSubcommands_ErrorWithTooManyArgs(t *testing.T) {
	for _, tc := range skillSubcommandsRequiringArg {
		root := NewRootCommand()
		root.SetArgs(tc.tooMany)
		err := root.Execute()
		if err == nil {
			t.Errorf("striatum %s (two args): expected error, got nil", tc.desc)
		}
	}
}

func TestSkillSubcommands_AcceptOneArg(t *testing.T) {
	argsByCmd := map[string][]string{
		"skill install":   {"skill", "install", "some-ref-or-name", "--target", "cursor"},
		"skill uninstall": {"skill", "uninstall", "some-name", "--target", "cursor"},
	}
	for _, tc := range skillSubcommandsRequiringArg {
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
