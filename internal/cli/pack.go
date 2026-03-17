package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pack",
		Short: "Bundle the artifact into a local OCI Image Layout directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "not implemented yet")
			return nil
		},
	}
}
