package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/spf13/cobra"
)

const defaultLayoutDir = ".striatum/oci-layout"

func newPackCmd(manifest *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pack",
		Short:   "Bundle the artifact into a local OCI Image Layout directory",
		Long:    "Reads artifact.json and spec.files from the manifest's project directory and writes an OCI Image Layout to <project>/.striatum/oci-layout/ for push or local use.",
		Example: "  striatum pack\n  striatum pack -f packages/my-skill",
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestPath, projectRoot, err := resolveManifestAndProjectRoot(*manifest)
			if err != nil {
				return err
			}
			m, err := artifact.Load(manifestPath)
			if err != nil {
				return fmt.Errorf("load manifest: %w", err)
			}
			layoutPath := filepath.Join(projectRoot, defaultLayoutDir)
			if err := os.MkdirAll(layoutPath, 0o755); err != nil {
				return fmt.Errorf("create layout dir: %w", err)
			}
			if err := oci.Pack(cmd.Context(), m, projectRoot, layoutPath); err != nil {
				return fmt.Errorf("pack artifact: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Packed artifact to %s/\n", defaultLayoutDir)
			return nil
		},
	}
	cmd.Flags().StringVarP(manifest, "manifest", "f", "", manifestFlagUsage)
	return cmd
}
