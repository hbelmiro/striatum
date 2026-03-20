package cli

import (
	"fmt"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "push",
		Short:   "Push the artifact to an OCI registry",
		Long:    "Packs the artifact (artifact.json and spec.files from the manifest's project directory) and pushes it to the given reference (e.g. localhost:5000/repo/my-skill:1.0.0).",
		Example: "  striatum push localhost:5000/skills/my-skill:1.0.0\n  striatum push -f path/to/artifact.json localhost:5000/skills/my-skill:1.0.0",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]
			manifestFlag, err := cmd.Flags().GetString("manifest")
			if err != nil {
				return err
			}
			manifestPath, projectRoot, err := resolveManifestAndProjectRoot(manifestFlag)
			if err != nil {
				return err
			}
			m, err := artifact.Load(manifestPath)
			if err != nil {
				return fmt.Errorf("load manifest: %w", err)
			}
			if err := artifact.Validate(m); err != nil {
				return fmt.Errorf("invalid manifest: %w", err)
			}
			if err := artifact.ValidateLocal(m, projectRoot); err != nil {
				return fmt.Errorf("validate local files: %w", err)
			}
			if err := oci.Push(cmd.Context(), m, projectRoot, reference); err != nil {
				return fmt.Errorf("push artifact: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pushed to", reference)
			return nil
		},
	}
	cmd.Flags().StringP("manifest", "f", "", manifestFlagUsage)
	return cmd
}
