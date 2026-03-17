package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/resolver"
	"github.com/spf13/cobra"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
)

func newInstallCmd() *cobra.Command {
	var target, projectPath, registry string
	var force, reinstallAll bool
	cmd := &cobra.Command{
		Use:     "install",
		Short:   "Pull and install an artifact into AI coding agent skills directories",
		Long:    "Resolves the artifact and dependencies, copies them to the install target (Cursor or Claude skills dir). Requires --target (cursor or claude). Use --project for project-level install.",
		Example: "  striatum install --target cursor localhost:5000/skills/my-skill:1.0.0",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reinstallAll {
				return runReinstallAll(cmd)
			}
			reference := args[0]
			target = strings.TrimSpace(target)
			if target == "" {
				return fmt.Errorf("--target is required (cursor or claude)")
			}
			if target != "cursor" && target != "claude" {
				return fmt.Errorf("--target must be cursor or claude, got %q", target)
			}
			return runInstall(cmd, reference, target, strings.TrimSpace(projectPath), strings.TrimSpace(registry), force)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Install target: cursor or claude (required)")
	cmd.Flags().StringVar(&projectPath, "project", "", "Project path for project-level install (e.g. .)")
	cmd.Flags().StringVar(&registry, "registry", "", "Registry base (required for oci: reference when root has dependencies)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite conflicting versions")
	cmd.Flags().BoolVar(&reinstallAll, "reinstall-all", false, "Replay all entries from install DB")
	return cmd
}

func runReinstallAll(cmd *cobra.Command) error {
	entries, err := installer.LoadInstalled()
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []installer.InstalledEntry{}
	}
	for i := range entries {
		e := &entries[i]
		targetDir, err := installer.Targets(e.Target, e.ProjectPath)
		if err != nil {
			e.Status = "error"
			e.LastError = err.Error()
			_ = installer.SaveInstalled(entries)
			return err
		}
		cacheDir := installer.CacheDir(e.Skill, e.Version)
		if _, err := os.Stat(filepath.Join(cacheDir, "artifact.json")); err != nil {
			// need to pull
			ref := strings.TrimSuffix(e.Registry, "/") + "/" + e.Skill + ":" + e.Version
			if err := pullOneToCache(context.Background(), ref, cacheDir, e.Skill); err != nil {
				e.Status = "error"
				e.LastError = err.Error()
				e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				_ = installer.SaveInstalled(entries)
				return err
			}
		}
		if err := installer.InstallToTarget(cacheDir, targetDir, e.Skill); err != nil {
			e.Status = "error"
			e.LastError = err.Error()
			e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_ = installer.SaveInstalled(entries)
			return err
		}
		e.Status = "ok"
		e.LastError = ""
		e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return installer.SaveInstalled(entries)
}

func runInstall(cmd *cobra.Command, reference, target, projectPath, registryFlag string, force bool) error {
	ctx := context.Background()
	targetObj, ref, err := resolveTargetAndRef(reference)
	if err != nil {
		return err
	}
	rootManifest, err := oci.Inspect(ctx, targetObj, ref)
	if err != nil {
		return fmt.Errorf("inspect artifact: %w", err)
	}
	isOCI := strings.HasPrefix(reference, "oci:")
	registry := deriveDefaultRegistry(reference)
	if isOCI && len(rootManifest.Dependencies) > 0 {
		if registryFlag == "" {
			return fmt.Errorf("install with oci: reference and dependencies requires --registry")
		}
		registry = registryFlag
	}

	var resolved []resolver.ResolvedArtifact
	if len(rootManifest.Dependencies) == 0 {
		resolved = []resolver.ResolvedArtifact{{
			Name:     rootManifest.Metadata.Name,
			Version:  rootManifest.Metadata.Version,
			Registry: registry,
			Manifest: rootManifest,
		}}
	} else {
		fetcher := NewRemoteFetcher()
		var err error
		resolved, err = resolver.Resolve(ctx, rootManifest, registry, fetcher)
		if err != nil {
			return fmt.Errorf("resolving dependencies: %w", err)
		}
	}

	// Ensure each artifact is in cache (pull if missing)
	cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
	for i, r := range resolved {
		idx, res := i, r
		cacheDir := installer.CacheDir(res.Name, res.Version)
		pullFn := func(ctx context.Context, outputDir string) error {
			var pullTarget oras.ReadOnlyTarget
			pullRef := ref
			if idx == 0 {
				pullTarget = targetObj
			} else {
				repo := strings.TrimSuffix(res.Registry, "/") + "/" + res.Name
				reg, err := remote.NewRepository(repo)
				if err != nil {
					return err
				}
				pullTarget = reg
				pullRef = res.Version
			}
			if err := oci.Pull(ctx, pullTarget, pullRef, cacheRoot); err != nil {
				return err
			}
			created := filepath.Join(cacheRoot, res.Name)
			if err := os.Rename(created, cacheDir); err != nil {
				_ = os.RemoveAll(cacheDir)
				return os.Rename(created, cacheDir)
			}
			return nil
		}
		if err := installer.EnsureInCache(ctx, cacheDir, pullFn); err != nil {
			return fmt.Errorf("pull %s@%s: %w", res.Name, res.Version, err)
		}
	}

	// Conflict check
	existing, err := installer.LoadInstalled()
	if err != nil {
		return err
	}
	if existing == nil {
		existing = []installer.InstalledEntry{}
	}
	required := buildRequired(existing)
	for _, r := range resolved {
		if v, ok := required[r.Name]; ok && v != r.Version && !force {
			return fmt.Errorf("%s@%s conflicts with installed %s@%s (use --force to override)", r.Name, r.Version, r.Name, v)
		}
	}

	targetDir, err := installer.Targets(target, projectPath)
	if err != nil {
		return err
	}
	for _, r := range resolved {
		cacheDir := installer.CacheDir(r.Name, r.Version)
		if err := installer.InstallToTarget(cacheDir, targetDir, r.Name); err != nil {
			return fmt.Errorf("install %s to target: %w", r.Name, err)
		}
	}

	// Merge new entries into DB (only on full success)
	rootName := rootManifest.Metadata.Name
	normProject, _ := filepath.Abs(projectPath)
	if projectPath == "" {
		normProject = ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	byKey := make(map[string]*installer.InstalledEntry)
	for i := range existing {
		e := &existing[i]
		key := e.Skill + "|" + e.Target + "|" + e.ProjectPath
		byKey[key] = e
	}
	for _, r := range resolved {
		installedWith := rootName
		if r.Name == rootName && r.Version == rootManifest.Metadata.Version {
			installedWith = ""
		}
		key := r.Name + "|" + target + "|" + normProject
		byKey[key] = &installer.InstalledEntry{
			Skill:         r.Name,
			Version:       r.Version,
			Registry:      r.Registry,
			Target:        target,
			ProjectPath:   normProject,
			InstalledWith: installedWith,
			Status:        "ok",
			UpdatedAt:     now,
		}
	}
	var newEntries []installer.InstalledEntry
	for _, e := range byKey {
		newEntries = append(newEntries, *e)
	}
	if err := installer.SaveInstalled(newEntries); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Installed", len(resolved), "artifact(s) to", targetDir)
	return nil
}

func buildRequired(entries []installer.InstalledEntry) map[string]string {
	required := make(map[string]string)
	for _, e := range entries {
		if e.InstalledWith != "" {
			continue
		}
		cacheDir := installer.CacheDir(e.Skill, e.Version)
		m, err := artifact.Load(filepath.Join(cacheDir, "artifact.json"))
		if err != nil {
			continue
		}
		required[m.Metadata.Name] = m.Metadata.Version
		for _, d := range m.Dependencies {
			required[d.Name] = d.Version
		}
	}
	return required
}

func pullOneToCache(ctx context.Context, reference, cacheDir, name string) error {
	targetObj, ref, err := resolveTargetAndRef(reference)
	if err != nil {
		return err
	}
	cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return err
	}
	if err := oci.Pull(ctx, targetObj, ref, cacheRoot); err != nil {
		return err
	}
	created := filepath.Join(cacheRoot, name)
	return os.Rename(created, cacheDir)
}
