package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/spf13/cobra"
	oras "oras.land/oras-go/v2"
	orasoci "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
)

func newPullCmd() *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Download an artifact and its transitive dependencies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			if outputDir == "" {
				// Default: ./<artifact-name>/ (we don't know name until we inspect; use current dir for now)
				outputDir = wd
			}
			target, ref, err := resolveTargetAndRef(reference)
			if err != nil {
				return err
			}
			if err := oci.Pull(context.Background(), target, ref, outputDir); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pulled to", outputDir)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory (default: current directory)")
	return cmd
}

// resolveTargetAndRef parses reference and returns a read-only target and the ref to resolve (tag).
// Supports "oci:/path:tag" for local layout (tag may contain colons, e.g. "name:1.0.0") and "host/repo/name:tag" for remote.
func resolveTargetAndRef(reference string) (oras.ReadOnlyTarget, string, error) {
	if strings.HasPrefix(reference, "oci:") {
		rest := reference[len("oci:"):]
		i := strings.Index(rest, ":")
		if i < 0 {
			return nil, "", fmt.Errorf("invalid oci reference %q: missing tag", reference)
		}
		layoutPath, tag := rest[:i], rest[i+1:]
		store, err := orasoci.New(layoutPath)
		if err != nil {
			return nil, "", fmt.Errorf("open OCI layout: %w", err)
		}
		return store, tag, nil
	}
	i := strings.LastIndex(reference, ":")
	if i < 0 {
		return nil, "", fmt.Errorf("invalid reference %q: missing tag", reference)
	}
	repo, tag := reference[:i], reference[i+1:]
	reg, err := remote.NewRepository(repo)
	if err != nil {
		return nil, "", fmt.Errorf("create repository: %w", err)
	}
	return reg, tag, nil
}
