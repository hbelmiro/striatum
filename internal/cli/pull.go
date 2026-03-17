package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Download an artifact and its transitive dependencies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args[0] // reference
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "not implemented yet")
			return nil
		},
	}
}
