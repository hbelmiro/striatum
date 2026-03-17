package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Push the artifact to an OCI registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			manifestPath := filepath.Join(wd, defaultManifestName)
			m, err := artifact.Load(manifestPath)
			if err != nil {
				return err
			}
			if err := artifact.Validate(m); err != nil {
				return fmt.Errorf("invalid manifest: %w", err)
			}
			if err := artifact.ValidateLocal(m, wd); err != nil {
				return err
			}
			if err := oci.Push(context.Background(), m, wd, reference); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pushed to", reference)
			return nil
		},
	}
}
