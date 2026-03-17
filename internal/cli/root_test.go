package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommand_PrintsUsage(t *testing.T) {
	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() err = %v, want nil", err)
	}
	if got := out.String(); !strings.Contains(got, "Usage") {
		t.Errorf("output %q does not contain Usage", got)
	}
}

func TestRootCommand_VersionFlag(t *testing.T) {
	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() err = %v, want nil", err)
	}
	got := out.String()
	if !strings.Contains(got, "striatum") {
		t.Errorf("output %q does not contain striatum", got)
	}
	if !strings.Contains(got, version) {
		t.Errorf("output %q does not contain version %q", got, version)
	}
}

func TestRootCommand_HelpExitsZero(t *testing.T) {
	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() err = %v, want nil", err)
	}
	if got := out.String(); !strings.Contains(got, "Usage") {
		t.Errorf("output %q does not contain Usage", got)
	}
}
