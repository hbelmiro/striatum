package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

// NewRootCommand returns the root cobra command for the striatum CLI.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "striatum",
		Short: "OCI-native CLI for packaging and distributing AI artifacts",
		Long:  "Striatum packages, versions, and distributes AI artifacts (skills, prompts, RAG configs) using OCI-compliant registries.",
	}
	cmd.Version = version
	cmd.SetVersionTemplate("striatum version {{.Version}}\n")
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newPackCmd())
	cmd.AddCommand(newPushCmd())
	cmd.AddCommand(newPullCmd())
	cmd.AddCommand(newInspectCmd())
	cmd.AddCommand(newSkillCmd())
	return cmd
}

// silenceRootPresentation matches Execute: no usage dump on RunE errors, errors returned not printed by Cobra.
func silenceRootPresentation(cmd *cobra.Command) {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
}

// Execute runs the root command. It is called from main.
func Execute() {
	cmd := NewRootCommand()
	silenceRootPresentation(cmd)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
