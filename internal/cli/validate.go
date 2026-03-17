package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/resolver"
	"github.com/spf13/cobra"
)

const defaultManifestName = "artifact.json"

func newValidateCmd() *cobra.Command {
	var checkDeps bool
	var registry string
	cmd := &cobra.Command{
		Use:     "validate",
		Short:   "Validate the local artifact.json",
		Long:    "Validates schema and that all spec.files exist. Use --check-deps with --registry to verify dependencies resolve in the registry.",
		Example: "  striatum validate\n  striatum validate --check-deps --registry localhost:5000/skills",
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

			if checkDeps {
				registry = strings.TrimSpace(registry)
				if registry == "" {
					return fmt.Errorf("--registry is required when using --check-deps")
				}
				fetcher := NewRemoteFetcher()
				resolved, err := resolver.Resolve(context.Background(), m, registry, fetcher)
				if err != nil {
					return fmt.Errorf("resolving dependencies: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Resolving dependency tree…")
				directNames := make(map[string]bool)
				for _, d := range m.Dependencies {
					directNames[d.Name+"@"+d.Version] = true
				}
				for _, r := range resolved {
					suffix := ""
					if !directNames[r.Name+"@"+r.Version] && (r.Name != m.Metadata.Name || r.Version != m.Metadata.Version) {
						suffix = " (transitive)"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %s@%s%s\n", r.Name, r.Version, suffix)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "All dependencies resolved.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkDeps, "check-deps", false, "Verify all dependencies exist in the registry")
	cmd.Flags().StringVar(&registry, "registry", "", "Registry base URL (required with --check-deps, e.g. localhost:5000/skills)")
	return cmd
}
