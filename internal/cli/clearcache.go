package cli

import (
	"fmt"

	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/spf13/cobra"
)

func newClearCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clear-cache",
		Short:   "Clear the local artifact cache",
		Long:    "Remove the entire local artifact cache (~/.striatum/cache), including self-contained image payloads. Installed artifacts are not affected.",
		Example: "  striatum clear-cache",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClearCache(cmd)
		},
	}
	return cmd
}

func runClearCache(cmd *cobra.Command) error {
	removed, err := installer.ClearCache()
	if err != nil {
		return fmt.Errorf("clear cache: %w", err)
	}
	out := cmd.OutOrStdout()
	if removed == 1 {
		_, _ = fmt.Fprintln(out, "Removed 1 cache entry")
		return nil
	}
	_, _ = fmt.Fprintf(out, "Removed %d cache entries\n", removed)
	return nil
}
