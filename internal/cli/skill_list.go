package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/spf13/cobra"
)

func newSkillListCmd() *cobra.Command {
	var installed bool
	var target string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List skills in cache or installed",
		Long:    "List skills in the local cache. Use --installed to list installed skills (optionally filter by --target).",
		Example: "  striatum skill list\n  striatum skill list --installed --target cursor",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillList(cmd, installed, target)
		},
	}
	cmd.Flags().BoolVar(&installed, "installed", false, "List installed skills instead of cache")
	cmd.Flags().StringVar(&target, "target", "", "Filter installed list by target (cursor or claude); only with --installed")
	return cmd
}

func runSkillList(cmd *cobra.Command, installed bool, target string) error {
	if !installed && target != "" {
		return fmt.Errorf("--target is only valid with --installed")
	}
	out := cmd.OutOrStdout()
	if installed {
		if target != "" && target != "cursor" && target != "claude" {
			return fmt.Errorf("--target must be cursor or claude, got %q", target)
		}
		entries, err := installer.LoadInstalled()
		if err != nil {
			return fmt.Errorf("load installed: %w", err)
		}
		if target != "" {
			filtered := entries[:0]
			for _, e := range entries {
				if e.Target == target {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}
		if len(entries) == 0 {
			_, _ = fmt.Fprintln(out, "No installed skills.")
			return nil
		}
		writeAlignedTable(out, []string{"NAME", "VERSION", "TARGET", "INSTALLED_WITH"}, func(w io.Writer) {
			for _, e := range entries {
				with := e.InstalledWith
				if with == "" {
					with = "-"
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Skill, e.Version, e.Target, with)
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
