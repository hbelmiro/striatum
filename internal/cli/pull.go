package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/resolver"
	"github.com/spf13/cobra"
	oras "oras.land/oras-go/v2"
	orasoci "oras.land/oras-go/v2/content/oci"
)

func newPullCmd() *cobra.Command {
	var outputDir string
	var registry string
	var noCache bool
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Download an artifact and its transitive dependencies",
		Long: `Downloads the artifact and all dependencies into the output directory (default: current working directory).
Reference can be a registry (host/repo/name:tag) or oci:/path:tag.
Each artifact is placed in a subdirectory named after the artifact (<output>/<name>/).

By default, artifacts are also stored under the Striatum cache (STRIATUM_HOME or ~/.striatum/cache), the same layout used by "skill install", so "skill list" can show pulled skills. Use --no-cache to write only to the output directory.`,
		Example: "  striatum pull localhost:5000/skills/my-skill:1.0.0\n  striatum pull -o ./out oci:./build:my-skill:1.0.0",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			target, ref, err := resolveTargetAndRef(reference)
			if err != nil {
				return fmt.Errorf("resolve reference: %w", err)
			}
			ctx := cmd.Context()
			rootManifest, err := oci.Inspect(ctx, target, ref)
			if err != nil {
				return fmt.Errorf("read artifact manifest: %w", err)
			}
			outputDir = strings.TrimSpace(outputDir)
			if outputDir == "" {
				outputDir = wd
			} else {
				outputDir = filepath.Clean(outputDir)
				if !filepath.IsAbs(outputDir) {
					outputDir = filepath.Join(wd, outputDir)
				}
			}
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}

			isOCI := strings.HasPrefix(reference, "oci:")
			if isOCI && len(rootManifest.Dependencies) > 0 && strings.TrimSpace(registry) == "" {
				return fmt.Errorf("pull with oci: reference and dependencies requires --registry")
			}

			defaultRegistry := deriveDefaultRegistry(reference)
			if isOCI {
				defaultRegistry = strings.TrimSpace(registry)
			}

			var resolved []resolver.ResolvedArtifact
			if len(rootManifest.Dependencies) == 0 {
				resolved = []resolver.ResolvedArtifact{{
					Name:     rootManifest.Metadata.Name,
					Version:  rootManifest.Metadata.Version,
					Registry: defaultRegistry,
					Manifest: rootManifest,
				}}
			} else {
				fetcher := NewRemoteFetcher()
				if !noCache {
					fetcher = NewCacheFirstFetcher(fetcher)
				}
				var resolveErr error
				resolved, resolveErr = resolver.Resolve(ctx, rootManifest, defaultRegistry, fetcher)
				if resolveErr != nil {
					return fmt.Errorf("resolving dependencies: %w", resolveErr)
				}
			}

			if noCache {
				for i, r := range resolved {
					var pullTarget oras.ReadOnlyTarget
					pullRef := r.Version
					if i == 0 {
						pullTarget = target
						pullRef = ref
					} else {
						repo := strings.TrimSuffix(r.Registry, "/") + "/" + r.Name
						reg, repoErr := oci.NewRepository(repo)
						if repoErr != nil {
							return fmt.Errorf("create repository for %s: %w", r.Name, repoErr)
						}
						pullTarget = reg
					}
					if err := oci.Pull(ctx, pullTarget, pullRef, outputDir); err != nil {
						return fmt.Errorf("pull %s@%s: %w", r.Name, r.Version, err)
					}
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pulled to", outputDir)
				return nil
			}

			if err := ensureArtifactsInCache(ctx, reference, target, ref, resolved); err != nil {
				return err
			}
			for _, r := range resolved {
				cacheDir := installer.CacheDir(r.Name, r.Version)
				if err := installer.InstallToTarget(cacheDir, outputDir, r.Name); err != nil {
					return fmt.Errorf("copy %s to output: %w", r.Name, err)
				}
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pulled to", outputDir)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory (default: current working directory)")
	cmd.Flags().StringVar(&registry, "registry", "", "Registry base URL (required for oci: reference when root has dependencies)")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Do not write to the Striatum cache; only populate the output directory")
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
func resolveTargetAndRef(reference string) (oras.ReadOnlyTarget, string, error) {
	if strings.HasPrefix(reference, "oci:") {
		layoutPath, tag, err := oci.SplitReference(reference)
		if err != nil {
			return nil, "", err
		}
		store, err := orasoci.New(layoutPath)
		if err != nil {
			return nil, "", fmt.Errorf("open OCI layout: %w", err)
		}
		return store, tag, nil
	}
	repo, tag, err := oci.SplitReference(reference)
	if err != nil {
		return nil, "", err
	}
	reg, err := oci.NewRepository(repo)
	if err != nil {
		return nil, "", fmt.Errorf("create repository: %w", err)
	}
	return reg, tag, nil
}
