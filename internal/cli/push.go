package cli

import (
	"fmt"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	var manifestFlag string
	var selfContained, printDigest bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push the artifact to an OCI registry",
		Long: `Packs the artifact (artifact.json and spec.files from the manifest's project directory) and pushes it to the given registry reference (e.g. localhost:5000/repo/my-skill:1.0.0). Push is registry-only: oci: layout references are rejected; use "striatum pack -o <dir>" to write a local OCI layout.

Use --self-contained to publish one image bundling the root Skill with its complete Skill and Prompt dependency closure, with artifact-owned references rewritten to resolve inside the image. --self-contained re-runs the same preparation as "striatum pack --self-contained" from the manifest; push never consumes a previously packed layout.

The default output is human-facing and may change between releases; it is not a stable application interface. Use --print-digest for a stable machine-readable contract: on success exactly one canonical reference line (registry/repository@sha256:...) is written to stdout, diagnostics go to stderr, and a failed push writes no successful reference to stdout.`,
		Example: "  striatum push localhost:5000/skills/my-skill:1.0.0\n  striatum push -f path/to/artifact.json localhost:5000/skills/my-skill:1.0.0\n  striatum push --self-contained --print-digest localhost:5000/skills/my-skill:999-SNAPSHOT",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]
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
			var desc ocispec.Descriptor
			if selfContained {
				p, err := prepareSelfContained(cmd.Context(), m, projectRoot)
				if err != nil {
					return fmt.Errorf("prepare self-contained image: %w", err)
				}
				defer p.Cleanup()
				pi, err := preparedImageFrom(m, p)
				if err != nil {
					return err
				}
				desc, err = oci.PushPrepared(cmd.Context(), pi, reference)
				if err != nil {
					return fmt.Errorf("push self-contained image: %w", err)
				}
			} else {
				desc, err = oci.Push(cmd.Context(), m, projectRoot, reference)
				if err != nil {
					return fmt.Errorf("push artifact: %w", err)
				}
			}
			if printDigest {
				repo, _, _, err := oci.SplitReference(reference)
				if err != nil {
					return fmt.Errorf("parse reference: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), repo+"@"+desc.Digest.String())
				return nil
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pushed to", reference)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Digest:", desc.Digest.String())
			return nil
		},
	}
	cmd.Flags().StringVarP(&manifestFlag, "manifest", "f", "", manifestFlagUsage)
	cmd.Flags().BoolVar(&selfContained, "self-contained", false, "Publish one image bundling the Skill with its complete dependency closure")
	cmd.Flags().BoolVar(&printDigest, "print-digest", false, "Print one canonical registry/repository@sha256:... line to stdout on success (machine-readable; diagnostics on stderr)")
	return cmd
}
