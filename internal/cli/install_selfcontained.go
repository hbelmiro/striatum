package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
)

// runSelfContainedInstall installs a self-contained image. The payload is
// pulled verbatim (root files, deps/ closure, provenance) into the isolated
// self-contained cache and installed as one unit; dependency declarations in the
// manifest are provenance only and are never re-resolved externally.
func runSelfContainedInstall(cmd *cobra.Command, reference string, targetObj oras.ReadOnlyTarget, ref string, rootManifest *artifact.Manifest, target, projectPath string, force bool) error {
	ctx := cmd.Context()
	if err := artifact.Validate(rootManifest); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	if rootManifest.Kind != "Skill" {
		return fmt.Errorf("self-contained images must contain a Skill root artifact, got kind %q", rootManifest.Kind)
	}
	if err := validateInstallableKind(rootManifest.Kind, target); err != nil {
		return err
	}

	name := rootManifest.Metadata.Name
	version := rootManifest.Metadata.Version
	if err := checkSafeNameVersion(name, version); err != nil {
		return err
	}

	// Canonical immutable reference for the DB: resolve by digest so
	// reinstall keeps selecting this exact image even if the tag moves.
	digest, err := oci.ResolveDigest(ctx, targetObj, ref)
	if err != nil {
		return fmt.Errorf("resolve digest: %w", err)
	}
	repo, _, _, splitErr := oci.SplitReference(reference)
	if splitErr != nil {
		return fmt.Errorf("parse reference: %w", splitErr)
	}
	registryRef := repo + "@" + digest
	if strings.HasPrefix(reference, "oci:") {
		// Layout stores can also resolve by digest ("oci:<path>@<digest>").
		registryRef = "oci:" + repo + "@" + digest
	}

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
	if v, ok := buildRequired(existing, normProject, target)["Skill|"+name]; ok && v != version && !force {
		return fmt.Errorf("export for %s@%s conflicts with installed %s@%s (use --force to override)", name, version, name, v)
	}

	exportCacheDir := installer.SelfContainedCacheDir(name, version)
	// Pull by digest so the installed content matches the resolved image
	// even if the tag moves between resolution and pull.
	if err := ensureSelfContainedInCache(ctx, targetObj, pullRefFor(reference, ref, digest), name, exportCacheDir, digest); err != nil {
		return err
	}

	targetDir, err := installer.Targets(target, projectPath, "Skill")
	if err != nil {
		return fmt.Errorf("resolve target dir: %w", err)
	}
	if err := installer.InstallToTarget(exportCacheDir, targetDir, name); err != nil {
		return fmt.Errorf("install export to target: %w", err)
	}

	key := "Skill|" + name + "|" + target + "|" + normProject
	now := time.Now().UTC().Format(time.RFC3339)
	replaced := false
	for i := range existing {
		if existing[i].Kind+"|"+existing[i].Name+"|"+existing[i].Target+"|"+existing[i].ProjectPath == key {
			existing[i].Version = version
			existing[i].Registry = registryRef
			existing[i].Status = "ok"
			existing[i].LastError = ""
			existing[i].UpdatedAt = now
			existing[i].SelfContained = true
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, installer.InstalledEntry{
			Name:          name,
			Kind:          "Skill",
			Version:       version,
			Registry:      registryRef,
			Target:        target,
			ProjectPath:   normProject,
			Status:        "ok",
			UpdatedAt:     now,
			SelfContained: true,
		})
	}
	sortInstalledEntries(existing)
	if err := installer.SaveInstalled(existing); err != nil {
		return fmt.Errorf("save installed: %w", err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Installed self-contained image %s@%s\n", name, version)
	return nil
}

// ensureSelfContainedInCache pulls the self-contained image verbatim into the isolated
// self-contained cache, keyed by its immutable manifest digest.
func ensureSelfContainedInCache(ctx context.Context, targetObj oras.ReadOnlyTarget, ref, name, exportCacheDir, digest string) error {
	cacheRoot := filepath.Join(installer.CacheRoot(), "cache")
	pullFn := func(ctx context.Context, _ string) error {
		created, cleanup, err := pullToStagingDir(cacheRoot, name, func(stagingDir string) error {
			return oci.Pull(ctx, targetObj, ref, stagingDir)
		})
		if err != nil {
			cleanup()
			return err
		}
		defer cleanup()
		return atomicReplaceCacheDir(created, exportCacheDir)
	}
	// The digest is immutable: an existing cache entry with the same digest
	// is always fresh.
	digestFn := func(context.Context) (string, error) { return digest, nil }
	return installer.EnsureInCache(ctx, exportCacheDir, pullFn, digestFn)
}

// repullSelfContainedToCache re-pulls a self-contained installation from its stored
// (immutable) source reference into the isolated self-contained cache.
func repullSelfContainedToCache(ctx context.Context, sourceRef, exportCacheDir, name string) error {
	targetObj, ref, _, err := resolveTargetAndRef(sourceRef)
	if err != nil {
		return fmt.Errorf("resolve reference %q: %w", sourceRef, err)
	}
	digest, err := oci.ResolveDigest(ctx, targetObj, ref)
	if err != nil {
		return fmt.Errorf("resolve digest for %q: %w", sourceRef, err)
	}
	return ensureSelfContainedInCache(ctx, targetObj, ref, name, exportCacheDir, digest)
}

// pullSelfContained handles "striatum pull" of a self-contained image. With
// noCache it pulls verbatim straight to outputDir/<name>/; otherwise the
// payload is materialized through the isolated self-contained cache and
// copied to the output directory. Either way there is no external dependency
// resolution, and the pull resolves by digest so the content matches the
// image the reference pointed at even if a tag moves mid-flight.
func pullSelfContained(cmd *cobra.Command, targetObj oras.ReadOnlyTarget, reference, ref string, rootManifest *artifact.Manifest, outputDir string, noCache bool) error {
	ctx := cmd.Context()
	name := rootManifest.Metadata.Name
	version := rootManifest.Metadata.Version
	if err := checkSafeNameVersion(name, version); err != nil {
		return err
	}
	digest, err := oci.ResolveDigest(ctx, targetObj, ref)
	if err != nil {
		return fmt.Errorf("resolve digest: %w", err)
	}
	pullRef := pullRefFor(reference, ref, digest)
	if noCache {
		if err := oci.Pull(ctx, targetObj, pullRef, outputDir); err != nil {
			return fmt.Errorf("pull self-contained image: %w", err)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pulled to", outputDir)
		return nil
	}
	exportCacheDir := installer.SelfContainedCacheDir(name, version)
	if err := ensureSelfContainedInCache(ctx, targetObj, pullRef, name, exportCacheDir, digest); err != nil {
		return err
	}
	if err := installer.InstallToTarget(exportCacheDir, outputDir, name); err != nil {
		return fmt.Errorf("copy self-contained image to output: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pulled to", outputDir)
	return nil
}

// pullRefFor returns the reference to pull with: for oci: layout
// references the resolved tag (layout stores are tag-indexed), otherwise
// the immutable digest.
func pullRefFor(reference, ref, digest string) string {
	if strings.HasPrefix(reference, "oci:") {
		return ref
	}
	return digest
}

// checkSafeNameVersion rejects manifest names and versions that could
// escape the self-contained cache or install target directories. It mirrors the rule
// the ordinary install path applies via validateResolvedPaths; the self-contained image
// flow must apply it itself because it never runs dependency resolution.
func checkSafeNameVersion(name, version string) error {
	for _, v := range []struct{ label, s string }{{"name", name}, {"version", version}} {
		if strings.TrimSpace(v.s) == "" || v.s == "." || strings.ContainsAny(v.s, "/\\") || strings.Contains(v.s, "..") {
			return fmt.Errorf("unsafe artifact %s %q: must be non-empty and must not contain path separators or '..'", v.label, v.s)
		}
	}
	return nil
}

// mayReferenceSelfContained reports whether a reference can point at a self-contained image
// image: full registry references and oci: layout references. Short
// cache-only refs (no slash) and git references cannot.
func mayReferenceSelfContained(reference string) bool {
	if strings.HasPrefix(reference, "git:") {
		return false
	}
	return strings.HasPrefix(reference, "oci:") || strings.Contains(reference, "/")
}
