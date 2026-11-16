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
	"github.com/hbelmiro/striatum/pkg/selfcontained"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
)

// runSelfContainedInstall installs a self-contained image. The image's
// immutable identity is resolved once (a digest supplied in the reference
// wins; otherwise the tag is resolved a single time), and validation,
// extraction, and the DB record all use that same digest, so a tag moving
// mid-flight cannot pair one image's metadata with another's payload. The
// payload is pulled verbatim into the isolated self-contained cache and
// installed as one unit; dependency declarations in the manifest are
// provenance only and are never re-resolved externally.
func runSelfContainedInstall(cmd *cobra.Command, reference string, targetObj oras.ReadOnlyTarget, ref, suppliedDigest string, target, projectPath string, force bool) error {
	ctx := cmd.Context()
	digest := suppliedDigest
	if digest == "" {
		var err error
		digest, err = oci.ResolveDigest(ctx, targetObj, ref)
		if err != nil {
			return fmt.Errorf("resolve digest: %w", err)
		}
	}
	// Inspect by the immutable digest so the validated manifest and the
	// extracted payload are the same image.
	rootManifest, configBytes, err := oci.InspectWithConfig(ctx, targetObj, digest)
	if err != nil {
		return fmt.Errorf("read artifact manifest: %w", err)
	}
	if !selfcontained.IsMarked(configBytes) {
		return fmt.Errorf("reference %q does not identify a self-contained image (the tag may have moved); resolve the reference again and retry", reference)
	}
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
	// Pull by the immutable digest (both registries and layout stores
	// resolve by digest), so the installed content is exactly the image the
	// digest identifies.
	if err := ensureSelfContainedInCache(ctx, targetObj, digest, name, exportCacheDir, digest); err != nil {
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
			// This is now an explicit root install; any dependency
			// relationship from a previous install no longer applies.
			existing[i].InstalledWith = ""
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
	targetObj, ref, suppliedDigest, err := resolveTargetAndRef(sourceRef)
	if err != nil {
		return fmt.Errorf("resolve reference %q: %w", sourceRef, err)
	}
	digest := suppliedDigest
	if digest == "" {
		digest, err = oci.ResolveDigest(ctx, targetObj, ref)
		if err != nil {
			return fmt.Errorf("resolve digest for %q: %w", sourceRef, err)
		}
	}
	return ensureSelfContainedInCache(ctx, targetObj, digest, name, exportCacheDir, digest)
}

// pullSelfContained handles "striatum pull" of a self-contained image. The
// image's immutable identity is resolved once (a supplied digest wins),
// metadata is re-read by that digest, and extraction uses it, so a tag
// moving mid-flight cannot pair one image's metadata with another's payload.
// With noCache it pulls verbatim straight to outputDir/<name>/ (clearing any
// previous extraction so the output is exactly the image's payload);
// otherwise the payload is materialized through the isolated self-contained
// cache and copied to the output directory. Either way there is no external
// dependency resolution.
func pullSelfContained(cmd *cobra.Command, targetObj oras.ReadOnlyTarget, reference, ref, suppliedDigest string, outputDir string, noCache bool) error {
	ctx := cmd.Context()
	digest := suppliedDigest
	if digest == "" {
		var err error
		digest, err = oci.ResolveDigest(ctx, targetObj, ref)
		if err != nil {
			return fmt.Errorf("resolve digest: %w", err)
		}
	}
	rootManifest, configBytes, err := oci.InspectWithConfig(ctx, targetObj, digest)
	if err != nil {
		return fmt.Errorf("read artifact manifest: %w", err)
	}
	if !selfcontained.IsMarked(configBytes) {
		return fmt.Errorf("reference %q does not identify a self-contained image (the tag may have moved); resolve the reference again and retry", reference)
	}
	name := rootManifest.Metadata.Name
	version := rootManifest.Metadata.Version
	if err := checkSafeNameVersion(name, version); err != nil {
		return err
	}
	if noCache {
		// Clear any previous extraction so the output is exactly this
		// image's payload, not a mix with an older image's files.
		if err := os.RemoveAll(filepath.Join(outputDir, name)); err != nil {
			return fmt.Errorf("clear output dir: %w", err)
		}
		if err := oci.Pull(ctx, targetObj, digest, outputDir); err != nil {
			return fmt.Errorf("pull self-contained image: %w", err)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pulled to", outputDir)
		return nil
	}
	exportCacheDir := installer.SelfContainedCacheDir(name, version)
	if err := ensureSelfContainedInCache(ctx, targetObj, digest, name, exportCacheDir, digest); err != nil {
		return err
	}
	if err := installer.InstallToTarget(exportCacheDir, outputDir, name); err != nil {
		return fmt.Errorf("copy self-contained image to output: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pulled to", outputDir)
	return nil
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
