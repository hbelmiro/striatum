package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var name, version, kind, entrypoint string
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Scaffold an artifact.json in the current directory",
		Long:    "Creates an artifact.json in the current directory with the given name, version, and kind. Requires --name, --kind, and --entrypoint.",
		Example: "  striatum init --name my-skill --kind Skill --entrypoint SKILL.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("artifact name is required (use --name)")
			}
			if strings.TrimSpace(kind) == "" {
				return fmt.Errorf("artifact kind is required (use --kind)")
			}
			if !artifact.IsSupportedKind(kind) {
				return fmt.Errorf("unsupported kind %q", kind)
			}
			if strings.TrimSpace(entrypoint) == "" {
				return fmt.Errorf("entrypoint is required (use --entrypoint)")
			}
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			m := &artifact.Manifest{
				APIVersion: "striatum.dev/v1alpha1",
				Kind:       kind,
				Metadata:   artifact.Metadata{Name: name, Version: version},
				Spec:       artifact.Spec{Entrypoint: entrypoint, Files: []string{entrypoint}},
			}
			path := filepath.Join(wd, "artifact.json")
			data, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal manifest: %w", err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				return fmt.Errorf("write artifact.json: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Created artifact.json")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Artifact name (required)")
	cmd.Flags().StringVar(&version, "version", "0.1.0", "Artifact version")
	cmd.Flags().StringVar(&kind, "kind", "", "Artifact kind (required, e.g. Skill)")
	cmd.Flags().StringVar(&entrypoint, "entrypoint", "", "Entrypoint file (required)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("kind")
	_ = cmd.MarkFlagRequired("entrypoint")
	return cmd
}
