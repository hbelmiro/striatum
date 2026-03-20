package cli

import (
	"fmt"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/resolver"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var checkDeps bool
	var registry string
	cmd := &cobra.Command{
		Use:     "validate",
		Short:   "Validate the local artifact.json",
		Long:    "Validates schema and that all spec.files exist (paths are relative to the manifest file's directory). Use --check-deps with --registry to verify dependencies resolve in the registry.",
		Example: "  striatum validate\n  striatum validate -f path/to/artifact.json\n  striatum validate --check-deps --registry localhost:5000/skills",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Manifest is valid.")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "All files referenced in spec.files exist.")

			if checkDeps {
				registry = strings.TrimSpace(registry)
				if registry == "" {
					return fmt.Errorf("--registry is required when using --check-deps")
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Resolving dependency tree...")
				fetcher := NewRemoteFetcher()
				resolved, err := resolver.Resolve(cmd.Context(), m, registry, fetcher)
				if err != nil {
					return fmt.Errorf("resolving dependencies: %w", err)
				}
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
	cmd.Flags().StringP("manifest", "f", "", manifestFlagUsage)
	return cmd
}
