package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/spf13/cobra"
)

func newSkillListCmd() *cobra.Command {
	var installed bool
	var target string
	var projectPath string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List skills in cache or installed",
		Long:    "List skills in the local cache. Use --installed to list installed skills (optionally filter by --target or --project).",
		Example: "  striatum skill list\n  striatum skill list --installed --target cursor\n  striatum skill list --installed --project .",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillList(cmd, installed, target, projectPath)
		},
	}
	cmd.Flags().BoolVar(&installed, "installed", false, "List installed skills instead of cache")
	cmd.Flags().StringVar(&target, "target", "", "Filter installed list by target (cursor or claude); only with --installed")
	cmd.Flags().StringVar(&projectPath, "project", "", "Filter installed list by project path; only with --installed")
	return cmd
}

func runSkillList(cmd *cobra.Command, installed bool, target string, projectPath string) error {
	if !installed && target != "" {
		return fmt.Errorf("--target is only valid with --installed")
	}
	if !installed && projectPath != "" {
		return fmt.Errorf("--project is only valid with --installed")
	}
	out := cmd.OutOrStdout()
	if installed {
		if target != "" && target != "cursor" && target != "claude" {
			return fmt.Errorf("--target must be cursor or claude, got %q", target)
		}

		// Normalize project path for filtering
		normProject := ""
		if projectPath != "" {
			abs, err := filepath.Abs(strings.TrimSpace(projectPath))
			if err != nil {
				return fmt.Errorf("resolve project path %q: %w", projectPath, err)
			}
			normProject = abs
		}

		entries, err := installer.LoadInstalled()
		if err != nil {
			return fmt.Errorf("load installed: %w", err)
		}

		// Filter by target if specified
		if target != "" {
			filtered := entries[:0]
			for _, e := range entries {
				if e.Target == target {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}

		// Filter by project path if specified
		if normProject != "" {
			filtered := entries[:0]
			for _, e := range entries {
				if e.ProjectPath == normProject {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}

		if len(entries) == 0 {
			_, _ = fmt.Fprintln(out, "No installed skills.")
			return nil
		}
		writeAlignedTable(out, []string{"NAME", "VERSION", "TARGET", "SCOPE", "INSTALLED_WITH"}, func(w io.Writer) {
			for _, e := range entries {
				with := e.InstalledWith
				if with == "" {
					with = "-"
				}
				scope := "global"
				if e.ProjectPath != "" {
					scope = e.ProjectPath
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.Skill, e.Version, e.Target, scope, with)
			}
		})
		return nil
	}
	skills, err := installer.ListCachedSkills()
	if err != nil {
		return fmt.Errorf("list cache: %w", err)
	}
	if len(skills) == 0 {
		_, _ = fmt.Fprintln(out, "No skills in cache.")
		return nil
	}
	writeAlignedTable(out, []string{"NAME", "VERSION", "DESCRIPTION"}, func(w io.Writer) {
		for _, s := range skills {
			desc := s.Description
			if desc == "" {
				desc = "-"
			}
			desc = strings.ReplaceAll(desc, "\n", " ")
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.Version, desc)
		}
	})
	return nil
}

// writeAlignedTable writes a header row and then body rows to out using tabwriter for column alignment.
func writeAlignedTable(out io.Writer, headers []string, writeRows func(io.Writer)) {
	tw := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))
	writeRows(tw)
	_ = tw.Flush()
}
