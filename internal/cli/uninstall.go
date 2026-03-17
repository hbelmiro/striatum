package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove a previously installed skill and orphaned dependencies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args[0] // name
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "not implemented yet")
			return nil
		},
	}
}
