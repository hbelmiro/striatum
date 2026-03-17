package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/spf13/cobra"
)

const defaultManifestName = "artifact.json"

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the local artifact.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			path := filepath.Join(wd, defaultManifestName)
			m, err := artifact.Load(path)
			if err != nil {
				return err
			}
			if err := artifact.Validate(m); err != nil {
				return fmt.Errorf("invalid manifest: %w", err)
			}
			if err := artifact.ValidateLocal(m, wd); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "artifact.json is valid.")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "All files referenced in spec.files exist.")
			return nil
		},
	}
	return cmd
}
