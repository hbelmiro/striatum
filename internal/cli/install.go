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
	"github.com/hbelmiro/striatum/pkg/registry"
	"github.com/hbelmiro/striatum/pkg/resolver"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
)

func newInstallCmd() *cobra.Command {
	var target, projectPath string
	var force, reinstallAll bool
	cmd := &cobra.Command{
		Use:     "install",
		Short:   "Pull and install a skill into AI coding agent skills directories",
		Long:    "Resolves the skill artifact and dependencies, copies them to the install target (Cursor or Claude skills dir). Requires --target (cursor or claude). Use --project for project-level install. Accepts a local directory path, oci:/path:tag, or registry reference.",
		Example: "  striatum skill install --target cursor localhost:5000/skills/my-skill:1.0.0\n  striatum skill install --target cursor oci:/path/to/layout:my-skill:1.0.0\n  striatum skill install --target cursor .",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reinstallAll {
				return runReinstallAll(cmd)
			}
			if len(args) == 0 {
				return fmt.Errorf("install requires a reference (e.g. host/repo/name:tag, oci:/path:tag, or local directory path)")
			}
			reference := args[0]
			target = strings.TrimSpace(target)
			if target == "" {
				return fmt.Errorf("--target is required (cursor or claude)")
			}
			if target != "cursor" && target != "claude" {
				return fmt.Errorf("--target must be cursor or claude, got %q", target)
			}
			return runInstall(cmd, reference, target, strings.TrimSpace(projectPath), force)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Install target: cursor or claude (required)")
	cmd.Flags().StringVar(&projectPath, "project", "", "Project path for project-level install (e.g. .)")
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
				e.LastError = "cannot re-pull: no source ref stored; re-install from original source"
				e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				if saveErr := installer.SaveInstalled(entries); saveErr != nil {
					return fmt.Errorf("%s@%s: %s (also failed to persist state: %v)", e.Skill, e.Version, e.LastError, saveErr)
				}
				return fmt.Errorf("%s@%s: %s", e.Skill, e.Version, e.LastError)
			}
			if err := repullToCache(cmd.Context(), e.Registry, cacheDir, e.Skill); err != nil {
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
// Returns ok=false when the reference cannot be mapped to name@version.
func refToCacheCandidate(reference string) (name, version string, ok bool) {
	if strings.HasPrefix(reference, "git:") {
		return "", "", false
	}
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

func isLocalDirRef(reference string) bool {
	abs, err := filepath.Abs(reference)
	if err != nil {
		return false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = os.Stat(filepath.Join(abs, "artifact.json"))
	return err == nil
}

func runInstall(cmd *cobra.Command, reference, target, projectPath string, force bool) error {
	if isLocalDirRef(reference) {
		return runLocalInstall(cmd, reference, target, projectPath, force)
	}
	if abs, err := filepath.Abs(reference); err == nil {
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return fmt.Errorf("directory %q exists but does not contain artifact.json", reference)
		}
	}

	ctx := cmd.Context()
	var targetObj oras.ReadOnlyTarget
	var ref string
	var rootManifest *artifact.Manifest

	if name, version, ok := refToCacheCandidate(reference); ok {
		m, err := loadCachedSkillManifest(name, version)
		if err != nil {
			return err
		}
		rootManifest = m
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

	var resolved []resolver.ResolvedArtifact
	if len(rootManifest.Dependencies) == 0 {
		resolved = []resolver.ResolvedArtifact{{
			Name:     rootManifest.Metadata.Name,
			Version:  rootManifest.Metadata.Version,
			Manifest: rootManifest,
		}}
	} else {
		fetcher := NewCacheFirstFetcher(NewRemoteFetcher())
		var err error
		resolved, err = resolver.Resolve(ctx, rootManifest, fetcher)
		if err != nil {
			return fmt.Errorf("resolving dependencies: %w", err)
		}
	}

	if err := ensureArtifactsInCache(ctx, reference, targetObj, ref, resolved); err != nil {
		return err
	}

	rootSourceRef := reference
	isShortRef := !strings.Contains(reference, "/") && !strings.HasPrefix(reference, "oci:")
	if isShortRef {
		rootSourceRef = ""
	}
	return installResolvedArtifacts(cmd, resolved, rootManifest, target, projectPath, rootSourceRef, force)
}

func runLocalInstall(cmd *cobra.Command, reference, target, projectPath string, force bool) error {
	absPath, err := filepath.Abs(reference)
	if err != nil {
		return fmt.Errorf("resolve local path %q: %w", reference, err)
	}

	rootManifest, err := artifact.Load(filepath.Join(absPath, "artifact.json"))
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	if err := artifact.Validate(rootManifest); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	if err := artifact.ValidateLocal(rootManifest, absPath); err != nil {
		return fmt.Errorf("validate local files: %w", err)
	}

	name := rootManifest.Metadata.Name
	version := rootManifest.Metadata.Version
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") ||
		strings.ContainsAny(version, "/\\") || strings.Contains(version, "..") {
		return fmt.Errorf("unsafe metadata.name or metadata.version: must not contain path separators or '..'")
	}

	ctx := cmd.Context()
	var resolved []resolver.ResolvedArtifact
	if len(rootManifest.Dependencies) == 0 {
		resolved = []resolver.ResolvedArtifact{{
			Name:     name,
			Version:  version,
			Manifest: rootManifest,
		}}
	} else {
		fetcher := NewCacheFirstFetcher(NewRemoteFetcher())
		resolved, err = resolver.Resolve(ctx, rootManifest, fetcher)
		if err != nil {
			return fmt.Errorf("resolving dependencies: %w", err)
		}
	}

	cacheDir := installer.CacheDir(name, version)
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("clean cache dir: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	if err := copyLocalToCache(absPath, cacheDir, rootManifest.Spec.Files); err != nil {
		return fmt.Errorf("cache local skill: %w", err)
	}

	if len(resolved) > 1 {
		cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
		for _, r := range resolved[1:] {
			res := r
			depCacheDir := installer.CacheDir(res.Name, res.Version)
			pullFn := func(ctx context.Context, _ string) error {
				if err := pullDependency(ctx, res.Dependency, cacheRoot); err != nil {
					return err
				}
				created := filepath.Join(cacheRoot, res.Name)
				return atomicReplaceCacheDir(created, depCacheDir)
			}
			var digestFn installer.DigestFunc
			if ociDep, ok := res.Dependency.(*artifact.OCIDependency); ok {
				capturedDep := ociDep
				digestFn = func(ctx context.Context) (string, error) {
					repoPath := capturedDep.RegistryHost + "/" + capturedDep.Repository
					reg, err := oci.NewRepository(repoPath)
					if err != nil {
						return "", err
					}
					return oci.ResolveDigest(ctx, reg, capturedDep.Tag)
				}
			}
			if err := installer.EnsureInCache(ctx, depCacheDir, pullFn, digestFn); err != nil {
				return fmt.Errorf("pull %s@%s: %w", res.Name, res.Version, err)
			}
		}
	}

	return installResolvedArtifacts(cmd, resolved, rootManifest, target, projectPath, "", force)
}

func copyLocalToCache(srcDir, cacheDir string, files []string) error {
	realSrcDir, err := filepath.EvalSymlinks(srcDir)
	if err != nil {
		return fmt.Errorf("resolve source dir: %w", err)
	}
	srcPrefix := realSrcDir + string(filepath.Separator)

	manifestSrc := filepath.Join(srcDir, "artifact.json")
	realManifest, err := filepath.EvalSymlinks(manifestSrc)
	if err != nil {
		return fmt.Errorf("resolve artifact.json: %w", err)
	}
	if realManifest != realSrcDir && !strings.HasPrefix(realManifest, srcPrefix) {
		return fmt.Errorf("artifact.json resolves outside skill directory via symlink")
	}
	data, err := os.ReadFile(manifestSrc)
	if err != nil {
		return fmt.Errorf("read artifact.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o644); err != nil {
		return fmt.Errorf("write artifact.json: %w", err)
	}

	for _, f := range files {
		src := filepath.Join(srcDir, filepath.FromSlash(f))
		realSrc, err := filepath.EvalSymlinks(src)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", f, err)
		}
		if realSrc != realSrcDir && !strings.HasPrefix(realSrc, srcPrefix) {
			return fmt.Errorf("file %q resolves outside skill directory via symlink", f)
		}

		dst := filepath.Join(cacheDir, filepath.FromSlash(f))
		if dir := filepath.Dir(dst); dir != cacheDir {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create dir for %s: %w", f, err)
			}
		}
		srcData, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		info, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("stat %s: %w", f, err)
		}
		if err := os.WriteFile(dst, srcData, info.Mode()); err != nil {
			return fmt.Errorf("write %s: %w", f, err)
		}
	}
	return nil
}

func installResolvedArtifacts(cmd *cobra.Command, resolved []resolver.ResolvedArtifact, rootManifest *artifact.Manifest, target, projectPath, rootSourceRef string, force bool) error {
	normProject := ""
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("resolve project path %q: %w", projectPath, err)
		}
		normProject = abs
	}

	existing, err := installer.LoadInstalled()
	if err != nil {
		return fmt.Errorf("load installed: %w", err)
	}
	if existing == nil {
		existing = []installer.InstalledEntry{}
	}
	required := buildRequired(existing, normProject)
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
		cd := installer.CacheDir(r.Name, r.Version)
		if err := installer.InstallToTarget(cd, targetDir, r.Name); err != nil {
			return fmt.Errorf("install %s to target: %w", r.Name, err)
		}
	}

	rootName := rootManifest.Metadata.Name
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
		reg := sourceRefForDB(r.Dependency)
		if installedWith == "" {
			reg = rootSourceRef
		}
		key := r.Name + "|" + target + "|" + normProject
		if prev, ok := byKey[key]; ok && installedWith != "" {
			installedWith = mergeInstalledWith(prev.InstalledWith, installedWith)
		}
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

// mergeInstalledWith adds rootName to the space-separated list in existing,
// deduplicating so uninstall can track multiple parent roots for one dep.
func mergeInstalledWith(existing, rootName string) string {
	if existing == "" {
		return rootName
	}
	for _, tok := range strings.Fields(existing) {
		if tok == rootName {
			return existing
		}
	}
	return existing + " " + rootName
}

// sourceRefForDB returns the canonical reference string to persist in installed.yaml.
// reinstall-all uses this value directly to re-pull when cache is missing.
func sourceRefForDB(dep artifact.Dependency) string {
	if dep == nil {
		return ""
	}
	return dep.CanonicalRef()
}

// repullToCache re-downloads an artifact using a stored source ref.
// The ref is parsed into a Locator and dispatched via the Router.
func repullToCache(ctx context.Context, sourceRef, cacheDir, name string) error {
	loc, err := registry.ParseReference(sourceRef)
	if err != nil {
		return fmt.Errorf("parse reference %q: %w", sourceRef, err)
	}
	cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return fmt.Errorf("create cache root: %w", err)
	}
	if err := defaultRouter().Pull(ctx, loc, cacheRoot); err != nil {
		return fmt.Errorf("pull artifact %q: %w", sourceRef, err)
	}
	created := filepath.Join(cacheRoot, name)
	if _, err := os.Stat(created); err != nil {
		return fmt.Errorf("expected artifact directory %q after pull (artifact metadata.name may differ from installed name %q): %w", created, name, err)
	}
	return atomicReplaceCacheDir(created, cacheDir)
}

// buildRequired returns a map of skill name -> version for installed entries in the given scope.
// Filters to entries matching projectPath to enable per-scope conflict detection.
func buildRequired(entries []installer.InstalledEntry, projectPath string) map[string]string {
	required := make(map[string]string)
	for _, e := range entries {
		if e.ProjectPath != projectPath {
			continue
		}
		required[e.Skill] = e.Version
	}
	return required
}

// ensureArtifactsInCache pulls each resolved artifact into the Striatum cache when missing.
// rootTarget and rootRef apply to resolved[0]; rootTarget may be nil when the root manifest
// was loaded from cache and re-pull lazy-resolves via reference.
func ensureArtifactsInCache(ctx context.Context, reference string, rootTarget oras.ReadOnlyTarget, rootRef string, resolved []resolver.ResolvedArtifact) error {
	cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
	for i, r := range resolved {
		idx, res := i, r
		cacheDir := installer.CacheDir(res.Name, res.Version)

		// Construct DigestFunc based on artifact type
		var digestFn installer.DigestFunc
		if idx == 0 {
			// Root artifact: use rootTarget/rootRef if available
			if rootTarget != nil && !strings.HasPrefix(reference, "git:") {
				capturedTarget, capturedRef := rootTarget, rootRef
				digestFn = func(ctx context.Context) (string, error) {
					return oci.ResolveDigest(ctx, capturedTarget, capturedRef)
				}
			} else if rootTarget == nil && !strings.HasPrefix(reference, "git:") && (strings.Contains(reference, "/") || strings.HasPrefix(reference, "oci:")) {
				// Root was loaded from cache; lazy-resolve target for digest (only if not short ref)
				capturedRef := reference
				digestFn = func(ctx context.Context) (string, error) {
					t, ref, err := resolveTargetAndRef(capturedRef)
					if err != nil {
						return "", err
					}
					return oci.ResolveDigest(ctx, t, ref)
				}
			}
			// If git ref or short ref, digestFn stays nil
		} else {
			// Dependency: check if it's an OCI dependency
			if ociDep, ok := res.Dependency.(*artifact.OCIDependency); ok {
				capturedDep := ociDep
				digestFn = func(ctx context.Context) (string, error) {
					repoPath := capturedDep.RegistryHost + "/" + capturedDep.Repository
					reg, err := oci.NewRepository(repoPath)
					if err != nil {
						return "", err
					}
					return oci.ResolveDigest(ctx, reg, capturedDep.Tag)
				}
			}
			// Git dependencies: digestFn stays nil (no digest checking)
		}

		pullFn := func(ctx context.Context, _ string) error {
			if idx == 0 {
				pullTarget := rootTarget
				pullRef := rootRef
				if pullTarget == nil {
					resolvedTarget, resolvedRef, err := resolveTargetAndRef(reference)
					if err != nil {
						return fmt.Errorf("root was loaded from cache but cache is no longer present; cannot re-pull: %w", err)
					}
					pullTarget, pullRef = resolvedTarget, resolvedRef
				}
				if err := oci.Pull(ctx, pullTarget, pullRef, cacheRoot); err != nil {
					return fmt.Errorf("download OCI artifact: %w", err)
				}
			} else {
				if err := pullDependency(ctx, res.Dependency, cacheRoot); err != nil {
					return err
				}
			}
			created := filepath.Join(cacheRoot, res.Name)
			if err := atomicReplaceCacheDir(created, cacheDir); err != nil {
				return fmt.Errorf("finalize cache directory: %w", err)
			}
			return nil
		}
		if err := installer.EnsureInCache(ctx, cacheDir, pullFn, digestFn); err != nil {
			return fmt.Errorf("pull %s@%s: %w", res.Name, res.Version, err)
		}
	}
	return nil
}

// pullDependency dispatches a pull to the correct backend via the Router.
func pullDependency(ctx context.Context, dep artifact.Dependency, outputDir string) error {
	if dep == nil {
		return fmt.Errorf("nil dependency")
	}
	return defaultRouter().Pull(ctx, dep, outputDir)
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
