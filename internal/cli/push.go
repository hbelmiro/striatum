package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "push",
		Short:   "Push the artifact to an OCI registry",
		Long:    "Packs the current artifact (artifact.json and spec.files) and pushes it to the given reference (e.g. localhost:5000/repo/my-skill:1.0.0).",
		Example: "  striatum push localhost:5000/skills/my-skill:1.0.0",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			manifestPath := filepath.Join(wd, defaultManifestName)
			m, err := artifact.Load(manifestPath)
			if err != nil {
				return fmt.Errorf("load manifest: %w", err)
			}
			if err := artifact.Validate(m); err != nil {
				return fmt.Errorf("invalid manifest: %w", err)
			}
			if err := artifact.ValidateLocal(m, wd); err != nil {
				return fmt.Errorf("validate local files: %w", err)
			}
			if err := oci.Push(cmd.Context(), m, wd, reference); err != nil {
				return fmt.Errorf("push artifact: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pushed to", reference)
			return nil
		},
	}
}
