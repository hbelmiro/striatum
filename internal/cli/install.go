package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/resolver"
	"github.com/spf13/cobra"
	oras "oras.land/oras-go/v2"
)

func newInstallCmd() *cobra.Command {
	var target, projectPath, registry string
	var force, reinstallAll bool
	cmd := &cobra.Command{
		Use:     "install",
		Short:   "Pull and install a skill into AI coding agent skills directories",
		Long:    "Resolves the skill artifact and dependencies, copies them to the install target (Cursor or Claude skills dir). Requires --target (cursor or claude). Use --project for project-level install.",
		Example: "  striatum skill install --target cursor localhost:5000/skills/my-skill:1.0.0",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reinstallAll {
				return runReinstallAll(cmd)
			}
			if len(args) == 0 {
				return fmt.Errorf("install requires a reference (e.g. host/repo/name:tag or oci:/path:tag)")
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
		return fmt.Errorf("load installed: %w", err)
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
			if saveErr := installer.SaveInstalled(entries); saveErr != nil {
				return fmt.Errorf("%w (also failed to persist state: %v)", err, saveErr)
			}
			return err
		}
		cacheDir := installer.CacheDir(e.Skill, e.Version)
		if _, statErr := os.Stat(filepath.Join(cacheDir, "artifact.json")); statErr != nil {
			if !os.IsNotExist(statErr) {
				return fmt.Errorf("check cache for %s@%s: %w", e.Skill, e.Version, statErr)
			}
			if strings.TrimSpace(e.Registry) == "" {
				e.Status = "error"
				e.LastError = "cannot re-pull: entry has no registry (e.g. was installed from oci: layout or cache-only name:version); re-install from original source"
				e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				if saveErr := installer.SaveInstalled(entries); saveErr != nil {
					return fmt.Errorf("%s@%s: %s (also failed to persist state: %v)", e.Skill, e.Version, e.LastError, saveErr)
				}
				return fmt.Errorf("%s@%s: %s", e.Skill, e.Version, e.LastError)
			}
			ref := strings.TrimSuffix(e.Registry, "/") + "/" + e.Skill + ":" + e.Version
			if err := pullOneToCache(cmd.Context(), ref, cacheDir, e.Skill); err != nil {
				e.Status = "error"
				e.LastError = err.Error()
				e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				if saveErr := installer.SaveInstalled(entries); saveErr != nil {
					return fmt.Errorf("%w (also failed to persist state: %v)", err, saveErr)
				}
				return err
			}
		}
		if err := installer.InstallToTarget(cacheDir, targetDir, e.Skill); err != nil {
			e.Status = "error"
			e.LastError = err.Error()
			e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if saveErr := installer.SaveInstalled(entries); saveErr != nil {
				return fmt.Errorf("%w (also failed to persist state: %v)", err, saveErr)
			}
			return err
		}
		e.Status = "ok"
		e.LastError = ""
		e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := installer.SaveInstalled(entries); err != nil {
		return fmt.Errorf("save installed: %w", err)
	}
	return nil
}

// refToCacheCandidate derives a cache key (name, version) from a reference when possible.
// Registry refs (host/repo/path:tag) use the last path segment as name and tag as version.
// OCI refs (oci:/path:tag) use tag; if tag contains ":", split on last ":" for name and version.
// Returns ok=false when the reference cannot be mapped to name@version (e.g. oci tag is version-only).
func refToCacheCandidate(reference string) (name, version string, ok bool) {
	if strings.HasPrefix(reference, "oci:") {
		_, tag, err := oci.SplitReference(reference)
		if err != nil {
			return "", "", false
		}
		i := strings.LastIndex(tag, ":")
		if i <= 0 || i == len(tag)-1 {
			return "", "", false
		}
		name = strings.TrimSpace(tag[:i])
		version = strings.TrimSpace(tag[i+1:])
		if name == "" || version == "" {
			return "", "", false
		}
		return name, version, true
	}
	repo, tag, err := oci.SplitReference(reference)
	if err != nil {
		return "", "", false
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", "", false
	}
	name = repo
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		name = repo[i+1:]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}
	return name, tag, true
}

func runInstall(cmd *cobra.Command, reference, target, projectPath, registryFlag string, force bool) error {
	ctx := cmd.Context()
	var targetObj oras.ReadOnlyTarget
	var ref string
	var rootManifest *artifact.Manifest

	if name, version, ok := refToCacheCandidate(reference); ok {
		cacheDir := installer.CacheDir(name, version)
		manifestPath := filepath.Join(cacheDir, "artifact.json")
		switch _, statErr := os.Stat(manifestPath); {
		case statErr == nil:
			m, err := artifact.Load(manifestPath)
			if err == nil && m != nil && m.Kind == "Skill" &&
				m.Metadata.Name == name && m.Metadata.Version == version {
				rootManifest = m
			} else {
				// Load failed or manifest mismatch: remove corrupt entry so EnsureInCache will re-pull.
				if err := os.Remove(manifestPath); err != nil {
					return fmt.Errorf("cache corruption for %s@%s; remove failed: %w", name, version, err)
				}
			}
		case !os.IsNotExist(statErr):
			return fmt.Errorf("stat cache for %s@%s: %w", name, version, statErr)
		}
	}
	if rootManifest == nil {
		if !strings.Contains(reference, "/") && !strings.HasPrefix(reference, "oci:") {
			return fmt.Errorf("short ref %q is not in cache (cache-only); use a full reference (host/repo/name:version or oci:/path:name:version) to pull from a registry", reference)
		}
		var err error
		targetObj, ref, err = resolveTargetAndRef(reference)
		if err != nil {
			return fmt.Errorf("resolve reference: %w", err)
		}
		rootManifest, err = oci.Inspect(ctx, targetObj, ref)
		if err != nil {
			return fmt.Errorf("read artifact manifest: %w", err)
		}
	}
	isOCI := strings.HasPrefix(reference, "oci:")
	isShortRef := !strings.Contains(reference, "/") && !strings.HasPrefix(reference, "oci:")
	registry := deriveDefaultRegistry(reference)
	if isOCI && len(rootManifest.Dependencies) > 0 {
		if registryFlag == "" {
			return fmt.Errorf("install with oci: reference and dependencies requires --registry")
		}
		registry = registryFlag
	}
	if isShortRef && len(rootManifest.Dependencies) > 0 && strings.TrimSpace(registryFlag) == "" {
		return fmt.Errorf("short ref with dependencies requires --registry (or use a full reference)")
	}
	if isShortRef && len(rootManifest.Dependencies) > 0 {
		registry = strings.TrimSpace(registryFlag)
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
		fetcher := NewCacheFirstFetcher(NewRemoteFetcher())
		var err error
		resolved, err = resolver.Resolve(ctx, rootManifest, registry, fetcher)
		if err != nil {
			return fmt.Errorf("resolving dependencies: %w", err)
		}
	}

	if err := ensureArtifactsInCache(ctx, reference, targetObj, ref, resolved); err != nil {
		return err
	}

	// Conflict check
	existing, err := installer.LoadInstalled()
	if err != nil {
		return fmt.Errorf("load installed: %w", err)
	}
	if existing == nil {
		existing = []installer.InstalledEntry{}
	}
	required, err := buildRequired(existing)
	if err != nil {
		return fmt.Errorf("build required set: %w", err)
	}
	for _, r := range resolved {
		if v, ok := required[r.Name]; ok && v != r.Version && !force {
			return fmt.Errorf("%s@%s conflicts with installed %s@%s (use --force to override)", r.Name, r.Version, r.Name, v)
		}
	}

	targetDir, err := installer.Targets(target, projectPath)
	if err != nil {
		return fmt.Errorf("resolve target dir: %w", err)
	}
	for _, r := range resolved {
		cacheDir := installer.CacheDir(r.Name, r.Version)
		if err := installer.InstallToTarget(cacheDir, targetDir, r.Name); err != nil {
			return fmt.Errorf("install %s to target: %w", r.Name, err)
		}
	}

	// Merge new entries into DB (only on full success)
	rootName := rootManifest.Metadata.Name
	normProject := ""
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("resolve project path %q: %w", projectPath, err)
		}
		normProject = abs
	}
	now := time.Now().UTC().Format(time.RFC3339)
	byKey := make(map[string]*installer.InstalledEntry)
	for i := range existing {
		e := &existing[i]
		key := e.Skill + "|" + e.Target + "|" + e.ProjectPath
		byKey[key] = e
	}
	// Short ref (e.g. "example-skill:1.0.0") has no registry; do not persist derived value
	// so --reinstall-all does not build invalid pull refs like "example-skill/example-skill:1.0.0".
	for _, r := range resolved {
		installedWith := rootName
		if r.Name == rootName && r.Version == rootManifest.Metadata.Version {
			installedWith = ""
		}
		reg := r.Registry
		if isShortRef && installedWith == "" {
			// Root from short ref: no registry to persist. Deps keep registry so --reinstall-all can re-pull them.
			reg = ""
		}
		key := r.Name + "|" + target + "|" + normProject
		byKey[key] = &installer.InstalledEntry{
			Skill:         r.Name,
			Version:       r.Version,
			Registry:      reg,
			Target:        target,
			ProjectPath:   normProject,
			InstalledWith: installedWith,
			Status:        "ok",
			UpdatedAt:     now,
		}
	}
	newEntries := make([]installer.InstalledEntry, 0, len(byKey))
	for _, e := range byKey {
		newEntries = append(newEntries, *e)
	}
	sort.Slice(newEntries, func(i, j int) bool {
		a, b := newEntries[i], newEntries[j]
		if a.Skill != b.Skill {
			return a.Skill < b.Skill
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.ProjectPath < b.ProjectPath
	})
	if err := installer.SaveInstalled(newEntries); err != nil {
		return fmt.Errorf("save installed: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Installed", len(resolved), "artifact(s) to", targetDir)
	return nil
}

func buildRequired(entries []installer.InstalledEntry) (map[string]string, error) {
	required := make(map[string]string)
	for _, e := range entries {
		if e.InstalledWith != "" {
			continue
		}
		cacheDir := installer.CacheDir(e.Skill, e.Version)
		m, err := artifact.Load(filepath.Join(cacheDir, "artifact.json"))
		if err != nil {
			return nil, fmt.Errorf("load cached manifest for %s@%s: %w", e.Skill, e.Version, err)
		}
		required[m.Metadata.Name] = m.Metadata.Version
		for _, d := range m.Dependencies {
			required[d.Name] = d.Version
		}
	}
	return required, nil
}

// ensureArtifactsInCache pulls each resolved artifact into the Striatum cache when missing.
// rootTarget and rootRef apply to resolved[0]; rootTarget may be nil when the root manifest
// was loaded from cache and re-pull lazy-resolves via reference.
func ensureArtifactsInCache(ctx context.Context, reference string, rootTarget oras.ReadOnlyTarget, rootRef string, resolved []resolver.ResolvedArtifact) error {
	cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
	for i, r := range resolved {
		idx, res := i, r
		cacheDir := installer.CacheDir(res.Name, res.Version)
		pullFn := func(ctx context.Context, _ string) error {
			var pullTarget oras.ReadOnlyTarget
			pullRef := rootRef
			if idx == 0 {
				pullTarget = rootTarget
				if pullTarget == nil {
					resolvedTarget, resolvedRef, err := resolveTargetAndRef(reference)
					if err != nil {
						return fmt.Errorf("root was loaded from cache but cache is no longer present; cannot re-pull: %w", err)
					}
					pullTarget, pullRef = resolvedTarget, resolvedRef
				}
			} else {
				repo := strings.TrimSuffix(res.Registry, "/") + "/" + res.Name
				reg, err := oci.NewRepository(repo)
				if err != nil {
					return fmt.Errorf("create repository for %s: %w", res.Name, err)
				}
				pullTarget = reg
				pullRef = res.Version
			}
			if err := oci.Pull(ctx, pullTarget, pullRef, cacheRoot); err != nil {
				return fmt.Errorf("download OCI artifact: %w", err)
			}
			created := filepath.Join(cacheRoot, res.Name)
			if err := atomicReplaceCacheDir(created, cacheDir); err != nil {
				return fmt.Errorf("finalize cache directory: %w", err)
			}
			return nil
		}
		if err := installer.EnsureInCache(ctx, cacheDir, pullFn); err != nil {
			return fmt.Errorf("pull %s@%s: %w", res.Name, res.Version, err)
		}
	}
	return nil
}

func pullOneToCache(ctx context.Context, reference, cacheDir, name string) error {
	targetObj, ref, err := resolveTargetAndRef(reference)
	if err != nil {
		return fmt.Errorf("resolve reference: %w", err)
	}
	cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return fmt.Errorf("create cache root: %w", err)
	}
	if err := oci.Pull(ctx, targetObj, ref, cacheRoot); err != nil {
		return fmt.Errorf("pull artifact: %w", err)
	}
	created := filepath.Join(cacheRoot, name)
	return atomicReplaceCacheDir(created, cacheDir)
}

// atomicReplaceCacheDir moves created (fresh pull) to cacheDir, removing cacheDir first if it exists
// (e.g. partial/corrupt cache missing artifact.json) so Rename succeeds.
func atomicReplaceCacheDir(created, cacheDir string) error {
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("remove existing cache dir: %w", err)
	}
	if err := os.Rename(created, cacheDir); err != nil {
		if rmErr := os.RemoveAll(created); rmErr != nil {
			return fmt.Errorf("rename to cache dir: %w (cleanup of %q failed: %v)", err, created, rmErr)
		}
		return fmt.Errorf("rename to cache dir: %w", err)
	}
	return nil
}
