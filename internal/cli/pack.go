package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/spf13/cobra"
)

const defaultLayoutDir = "build"

func newPackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Bundle the artifact into a local OCI Image Layout directory (default <project>/build/; override with -o / --output)",
		Long: `Reads artifact.json and spec.files from the manifest’s project directory and writes an OCI Image Layout for push or local use.

By default the layout is written to <project>/build/. Use -o / --output to set a different directory; paths are relative to the shell’s current working directory (same as striatum pull --output).`,
		Example: "  striatum pack\n  striatum pack -f packages/my-skill\n  striatum pack -o ./dist\n  striatum pack -f packages/my-skill -o /tmp/my-layout",
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestFlag, err := cmd.Flags().GetString("manifest")
			if err != nil {
				return err
			}
			outputFlag, err := cmd.Flags().GetString("output")
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
			var layoutPath string
			if trimmed := strings.TrimSpace(outputFlag); trimmed == "" {
				layoutPath = filepath.Join(projectRoot, defaultLayoutDir)
			} else {
				layoutPath, err = filepath.Abs(filepath.Clean(trimmed))
				if err != nil {
					return fmt.Errorf("resolve output path: %w", err)
				}
			}
			if err := os.MkdirAll(layoutPath, 0o755); err != nil {
				return fmt.Errorf("create layout dir: %w", err)
			}
			if err := oci.Pack(cmd.Context(), m, projectRoot, layoutPath); err != nil {
				return fmt.Errorf("pack artifact: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Packed artifact to %s\n", layoutPath)
			return nil
		},
	}
	cmd.Flags().StringP("manifest", "f", "", manifestFlagUsage)
	cmd.Flags().StringP("output", "o", "", "OCI layout output directory (default: <project>/build/; relative paths use the current working directory)")
	return cmd
}
