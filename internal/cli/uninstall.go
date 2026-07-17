package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/spf13/cobra"
)

func newUninstallCmd() *cobra.Command {
	var target, projectPath, kindFlag string
	var allExistingProjects bool
	cmd := &cobra.Command{
		Use:     "uninstall",
		Short:   "Remove a previously installed artifact and orphaned dependencies",
		Long:    "Removes the named artifact from the given --target (cursor or claude) and removes any dependencies that are no longer required by other installed artifacts.",
		Example: "  striatum uninstall --target claude my-skill\n  striatum uninstall --target claude --kind Workflow my-workflow",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := args[0]
			name := normalizeUninstallName(raw)
			if name != strings.TrimSpace(raw) {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Note: version in %q ignored; uninstalling %q regardless of version\n", strings.TrimSpace(raw), name)
			}
			var err error
			target, err = validateTarget(target)
			if err != nil {
				return err
			}
			normProject := ""
			if projectPath != "" {
				abs, err := filepath.Abs(strings.TrimSpace(projectPath))
				if err != nil {
					return fmt.Errorf("resolve project path %q: %w", projectPath, err)
				}
				normProject = abs
			}
			return runUninstall(cmd, name, target, normProject, strings.TrimSpace(kindFlag), allExistingProjects)
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Uninstall from target: cursor or claude (required)")
	_ = cmd.MarkFlagRequired("target")
	cmd.Flags().StringVar(&projectPath, "project", "", "Project path (match project-level install)")
	cmd.Flags().StringVarP(&kindFlag, "kind", "k", "", "Artifact kind filter (Skill, Prompt, Workflow, or Memory); required when multiple kinds share the same name")
	cmd.Flags().BoolVar(&allExistingProjects, "all-existing-projects", false, "Uninstall Memory artifacts from all existing Claude Code project directories")
	return cmd
}

// normalizeUninstallName maps a plain name:version (no '/', not "oci:") to name so uninstall
// accepts the same short ref style as install. Full registry refs and oci: refs are left unchanged;
// for those, pass the artifact name as stored in the install DB (not the full reference).
func normalizeUninstallName(arg string) string {
	arg = strings.TrimSpace(arg)
	if strings.Contains(arg, "/") || strings.HasPrefix(arg, "oci:") {
		return arg
	}
	if i := strings.LastIndex(arg, ":"); i > 0 && i < len(arg)-1 {
		return strings.TrimSpace(arg[:i])
	}
	return arg
}

func runUninstall(cmd *cobra.Command, name, target, normProject, kindFilter string, allExistingProjects bool) error {
	if kindFilter != "" && !artifact.IsSupportedKind(kindFilter) {
		return fmt.Errorf("unsupported kind %q; supported: %s", kindFilter, artifact.SupportedKindsList())
	}
	entries, err := installer.LoadInstalled()
	if err != nil {
		return fmt.Errorf("load installed: %w", err)
	}
	if entries == nil {
		entries = []installer.InstalledEntry{}
	}

	var toRemove []installer.InstalledEntry
	for _, e := range entries {
		if e.Name != name {
			continue
		}
		if e.Target != target {
			continue
		}
		if allExistingProjects && e.Kind == "Memory" {
			// match all project paths
		} else if e.ProjectPath != normProject {
			continue
		}
		if e.InstalledWith != "" {
			continue
		}
		if kindFilter != "" && e.Kind != kindFilter {
			continue
		}
		toRemove = append(toRemove, e)
	}
	if len(toRemove) > 1 {
		allMemory := true
		for _, e := range toRemove {
			if e.Kind != "Memory" {
				allMemory = false
				break
			}
		}
		if !allMemory {
			kinds := make([]string, len(toRemove))
			for i, e := range toRemove {
				kinds[i] = e.Kind
			}
			return fmt.Errorf("multiple artifacts named %q installed for target %s (kinds: %s); use --kind to disambiguate", name, target, strings.Join(kinds, ", "))
		}
	}
	if len(toRemove) == 0 {
		seen := make(map[string]bool)
		var roots []string
		for _, e := range entries {
			if e.Name == name && e.Target == target && e.ProjectPath == normProject && e.InstalledWith != "" {
				for _, r := range strings.Fields(e.InstalledWith) {
					if !seen[r] {
						seen[r] = true
						roots = append(roots, r)
					}
				}
			}
		}
		if len(roots) > 0 {
			return fmt.Errorf("%q is a dependency installed by %s; uninstall the root artifact instead", name, strings.Join(roots, ", "))
		}
		return fmt.Errorf("artifact %q is not installed for target %s", name, target)
	}

	for _, e := range toRemove {
		if err := removeInstalledArtifact(cmd, e); err != nil {
			return err
		}
	}

	remaining := make([]installer.InstalledEntry, 0, len(entries))
	for _, e := range entries {
		keep := true
		for _, r := range toRemove {
			if e.Name == r.Name && e.Kind == r.Kind && e.Target == r.Target && e.ProjectPath == r.ProjectPath {
				keep = false
				break
			}
		}
		if keep {
			remaining = append(remaining, e)
		}
	}

	const maxOrphanPasses = 100
	for pass := 0; pass < maxOrphanPasses; pass++ {
		orphans := computeOrphans(remaining)
		if len(orphans) == 0 {
			break
		}
		for _, e := range orphans {
			if err := removeInstalledArtifact(cmd, e); err != nil {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: could not remove orphan", e.Name, ":", err)
			}
		}
		remaining = removeEntries(remaining, orphans)
	}

	if err := installer.SaveInstalled(remaining); err != nil {
		return fmt.Errorf("save installed: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Removed", name)
	return nil
}

func removeInstalledArtifact(cmd *cobra.Command, e installer.InstalledEntry) error {
	if e.Kind == "Memory" {
		targetDir, err := installer.MemoryTargetDir(e.ProjectPath)
		if err != nil {
			return fmt.Errorf("resolve memory target for %s: %w", e.Name, err)
		}
		memoryMDPath := filepath.Join(targetDir, "MEMORY.md")
		if err := installer.RemoveMemoryIndexEntries(memoryMDPath, e.Name); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not update MEMORY.md: %v\n", err)
		}
		if err := installer.RemoveFromTarget(targetDir, e.Name); err != nil {
			return fmt.Errorf("remove %s from target: %w", e.Name, err)
		}
		return nil
	}
	targetDir, err := installer.Targets(e.Target, e.ProjectPath, e.Kind)
	if err != nil {
		return fmt.Errorf("resolve target for %s: %w", e.Name, err)
	}
	if e.Kind == "Workflow" {
		if err := installer.RemoveWorkflowSymlink(targetDir, e.Name); err != nil {
			return fmt.Errorf("remove workflow symlink for %s: %w", e.Name, err)
		}
	}
	if err := installer.RemoveFromTarget(targetDir, e.Name); err != nil {
		return fmt.Errorf("remove %s from target: %w", e.Name, err)
	}
	return nil
}

// computeOrphans returns entries whose InstalledWith roots are all absent.
// InstalledWith may contain multiple space-separated root names when a dep
// was installed by more than one root. The dep is only orphaned when none
// of those roots are present.
func computeOrphans(entries []installer.InstalledEntry) []installer.InstalledEntry {
	type ctxKey string
	roots := make(map[ctxKey]map[string]bool)
	for _, e := range entries {
		if e.InstalledWith != "" {
			continue
		}
		ck := ctxKey(e.Target + "|" + e.ProjectPath)
		if roots[ck] == nil {
			roots[ck] = make(map[string]bool)
		}
		roots[ck][e.Name] = true
	}
	var orphans []installer.InstalledEntry
	for _, e := range entries {
		if e.InstalledWith == "" {
			continue
		}
		ck := ctxKey(e.Target + "|" + e.ProjectPath)
		anyPresent := false
		for _, rn := range strings.Fields(e.InstalledWith) {
			if roots[ck] != nil && roots[ck][rn] {
				anyPresent = true
				break
			}
		}
		if !anyPresent {
			orphans = append(orphans, e)
		}
	}
	return orphans
}

func removeEntries(entries, toRemove []installer.InstalledEntry) []installer.InstalledEntry {
	var out []installer.InstalledEntry
	for _, e := range entries {
		keep := true
		for _, r := range toRemove {
			if e.Name == r.Name && e.Kind == r.Kind && e.Target == r.Target && e.ProjectPath == r.ProjectPath {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, e)
		}
	}
	return out
}
