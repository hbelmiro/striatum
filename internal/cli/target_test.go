package cli

import (
	"strings"
	"testing"
)

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{"cursor", "cursor", "cursor", ""},
		{"claude", "claude", "claude", ""},
		{"cursor with spaces", "  cursor  ", "cursor", ""},
		{"claude with spaces", "  claude  ", "claude", ""},
		{"invalid target", "vscode", "", "must be cursor or claude"},
		{"empty string", "", "", "must be cursor or claude"},
		{"whitespace only", "   ", "", "must be cursor or claude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateTarget(tt.input)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.want {
					t.Errorf("validateTarget(%q) = %q, want %q", tt.input, got, tt.want)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestInstall_ShortTargetFlag(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "install", "-t", "invalid", "localhost:5000/skills/foo:1.0.0"})
	err := root.Execute()
	if err == nil {
		t.Error("install -t invalid: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "must be cursor or claude") {
		t.Errorf("error should mention valid targets: %v", err)
	}
}

func TestUninstall_ShortTargetFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "uninstall", "-t", "invalid", "foo"})
	err := root.Execute()
	if err == nil {
		t.Error("uninstall -t invalid: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "must be cursor or claude") {
		t.Errorf("error should mention valid targets: %v", err)
	}
}

func TestSkillList_ShortTargetFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	root := NewRootCommand()
	root.SetArgs([]string{"skill", "list", "--installed", "-t", "invalid"})
	err := root.Execute()
	if err == nil {
		t.Error("skill list -t invalid: expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "must be cursor or claude") {
		t.Errorf("error should mention valid targets: %v", err)
	}
}
