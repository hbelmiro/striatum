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
		Use:   "uninstall",
		Short: "Remove a previously installed skill and orphaned dependencies",
		Args:  cobra.ExactArgs(1),
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
					return err
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
		return err
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
			return err
		}
		_ = installer.RemoveFromTarget(targetDir, e.Skill)
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
	for {
		orphans := computeOrphans(remaining)
		if len(orphans) == 0 {
			break
		}
		for _, e := range orphans {
			targetDir, _ := installer.Targets(e.Target, e.ProjectPath)
			_ = installer.RemoveFromTarget(targetDir, e.Skill)
		}
		remaining = removeEntries(remaining, orphans)
	}

	if err := installer.SaveInstalled(remaining); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Removed", name)
	return nil
}

func computeOrphans(entries []installer.InstalledEntry) []installer.InstalledEntry {
	required := make(map[string]bool)
	for _, e := range entries {
		if e.InstalledWith != "" {
			continue
		}
		required[e.Skill] = true
		cacheDir := installer.CacheDir(e.Skill, e.Version)
		m, err := artifact.Load(filepath.Join(cacheDir, "artifact.json"))
		if err != nil {
			continue
		}
		for _, d := range m.Dependencies {
			required[d.Name] = true
		}
	}
	var orphans []installer.InstalledEntry
	for _, e := range entries {
		if required[e.Skill] {
			continue
		}
		orphans = append(orphans, e)
	}
	return orphans
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
