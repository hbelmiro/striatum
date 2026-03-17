package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/resolver"
	"github.com/spf13/cobra"
	oras "oras.land/oras-go/v2"
	orasoci "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
)

func newPullCmd() *cobra.Command {
	var outputDir string
	var registry string
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
			target, ref, err := resolveTargetAndRef(reference)
			if err != nil {
				return err
			}
			ctx := context.Background()
			rootManifest, err := oci.Inspect(ctx, target, ref)
			if err != nil {
				return fmt.Errorf("inspect artifact: %w", err)
			}
			if outputDir == "" {
				outputDir = filepath.Join(wd, rootManifest.Metadata.Name)
			}
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}

			isOCI := strings.HasPrefix(reference, "oci:")
			if isOCI && len(rootManifest.Dependencies) > 0 && strings.TrimSpace(registry) == "" {
				return fmt.Errorf("pull with oci: reference and dependencies requires --registry")
			}

			if len(rootManifest.Dependencies) == 0 {
				if err := oci.Pull(ctx, target, ref, outputDir); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pulled to", outputDir)
				return nil
			}

			defaultRegistry := deriveDefaultRegistry(reference)
			if isOCI {
				defaultRegistry = strings.TrimSpace(registry)
			}
			fetcher := NewRemoteFetcher()
			resolved, err := resolver.Resolve(ctx, rootManifest, defaultRegistry, fetcher)
			if err != nil {
				return fmt.Errorf("resolving dependencies: %w", err)
			}
			for i, r := range resolved {
				var pullTarget oras.ReadOnlyTarget
				pullRef := r.Version
				if i == 0 {
					pullTarget = target
					pullRef = ref
				} else {
					repo := strings.TrimSuffix(r.Registry, "/") + "/" + r.Name
					reg, err := remote.NewRepository(repo)
					if err != nil {
						return fmt.Errorf("create repository for %s: %w", r.Name, err)
					}
					pullTarget = reg
				}
				if err := oci.Pull(ctx, pullTarget, pullRef, outputDir); err != nil {
					return fmt.Errorf("pull %s@%s: %w", r.Name, r.Version, err)
				}
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pulled to", outputDir)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory (default: ./<root-name>/)")
	cmd.Flags().StringVar(&registry, "registry", "", "Registry base URL (required for oci: reference when root has dependencies)")
	return cmd
}

// deriveDefaultRegistry returns the registry base from a remote reference (host/repo/name:tag -> host/repo).
func deriveDefaultRegistry(reference string) string {
	if strings.HasPrefix(reference, "oci:") {
		return ""
	}
	i := strings.LastIndex(reference, ":")
	if i < 0 {
		return ""
	}
	repoPart := reference[:i]
	j := strings.LastIndex(repoPart, "/")
	if j < 0 {
		return repoPart
	}
	return repoPart[:j]
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
