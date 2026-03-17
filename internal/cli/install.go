package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Pull and install an artifact into AI coding agent skills directories",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args[0] // reference
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "not implemented yet")
			return nil
		},
	}
}
