package cli

import (
	"bytes"
	"testing"
)

var subcommands = []string{"init", "validate", "pack", "push", "pull", "install", "uninstall", "inspect"}

func TestSubcommands_Registered(t *testing.T) {
	root := NewRootCommand()
	for _, name := range subcommands {
		cmd, _, _ := root.Find([]string{name})
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
	for _, name := range commandsRequiringArg {
		root := NewRootCommand()
		out := &bytes.Buffer{}
		root.SetOut(out)
		root.SetArgs([]string{name, "some-ref-or-name"})
		err := root.Execute()
		if err != nil {
			t.Errorf("striatum %s some-ref: err = %v", name, err)
		}
		if got := out.String(); got != "not implemented yet\n" {
			t.Errorf("striatum %s: output = %q, want %q", name, got, "not implemented yet\n")
		}
	}
}

// commandsWithNoRequiredArg lists subcommands that take no required args and still show stub output.
// init, validate, and pack are implemented and have their own tests.
var commandsWithNoRequiredArg = []string{}

func TestCommandsWithNoRequiredArg_RunExitsZero(t *testing.T) {
	for _, name := range commandsWithNoRequiredArg {
		root := NewRootCommand()
		out := &bytes.Buffer{}
		root.SetOut(out)
		root.SetArgs([]string{name})
		err := root.Execute()
		if err != nil {
			t.Errorf("striatum %s: err = %v", name, err)
		}
		if got := out.String(); got != "not implemented yet\n" {
			t.Errorf("striatum %s: output = %q, want %q", name, got, "not implemented yet\n")
		}
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
