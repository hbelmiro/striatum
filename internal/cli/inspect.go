package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/registry"
	gitbackend "github.com/hbelmiro/striatum/pkg/registry/git"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "inspect",
		Short:   "Display manifest and metadata of a remote artifact",
		Long:    "Fetches and prints the artifact manifest (name, version, dependencies) without downloading layers. Reference can be a registry, oci:/path:tag, or git:URL@ref.",
		Example: "  striatum inspect localhost:5000/skills/my-skill:1.0.0\n  striatum inspect git:https://github.com/example/skill.git@v1.0.0",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]

			if strings.HasPrefix(reference, "git:") {
				return inspectGit(cmd, reference)
			}
			return inspectOCI(cmd, reference)
		},
	}
}

func inspectOCI(cmd *cobra.Command, reference string) error {
	target, ref, digest, err := resolveTargetAndRef(reference)
	if err != nil {
		return fmt.Errorf("resolve reference: %w", err)
	}
	if digest == "" {
		digest, err = oci.ResolveDigest(cmd.Context(), target, ref)
		if err != nil {
			return fmt.Errorf("resolve digest: %w", err)
		}
	}
	inspectRef := ref
	if digest != "" {
		inspectRef = digest
	}
	m, err := oci.Inspect(cmd.Context(), target, inspectRef)
	if err != nil {
		return fmt.Errorf("inspect artifact manifest and metadata: %w", err)
	}
	printManifest(cmd.OutOrStdout(), m, digest, "")
	return nil
}

func inspectGit(cmd *cobra.Command, reference string) error {
	loc, err := registry.ParseReference(reference)
	if err != nil {
		return fmt.Errorf("parse git reference: %w", err)
	}
	gitDep, ok := loc.(*artifact.GitDependency)
	if !ok {
		return fmt.Errorf("expected git dependency from %q", reference)
	}
	backend := &gitbackend.Backend{}
	commit, err := backend.ResolveCommit(cmd.Context(), gitDep)
	if err != nil {
		return fmt.Errorf("resolve git commit: %w", err)
	}
	gitDep.Commit = commit
	m, err := backend.Inspect(cmd.Context(), gitDep)
	if err != nil {
		return fmt.Errorf("inspect git artifact: %w", err)
	}
	printManifest(cmd.OutOrStdout(), m, "", commit)
	return nil
}

func printManifest(w io.Writer, m *artifact.Manifest, digest, commit string) {
	_, _ = fmt.Fprintf(w, "Name:         %s\n", m.Metadata.Name)
	_, _ = fmt.Fprintf(w, "Version:      %s\n", m.Metadata.Version)
	_, _ = fmt.Fprintf(w, "Kind:         %s\n", m.Kind)
	if digest != "" {
		_, _ = fmt.Fprintf(w, "Digest:       %s\n", digest)
	}
	if commit != "" {
		_, _ = fmt.Fprintf(w, "Commit:       %s\n", commit)
	}
	if m.Metadata.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:  %s\n", m.Metadata.Description)
	}
	_, _ = fmt.Fprintf(w, "Entrypoint:   %s\n", m.Spec.Entrypoint)
	_, _ = fmt.Fprintf(w, "Files:        %s\n", strings.Join(m.Spec.Files, ", "))
	if len(m.Dependencies) > 0 {
		_, _ = fmt.Fprintln(w, "Dependencies:")
		for _, d := range m.Dependencies {
			_, _ = fmt.Fprintf(w, "  - %s [%s]\n", d.CanonicalRef(), d.Source())
		}
	}
}
