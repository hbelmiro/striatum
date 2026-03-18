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
	var target, projectPath string
	cmd := &cobra.Command{
		Use:     "uninstall",
		Short:   "Remove a previously installed skill and orphaned dependencies",
		Long:    "Removes the named skill from the given --target (cursor or claude) and removes any dependencies that are no longer required by other installed skills.",
		Example: "  striatum skill uninstall --target cursor my-skill",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			target = strings.TrimSpace(target)
			if target == "" {
				return fmt.Errorf("--target is required (cursor or claude)")
			}
			if target != "cursor" && target != "claude" {
				return fmt.Errorf("--target must be cursor or claude, got %q", target)
			}
			normProject := ""
			if projectPath != "" {
				abs, err := filepath.Abs(strings.TrimSpace(projectPath))
				if err != nil {
					return fmt.Errorf("resolve project path %q: %w", projectPath, err)
				}
				normProject = abs
			}
			return runUninstall(cmd, name, target, normProject)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Uninstall from target: cursor or claude (required)")
	cmd.Flags().StringVar(&projectPath, "project", "", "Project path (match project-level install)")
	return cmd
}

func runUninstall(cmd *cobra.Command, name, target, normProject string) error {
	entries, err := installer.LoadInstalled()
	if err != nil {
		return fmt.Errorf("load installed: %w", err)
	}
	if entries == nil {
		entries = []installer.InstalledEntry{}
	}

	var toRemove []installer.InstalledEntry
	for _, e := range entries {
		if e.Skill != name {
			continue
		}
		if e.Target != target {
			continue
		}
		if e.ProjectPath != normProject {
			continue
		}
		toRemove = append(toRemove, e)
	}
	if len(toRemove) == 0 {
		return fmt.Errorf("skill %q is not installed for target %s", name, target)
	}

	// Remove target dirs for toRemove
	for _, e := range toRemove {
		targetDir, err := installer.Targets(e.Target, e.ProjectPath)
		if err != nil {
			return fmt.Errorf("resolve target for %s: %w", e.Skill, err)
		}
		if err := installer.RemoveFromTarget(targetDir, e.Skill); err != nil {
			return fmt.Errorf("remove %s from target: %w", e.Skill, err)
		}
	}

	// Remove toRemove from entries
	remaining := make([]installer.InstalledEntry, 0, len(entries))
	for _, e := range entries {
		keep := true
		for _, r := range toRemove {
			if e.Skill == r.Skill && e.Target == r.Target && e.ProjectPath == r.ProjectPath {
				keep = false
				break
			}
		}
		if keep {
			remaining = append(remaining, e)
		}
	}

	// Orphan cleanup: remove entries that are no longer required by any root
	const maxOrphanPasses = 100
	for pass := 0; pass < maxOrphanPasses; pass++ {
		orphans, hadUnloadableRoot := computeOrphans(remaining)
		if hadUnloadableRoot {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Could not load manifest for some installed roots; skipping orphan cleanup")
			break
		}
		if len(orphans) == 0 {
			break
		}
		for _, e := range orphans {
			targetDir, err := installer.Targets(e.Target, e.ProjectPath)
			if err != nil {
				return fmt.Errorf("resolve target for orphan %s: %w", e.Skill, err)
			}
			if err := installer.RemoveFromTarget(targetDir, e.Skill); err != nil {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: could not remove orphan", e.Skill, "from target:", err)
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

// computeOrphans returns entries that are no longer required by any root in the same target/project.
// If any root's manifest cannot be loaded from cache, it returns (nil, true) so the caller
// skips orphan removal (conservative: treat unknown deps as required).
func computeOrphans(entries []installer.InstalledEntry) (orphans []installer.InstalledEntry, hadUnloadableRoot bool) {
	// required[target|project] = set of skill names required in that context
	type key string
	required := make(map[key]map[string]bool)
	for _, e := range entries {
		if e.InstalledWith != "" {
			continue
		}
		ctxKey := key(e.Target + "|" + e.ProjectPath)
		if required[ctxKey] == nil {
			required[ctxKey] = make(map[string]bool)
		}
		required[ctxKey][e.Skill] = true
		cacheDir := installer.CacheDir(e.Skill, e.Version)
		m, err := artifact.Load(filepath.Join(cacheDir, "artifact.json"))
		if err != nil {
			hadUnloadableRoot = true
			continue
		}
		for _, d := range m.Dependencies {
			required[ctxKey][d.Name] = true
		}
	}
	if hadUnloadableRoot {
		return nil, true
	}
	for _, e := range entries {
		ctxKey := key(e.Target + "|" + e.ProjectPath)
		if required[ctxKey] != nil && required[ctxKey][e.Skill] {
			continue
		}
		orphans = append(orphans, e)
	}
	return orphans, false
}

func removeEntries(entries, toRemove []installer.InstalledEntry) []installer.InstalledEntry {
	var out []installer.InstalledEntry
	for _, e := range entries {
		keep := true
		for _, r := range toRemove {
			if e.Skill == r.Skill && e.Target == r.Target && e.ProjectPath == r.ProjectPath {
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
