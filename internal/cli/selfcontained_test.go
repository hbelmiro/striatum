package cli

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	orasoci "oras.land/oras-go/v2/content/oci"
)

// seedExportDep writes a dependency artifact into the Striatum cache so the
// cache-first fetcher resolves it without network access.
func seedExportDep(t *testing.T, home, kind, name, version string, files map[string]string, deps ...artifact.Dependency) {
	t.Helper()
	dir := filepath.Join(home, "cache", kind, name+"@"+version)
	m := &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha2",
		Kind:         kind,
		Metadata:     artifact.Metadata{Name: name, Version: version},
		Spec:         artifact.Spec{Entrypoint: entrypointOf(kind), Files: sortedKeys(files)},
		Dependencies: deps,
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for f, c := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func entrypointOf(kind string) string {
	switch kind {
	case "Prompt":
		return "PROMPT.md"
	default:
		return "SKILL.md"
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, _, err := runCLIWithErr(t, args...)
	return out, err
}

// runCLIWithErr also captures stderr, for tests asserting the diagnostics
// half of output contracts.
func runCLIWithErr(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	out := &strings.Builder{}
	errOut := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetErr(errOut)
	// The stable stdout contract under test is striatum's own output, not
	// cobra's usage dump on errors (which goes to stderr in the real CLI).
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func openExportLayout(t *testing.T, layoutDir, tag string) (*orasoci.Store, map[string]ocispec.Descriptor) {
	t.Helper()
	store, err := orasoci.New(layoutDir)
	if err != nil {
		t.Fatalf("open layout: %v", err)
	}
	ctx := context.Background()
	desc, err := store.Resolve(ctx, tag)
	if err != nil {
		t.Fatalf("resolve %q: %v", tag, err)
	}
	data, err := content.FetchAll(ctx, store, desc)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Layers []ocispec.Descriptor `json:"layers"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	titles := map[string]ocispec.Descriptor{}
	for _, l := range manifest.Layers {
		titles[l.Annotations["org.opencontainers.image.title"]] = l
	}
	return store, titles
}

func TestPackExport_StandaloneSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	dir := t.TempDir()
	t.Chdir(dir)

	writeLocalSkill(t, dir, "export-solo", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: export-solo\n---\n# Solo\n",
	})

	out, err := runCLI(t, "pack", "--self-contained", "-o", filepath.Join(dir, "dist"))
	if err != nil {
		t.Fatalf("pack --self-contained: %v (out: %s)", err, out)
	}
	_, titles := openExportLayout(t, filepath.Join(dir, "dist"), "export-solo:1.0.0")
	if _, ok := titles["SKILL.md"]; !ok {
		t.Errorf("layers = %v, want root SKILL.md", titles)
	}
	if _, ok := titles[".striatum/provenance.json"]; !ok {
		t.Errorf("layers = %v, want provenance layer", titles)
	}
}

func TestPackExport_DependencyClosureAndRewriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	dir := t.TempDir()
	t.Chdir(dir)

	// generic-review depends on the review-policy prompt; pr-review (root)
	// depends on generic-review.
	seedExportDep(t, home, "Prompt", "review-policy", "1.0.0", map[string]string{
		"PROMPT.md":        "policy body",
		"examples/case.md": "case",
	})
	seedExportDep(t, home, "Skill", "generic-review", "1.0.0", map[string]string{
		"SKILL.md": "---\nname: generic-review\n---\nUses [policy](../../prompts/review-policy/PROMPT.md).\n",
	}, &artifact.OCIDependency{RegistryHost: "reg.invalid", Repository: "prompts/review-policy", Tag: "1.0.0"})

	root := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "pr-review", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "reg.invalid", Repository: "skills/generic-review", Tag: "1.0.0"},
		},
	}
	rootJSON, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(dir, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	rootSkill := "---\nname: pr-review\n---\nSee [gr](../generic-review/SKILL.md).\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(rootSkill), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(dir, "dist")
	out, err := runCLI(t, "pack", "--self-contained", "-o", layout)
	if err != nil {
		t.Fatalf("pack --self-contained: %v (out: %s)", err, out)
	}
	store, titles := openExportLayout(t, layout, "pr-review:1.0.0")
	for _, want := range []string{
		"SKILL.md",
		"deps/skills/generic-review/SKILL.md",
		"deps/prompts/review-policy/PROMPT.md",
		"deps/prompts/review-policy/examples/case.md",
		".striatum/provenance.json",
	} {
		if _, ok := titles[want]; !ok {
			t.Errorf("layers missing %q (have %v)", want, titles)
		}
	}

	// Root link rewritten to the in-tree dep location.
	rootData := fetchLayerByTitle(t, store, titles, "SKILL.md")
	if !strings.Contains(rootData, "](deps/skills/generic-review/SKILL.md)") {
		t.Errorf("root SKILL.md not rewritten: %q", rootData)
	}
	depData := fetchLayerByTitle(t, store, titles, "deps/skills/generic-review/SKILL.md")
	if !strings.Contains(depData, "](../../prompts/review-policy/PROMPT.md)") {
		t.Errorf("dep SKILL.md not rewritten: %q", depData)
	}

	// Original sources untouched.
	after, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if string(after) != rootSkill {
		t.Errorf("source SKILL.md mutated: %q", after)
	}
	depSrc, _ := os.ReadFile(filepath.Join(home, "cache", "Skill", "generic-review@1.0.0", "SKILL.md"))
	if !strings.Contains(string(depSrc), "../../prompts/review-policy/PROMPT.md") {
		t.Errorf("dep source mutated: %q", depSrc)
	}
}

func fetchLayerByTitle(t *testing.T, store *orasoci.Store, titles map[string]ocispec.Descriptor, title string) string {
	t.Helper()
	ctx := context.Background()
	desc, ok := titles[title]
	if !ok {
		t.Fatalf("no layer titled %q", title)
	}
	data, err := content.FetchAll(ctx, store, desc)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPackExport_NonSkillRejected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	dir := t.TempDir()
	t.Chdir(dir)
	writeLocalArtifact(t, dir, "Prompt", "not-a-skill", "1.0.0", "PROMPT.md", map[string]string{"PROMPT.md": "x"})
	_, err := runCLI(t, "pack", "--self-contained", "-o", filepath.Join(dir, "dist"))
	if err == nil || !strings.Contains(err.Error(), "Skill") {
		t.Fatalf("err = %v, want non-Skill rejection", err)
	}
}

func TestPushSelfContained_PrintDigestFailureWritesNoStdout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	dir := t.TempDir()
	t.Chdir(dir)
	writeLocalSkill(t, dir, "push-fail", "1.0.0", "SKILL.md", map[string]string{"SKILL.md": "---\nname: push-fail\n---\nx\n"})

	out, err := runCLI(t, "push", "--self-contained", "--print-digest", "localhost:1/repo/push-fail:1.0.0")
	if err == nil {
		t.Fatal("expected push failure for unreachable registry")
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("stdout = %q, want empty on failure", out)
	}
}

func TestPush_PrintDigestFailureWritesNoStdout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	dir := t.TempDir()
	t.Chdir(dir)
	writeLocalSkill(t, dir, "push-fail-2", "1.0.0", "SKILL.md", map[string]string{"SKILL.md": "---\nname: push-fail-2\n---\nx\n"})

	out, err := runCLI(t, "push", "--print-digest", "localhost:1/repo/push-fail-2:1.0.0")
	if err == nil {
		t.Fatal("expected push failure for unreachable registry")
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("stdout = %q, want empty on failure", out)
	}
}

func TestPushSelfContained_PrintDigestPrepareFailureWritesNoStdout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	dir := t.TempDir()
	t.Chdir(dir)
	// A Prompt root cannot be exported: prepare fails before any push.
	writeLocalArtifact(t, dir, "not-exportable", "Prompt", "1.0.0", "PROMPT.md", map[string]string{"PROMPT.md": "x"})

	out, err := runCLI(t, "push", "--self-contained", "--print-digest", "localhost:5000/repo/not-exportable:1.0.0")
	if err == nil || !strings.Contains(err.Error(), "Skill") {
		t.Fatalf("err = %v, want non-Skill root rejection", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("stdout = %q, want empty on prepare failure", out)
	}
}

var linkDestRegex = regexp.MustCompile(`\]\(([^)\s]+)[^)]*\)`)

// TestPackExport_FixPRCommentsScaleFixture mirrors the shape of the
// ai-skills fix-pr-comments skill: a root Skill with 11 transitive
// dependencies across Skills and Prompts (23 declared files with the root
// plus helper assets), cross-referenced and installed with the dependency
// sources unavailable.
func TestPackExport_FixPRCommentsScaleFixture(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	// 6 skills, each with SKILL.md + a helper asset; skills 2..6 also depend
	// on their predecessor skill.
	for i := 1; i <= 6; i++ {
		name := fmt.Sprintf("scale-skill-%d", i)
		files := map[string]string{
			"SKILL.md":          fmt.Sprintf("---\nname: %s\n---\nUses [p1](../../prompts/scale-prompt-1/PROMPT.md).\n", name),
			"helpers/rubric.md": "rubric",
		}
		deps := []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "reg.invalid", Repository: "prompts/scale-prompt-1", Tag: "1.0.0"},
		}
		if i >= 2 {
			deps = append(deps, &artifact.OCIDependency{RegistryHost: "reg.invalid", Repository: fmt.Sprintf("skills/scale-skill-%d", i-1), Tag: "1.0.0"})
		}
		seedExportDep(t, home, "Skill", name, "1.0.0", files, deps...)
	}
	// 5 prompts, each with PROMPT.md + an example; prompts 2..5 depend on prompt-1.
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("scale-prompt-%d", i)
		files := map[string]string{"PROMPT.md": "policy", "examples/case.md": "case"}
		var deps []artifact.Dependency
		if i >= 2 {
			deps = append(deps, &artifact.OCIDependency{RegistryHost: "reg.invalid", Repository: "prompts/scale-prompt-1", Tag: "1.0.0"})
		}
		seedExportDep(t, home, "Prompt", name, "1.0.0", files, deps...)
	}

	// Root depends on skill-6 (which transitively pulls skills 1..5) plus all
	// five prompts: 11 distinct dependencies.
	rootDeps := []artifact.Dependency{
		&artifact.OCIDependency{RegistryHost: "reg.invalid", Repository: "skills/scale-skill-6", Tag: "1.0.0"},
	}
	for i := 1; i <= 5; i++ {
		rootDeps = append(rootDeps, &artifact.OCIDependency{
			RegistryHost: "reg.invalid", Repository: fmt.Sprintf("prompts/scale-prompt-%d", i), Tag: "1.0.0",
		})
	}
	root := &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha2",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: "scale-review", Version: "1.0.0"},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: rootDeps,
	}
	rootJSON, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: scale-review\n---\nSee [s1](../scale-skill-1/SKILL.md) and [p5](../../prompts/scale-prompt-5/PROMPT.md).\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(project, "dist")
	if out, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatalf("pack --self-contained: %v (out: %s)", err, out)
	}
	_, titles := openExportLayout(t, layout, "scale-review:1.0.0")

	// 1 root file + 6 skills x 2 files + 5 prompts x 2 files + provenance.
	wantCount := 1 + 6*2 + 5*2 + 1
	if len(titles) != wantCount {
		t.Errorf("layer count = %d, want %d (titles: %v)", len(titles), wantCount, titles)
	}
	for _, want := range []string{
		"deps/skills/scale-skill-6/SKILL.md",
		"deps/skills/scale-skill-6/helpers/rubric.md",
		"deps/prompts/scale-prompt-5/PROMPT.md",
		"deps/prompts/scale-prompt-5/examples/case.md",
		".striatum/provenance.json",
	} {
		if _, ok := titles[want]; !ok {
			t.Errorf("layers missing %q", want)
		}
	}

	// Install with dependency sources unavailable; every Markdown link in
	// the installed tree must resolve to an existing file inside the tree.
	for _, kind := range []string{"Skill", "Prompt"} {
		if err := os.RemoveAll(filepath.Join(home, "cache", kind)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runCLI(t, "install", "--target", "claude", "oci:"+layout+":scale-review:1.0.0"); err != nil {
		t.Fatalf("install exported scale fixture: %v", err)
	}
	targetDir := filepath.Join(home, ".claude", "skills", "scale-review")
	rootMD, err := os.ReadFile(filepath.Join(targetDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rootMD), "](deps/skills/scale-skill-1/SKILL.md)") {
		t.Errorf("root links not rewritten: %q", rootMD)
	}
	if !strings.Contains(string(rootMD), "](deps/prompts/scale-prompt-5/PROMPT.md)") {
		t.Errorf("root prompt link not rewritten: %q", rootMD)
	}
	_ = filepath.WalkDir(targetDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, _ := os.ReadFile(path)
		for _, m := range linkDestRegex.FindAllStringSubmatch(string(data), -1) {
			dest := m[1]
			if strings.Contains(dest, "://") || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "/") {
				continue
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), dest))
			if !strings.HasPrefix(resolved, targetDir+string(filepath.Separator)) {
				t.Errorf("%s: link %q escapes install dir", path, dest)
				return nil
			}
			if _, err := os.Stat(resolved); err != nil {
				t.Errorf("%s: link %q does not resolve (target %q missing)", path, dest, resolved)
			}
		}
		return nil
	})
}
