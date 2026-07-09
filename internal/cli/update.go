package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
	"github.com/hbelmiro/striatum/pkg/semver"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [artifact-name...]",
		Short: "Update installed artifacts to their latest available versions",
		Long: `Checks the OCI registry for newer versions of installed artifacts and
upgrades them in place. Without arguments, updates all installed artifacts.
With arguments, updates only the named artifacts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd, args)
		},
	}
	cmd.Flags().Bool("check", false, "List outdated artifacts without installing")
	cmd.Flags().StringP("target", "t", "", "Filter by install target (cursor or claude)")
	cmd.Flags().String("project", "", "Filter by project path")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	return cmd
}

type updateCandidate struct {
	entry    installer.InstalledEntry
	latest   string
	repoRef  string
	outdated bool
}

func runUpdate(cmd *cobra.Command, args []string) error {
	check, _ := cmd.Flags().GetBool("check")
	yes, _ := cmd.Flags().GetBool("yes")
	targetFilter, _ := cmd.Flags().GetString("target")
	projectFilter, _ := cmd.Flags().GetString("project")

	if targetFilter != "" {
		if _, err := validateTarget(targetFilter); err != nil {
			return err
		}
	}

	entries, err := installer.LoadInstalled()
	if err != nil {
		return fmt.Errorf("load installed: %w", err)
	}
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Nothing installed.")
		return nil
	}

	filtered := filterEntries(entries, targetFilter, projectFilter, args)
	if len(filtered) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No matching artifacts found.")
		return nil
	}

	ctx := cmd.Context()
	candidates, err := discoverUpdates(ctx, filtered, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	var outdated []updateCandidate
	for _, c := range candidates {
		if c.outdated {
			outdated = append(outdated, c)
		}
	}

	if len(outdated) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "==> All artifacts are up to date.")
		return nil
	}

	if check {
		printOutdatedSummary(cmd.OutOrStdout(), outdated)
		return nil
	}

	if !yes {
		printOutdatedSummary(cmd.OutOrStdout(), outdated)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nProceed? [y/N] ")
		reader := bufio.NewReader(cmd.InOrStdin())
		line, _ := reader.ReadString('\n')
		answer := strings.TrimSpace(strings.ToLower(line))
		if answer != "y" && answer != "yes" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	return performUpdates(cmd, outdated)
}

type entryKey struct {
	Name        string
	Target      string
	ProjectPath string
}

func filterEntries(entries []installer.InstalledEntry, target, project string, names []string) []installer.InstalledEntry {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	seen := make(map[entryKey]bool)
	var result []installer.InstalledEntry
	for _, e := range entries {
		if target != "" && e.Target != target {
			continue
		}
		if project != "" && e.ProjectPath != project {
			continue
		}
		if len(nameSet) > 0 && !nameSet[e.Name] {
			continue
		}
		key := entryKey{Name: e.Name, Target: e.Target, ProjectPath: e.ProjectPath}
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, e)
	}
	return result
}

func registryRepoRef(registryField string) string {
	if registryField == "" {
		return ""
	}
	if strings.HasPrefix(registryField, "oci:") {
		rest := registryField[len("oci:"):]
		i := strings.Index(rest, ":")
		if i < 0 {
			return registryField
		}
		if i == 1 && len(rest) > 2 && (rest[2] == '\\' || rest[2] == '/') {
			i = strings.LastIndex(rest, ":")
		}
		return "oci:" + rest[:i]
	}
	if strings.HasPrefix(registryField, "git:") {
		return ""
	}
	i := strings.LastIndex(registryField, ":")
	if i < 0 {
		return registryField
	}
	return registryField[:i]
}

func discoverUpdates(ctx context.Context, entries []installer.InstalledEntry, stdout, stderr io.Writer) ([]updateCandidate, error) {
	tagCache := make(map[string][]string)
	var candidates []updateCandidate

	for _, e := range entries {
		repoRef := registryRepoRef(e.Registry)
		if repoRef == "" && strings.HasPrefix(e.Registry, "git:") {
			_, _ = fmt.Fprintf(stdout, "Checking %s (git)...\n", e.Name)
			_, _ = fmt.Fprintf(stdout, "==> %s: installed from git, not auto-updatable.\n", e.Name)
			candidates = append(candidates, updateCandidate{
				entry:   e,
				latest:  e.Version,
				repoRef: "",
			})
			continue
		}
		_, _ = fmt.Fprintf(stdout, "Checking %s (oci)...\n", e.Name)
		if repoRef == "" {
			candidates = append(candidates, updateCandidate{
				entry:   e,
				latest:  e.Version,
				repoRef: "",
			})
			continue
		}

		tags, ok := tagCache[repoRef]
		if !ok {
			var err error
			tags, err = oci.ListTags(ctx, repoRef)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "Warning: could not list tags for %s: %v\n", repoRef, err)
				candidates = append(candidates, updateCandidate{
					entry:   e,
					latest:  e.Version,
					repoRef: repoRef,
				})
				continue
			}
			tagCache[repoRef] = tags
		}

		latest := semver.LatestVersion(tags)
		if latest == "" {
			latest = e.Version
		}

		outdated := semver.Compare(e.Version, latest) < 0
		candidates = append(candidates, updateCandidate{
			entry:    e,
			latest:   latest,
			repoRef:  repoRef,
			outdated: outdated,
		})
	}
	return candidates, nil
}

func printOutdatedSummary(out io.Writer, outdated []updateCandidate) {
	_, _ = fmt.Fprintf(out, "==> %d outdated artifact(s):\n", len(outdated))
	maxName := 0
	for _, c := range outdated {
		if len(c.entry.Name) > maxName {
			maxName = len(c.entry.Name)
		}
	}
	for _, c := range outdated {
		_, _ = fmt.Fprintf(out, "%-*s  %s  →  %s\n", maxName, c.entry.Name, c.entry.Version, c.latest)
	}
}

func performUpdates(cmd *cobra.Command, outdated []updateCandidate) error {
	var updated, failed int
	for _, c := range outdated {
		newRef := c.repoRef + ":" + c.latest
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "==> Updating %s (%s → %s)\n", c.entry.Name, c.entry.Version, c.latest)

		root := NewRootCommand()
		silenceRootPresentation(root)
		root.SetOut(cmd.OutOrStdout())
		root.SetErr(cmd.ErrOrStderr())

		installArgs := []string{"install", "--target", c.entry.Target, newRef}
		if c.entry.ProjectPath != "" {
			installArgs = append(installArgs, "--project", c.entry.ProjectPath)
		}
		installArgs = append(installArgs, "--force")
		root.SetArgs(installArgs)
		if err := root.Execute(); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update %s: %v\n", c.entry.Name, err)
			failed++
			continue
		}
		updated++
	}
	if updated > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "==> Updated %d artifact(s).\n", updated)
	}
	if failed > 0 {
		return fmt.Errorf("%d artifact(s) failed to update", failed)
	}
	return nil
}
