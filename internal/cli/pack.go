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

func newPackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pack",
		Short:   "Bundle the artifact into a local OCI Image Layout directory",
		Long:    "Reads artifact.json and spec.files from the current directory and writes an OCI Image Layout to .striatum/oci-layout/ for push or local use.",
		Example: "  striatum pack",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			manifestPath := filepath.Join(wd, defaultManifestName)
			m, err := artifact.Load(manifestPath)
			if err != nil {
				return err
			}
			layoutPath := filepath.Join(wd, defaultLayoutDir)
			if err := os.MkdirAll(layoutPath, 0o755); err != nil {
				return fmt.Errorf("create layout dir: %w", err)
			}
			if err := oci.Pack(m, wd, layoutPath); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Packed artifact to %s/\n", defaultLayoutDir)
			return nil
		},
	}
	return cmd
}
