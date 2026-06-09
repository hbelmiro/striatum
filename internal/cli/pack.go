package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/resolver"
	"github.com/spf13/cobra"
)

const defaultLayoutDir = "build"

func newPackCmd() *cobra.Command {
	var manifestFlag, outputFlag string
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Bundle the artifact into a local OCI Image Layout directory (default <project>/build/; override with -o / --output)",
		Long: `Reads artifact.json and spec.files from the manifest’s project directory and writes an OCI Image Layout for push or local use.

By default the layout is written to <project>/build/. Use -o / --output to set a different directory; paths are relative to the shell’s current working directory (same as striatum pull --output).`,
		Example: "  striatum pack\n  striatum pack -f packages/my-skill\n  striatum pack -o ./dist\n  striatum pack -f packages/my-skill -o /tmp/my-layout",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			depFiles, err := resolvePromptDeps(cmd.Context(), m)
			if err != nil {
				return err
			}
			if err := oci.Pack(cmd.Context(), m, projectRoot, layoutPath, depFiles...); err != nil {
				return fmt.Errorf("pack artifact: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Packed artifact to %s\n", layoutPath)
			return nil
		},
	}
	cmd.Flags().StringVarP(&manifestFlag, "manifest", "f", "", manifestFlagUsage)
	cmd.Flags().StringVarP(&outputFlag, "output", "o", "", "OCI layout output directory (default: <project>/build/; relative paths use the current working directory)")
	return cmd
}

func resolvePromptDeps(ctx context.Context, m *artifact.Manifest) ([]oci.DepFile, error) {
	if m.Kind != "Workflow" || len(m.Dependencies) == 0 {
		return nil, nil
	}
	fetcher := NewCacheFirstFetcher(NewRemoteFetcher())
	resolved, err := resolver.Resolve(ctx, m, fetcher)
	if err != nil {
		return nil, fmt.Errorf("resolve dependencies: %w", err)
	}
	var depFiles []oci.DepFile
	for _, r := range resolved[1:] {
		if r.Manifest == nil || r.Manifest.Kind != "Prompt" {
			continue
		}
		cacheDir := installer.CacheDir(r.Name, r.Version)
		for _, f := range r.Manifest.Spec.Files {
			depFiles = append(depFiles, oci.DepFile{
				AnnotationPath: fmt.Sprintf("deps/%s/%s", r.Name, f),
				DiskPath:       filepath.Join(cacheDir, f),
			})
		}
	}
	return depFiles, nil
}
