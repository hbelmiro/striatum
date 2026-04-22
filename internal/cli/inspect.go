package cli

import (
	"fmt"
	"strings"

	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "inspect",
		Short:   "Display manifest and metadata of a remote artifact",
		Long:    "Fetches and prints the artifact manifest (name, version, dependencies) without downloading layers. Reference can be a registry or oci:/path:tag.",
		Example: "  striatum inspect localhost:5000/skills/my-skill:1.0.0",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]
			target, ref, err := resolveTargetAndRef(reference)
			if err != nil {
				return fmt.Errorf("resolve reference: %w", err)
			}
			m, err := oci.Inspect(cmd.Context(), target, ref)
			if err != nil {
				return fmt.Errorf("inspect artifact manifest and metadata: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Name:         %s\n", m.Metadata.Name)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Version:      %s\n", m.Metadata.Version)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Kind:         %s\n", m.Kind)
			if m.Metadata.Description != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Description:  %s\n", m.Metadata.Description)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Entrypoint:   %s\n", m.Spec.Entrypoint)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Files:        %s\n", strings.Join(m.Spec.Files, ", "))
			if len(m.Dependencies) > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Dependencies:")
				for _, d := range m.Dependencies {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s [%s]\n", d.CanonicalRef(), d.Source())
				}
			}
			return nil
		},
	}
}
