package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/registry"
	gitbackend "github.com/hbelmiro/striatum/pkg/registry/git"
	"github.com/hbelmiro/striatum/pkg/selfcontained"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
)

func newInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "inspect",
		Short:   "Display manifest and metadata of a remote artifact",
		Long:    "Fetches and prints the artifact manifest (name, version, dependencies) without downloading layers. Reference can be a registry, oci:/path:tag, or git:URL@ref.",
		Example: "  striatum inspect localhost:5000/skills/my-skill:1.0.0\n  striatum inspect git:https://github.com/example/skill.git@v1.0.0",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := strings.TrimSpace(args[0])

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
	if digest != "" && !artifact.IsValidDigest(digest) {
		return fmt.Errorf("invalid digest %q: must match sha256:<64 lowercase hex chars>", digest)
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
	ociManifest, m, configBytes, err := oci.LoadImage(cmd.Context(), target, inspectRef)
	if err != nil {
		return fmt.Errorf("inspect artifact manifest and metadata: %w", err)
	}
	printManifest(cmd.OutOrStdout(), m, digest, "")
	printSelfContainedSection(cmd, target, ociManifest, configBytes)
	return nil
}

// printSelfContainedSection appends the self-contained image provenance summary
// when the referenced image is self-contained. The OCI manifest and config
// blob are the ones already fetched by inspectOCI; provenance fetch or parse
// failures produce a stderr warning instead of a silently truncated report.
func printSelfContainedSection(cmd *cobra.Command, target oras.ReadOnlyTarget, ociManifest ocispec.Manifest, configBytes []byte) {
	if !selfcontained.IsMarked(configBytes) {
		return
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Self-contained: v1 (dev.striatum.self-contained)")
	provBytes, err := oci.FetchLayerFromManifest(cmd.Context(), target, ociManifest, selfcontained.ProvenancePath)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not fetch provenance layer: %v\n", err)
		return
	}
	var prov selfcontained.Provenance
	if err := json.Unmarshal(provBytes, &prov); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not parse provenance document: %v\n", err)
		return
	}
	if len(prov.Dependencies) > 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Bundled dependencies:")
		for _, d := range prov.Dependencies {
			src := d.ResolvedSource.Ref
			if src == "" {
				src = d.ResolvedSource.URL
			}
			if d.ResolvedSource.Digest != "" {
				src += " (" + d.ResolvedSource.Digest + ")"
			} else if d.ResolvedSource.Commit != "" {
				src += " (" + d.ResolvedSource.Commit + ")"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s %s@%s from %s\n", d.Kind, d.Name, d.Version, src)
		}
	}
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
