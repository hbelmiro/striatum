package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
)

// buildSelfContainedLayout packs a pr-review-like self-contained image into an OCI layout and
// returns the layout path and the oci: install reference. Dependency caches
// are removed afterwards so installs cannot rely on the original sources.
func buildSelfContainedLayout(t *testing.T, home, project string) string {
	t.Helper()
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
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: pr-review\n---\nSee [gr](../generic-review/SKILL.md).\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(project, "dist")
	if out, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatalf("pack --self-contained: %v (out: %s)", err, out)
	}
	// Dependency sources become unavailable.
	if err := os.RemoveAll(filepath.Join(home, "cache", "Skill")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(home, "cache", "Prompt")); err != nil {
		t.Fatal(err)
	}
	return layout
}

func installSelfContainedFixture(t *testing.T) (home, layout, installRef string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)
	layout = buildSelfContainedLayout(t, home, project)
	return home, layout, "oci:" + layout + ":pr-review:1.0.0"
}

func TestInstall_SelfContainedImageFromOCILayout(t *testing.T) {
	home, _, installRef := installSelfContainedFixture(t)

	out, err := runCLI(t, "install", "--target", "claude", installRef)
	if err != nil {
		t.Fatalf("install self-contained: %v (out: %s)", err, out)
	}

	targetDir := filepath.Join(home, ".claude", "skills", "pr-review")
	root, err := os.ReadFile(filepath.Join(targetDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("installed root SKILL.md missing: %v", err)
	}
	if !strings.Contains(string(root), "](deps/skills/generic-review/SKILL.md)") {
		t.Errorf("installed root links not contained: %q", root)
	}
	// The link target must actually exist: references work post-install.
	for _, f := range []string{
		"deps/skills/generic-review/SKILL.md",
		"deps/prompts/review-policy/PROMPT.md",
		"deps/prompts/review-policy/examples/case.md",
		".striatum/provenance.json",
	} {
		if _, err := os.Stat(filepath.Join(targetDir, filepath.FromSlash(f))); err != nil {
			t.Errorf("installed file %q missing: %v", f, err)
		}
	}
	dep, _ := os.ReadFile(filepath.Join(targetDir, "deps", "skills", "generic-review", "SKILL.md"))
	if !strings.Contains(string(dep), "](../../prompts/review-policy/PROMPT.md)") {
		t.Errorf("installed dep links not contained: %q", dep)
	}

	// Isolated self-contained cache; no ordinary cache entry for the root.
	if _, err := os.Stat(filepath.Join(installer.SelfContainedCacheDir("pr-review", "1.0.0"), "artifact.json")); err != nil {
		t.Errorf("self-contained cache missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "cache", "Skill", "pr-review@1.0.0")); !os.IsNotExist(err) {
		t.Errorf("ordinary cache entry created for self-contained install: %v", err)
	}

	// DB entry marked as self-contained with the immutable source ref.
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "pr-review" {
			found = true
			if !e.SelfContained {
				t.Errorf("entry not marked self-contained: %+v", e)
			}
			if e.Registry == "" {
				t.Errorf("entry has no source ref: %+v", e)
			}
		}
	}
	if !found {
		t.Fatalf("no DB entry for pr-review: %+v", entries)
	}
}

func TestInstall_SelfContainedCacheNotSubstitutedByOrdinaryCache(t *testing.T) {
	home, _, installRef := installSelfContainedFixture(t)

	// Poison the ordinary cache with different content for the same name@version.
	poison := installer.CacheDir("Skill", "pr-review", "1.0.0")
	if err := os.MkdirAll(poison, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLocalSkill(t, poison, "pr-review", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: pr-review\n---\nPOISONED ordinary cache\n",
	})

	if _, err := runCLI(t, "install", "--target", "claude", installRef); err != nil {
		t.Fatalf("install self-contained: %v", err)
	}
	installed, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "pr-review", "SKILL.md"))
	if strings.Contains(string(installed), "POISONED") {
		t.Fatalf("ordinary cache substituted for self-contained image: %q", installed)
	}
	if !strings.Contains(string(installed), "](deps/skills/generic-review/SKILL.md)") {
		t.Fatalf("installed content is not the self-contained payload: %q", installed)
	}
}

func TestInstall_SelfContainedByLayoutRefNotShadowedOrContaminatedByOrdinaryCache(t *testing.T) {
	home, _, installRef := installSelfContainedFixture(t)

	// An ordinary cache entry for the same name@version must neither be
	// installed in place of the self-contained image nor overwritten by the self-contained payload.
	poison := installer.CacheDir("Skill", "pr-review", "1.0.0")
	if err := os.MkdirAll(poison, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLocalSkill(t, poison, "pr-review", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: pr-review\n---\nPOISONED ordinary cache\n",
	})

	if _, err := runCLI(t, "install", "--target", "claude", installRef); err != nil {
		t.Fatalf("install self-contained by tag: %v", err)
	}

	// The self-contained payload is installed, not the ordinary cache content.
	installed, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "pr-review", "SKILL.md"))
	if strings.Contains(string(installed), "POISONED") {
		t.Fatalf("ordinary cache content substituted for self-contained image: %q", installed)
	}
	if !strings.Contains(string(installed), "](deps/skills/generic-review/SKILL.md)") {
		t.Fatalf("installed content is not the self-contained payload: %q", installed)
	}

	// The ordinary cache entry is untouched: no deps/ or .striatum/ leakage.
	if _, err := os.Stat(filepath.Join(poison, "deps")); err == nil {
		t.Errorf("self-contained payload contaminated the ordinary cache (deps/ present)")
	}
	if _, err := os.Stat(filepath.Join(poison, ".striatum")); err == nil {
		t.Errorf("self-contained payload contaminated the ordinary cache (.striatum/ present)")
	}
	poisonFile, err := os.ReadFile(filepath.Join(poison, "SKILL.md"))
	if err != nil || !strings.Contains(string(poisonFile), "POISONED") {
		t.Errorf("ordinary cache entry was modified: %q (%v)", poisonFile, err)
	}

	// The payload is cached in the isolated self-contained cache and marked self-contained.
	if _, err := os.Stat(installer.SelfContainedCacheDir("pr-review", "1.0.0")); err != nil {
		t.Errorf("self-contained cache missing: %v", err)
	}
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "pr-review" {
			found = e.SelfContained
		}
	}
	if !found {
		t.Errorf("DB entry for pr-review not marked exported: %+v", entries)
	}
}

func TestReinstall_SelfContainedEntry(t *testing.T) {
	home, layout, installRef := installSelfContainedFixture(t)

	if _, err := runCLI(t, "install", "--target", "claude", installRef); err != nil {
		t.Fatalf("install self-contained: %v", err)
	}
	// Simulate cache eviction and a removed install dir.
	if err := os.RemoveAll(installer.SelfContainedCacheDir("pr-review", "1.0.0")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(home, ".claude", "skills", "pr-review")); err != nil {
		t.Fatal(err)
	}
	// The original tag "moves": pack different content under the same tag in
	// the same layout. Reinstall must still restore the originally installed
	// image (the DB stores its immutable digest reference).
	movedDir := t.TempDir()
	if err := os.MkdirAll(movedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movedDir, "SKILL.md"), []byte("---\nname: pr-review\n---\nMOVED tag content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pi := oci.PreparedImage{
		Config:              mustMarkedConfig(t, "pr-review"),
		ArtifactType:        oci.ArtifactTypeForKind("Skill"),
		Dir:                 movedDir,
		Files:               []string{"SKILL.md"},
		ManifestAnnotations: map[string]string{"dev.striatum.self-contained": "v1"},
	}
	if _, err := oci.PackPreparedToLayout(t.Context(), pi, layout, "pr-review:1.0.0"); err != nil {
		t.Fatalf("move tag in layout: %v", err)
	}

	if _, err := runCLI(t, "install", "--reinstall-all"); err != nil {
		t.Fatalf("reinstall-all: %v", err)
	}
	restored, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "pr-review", "SKILL.md"))
	if err != nil {
		t.Fatalf("reinstall did not restore self-contained image: %v", err)
	}
	if strings.Contains(string(restored), "MOVED tag content") {
		t.Fatalf("reinstall picked up the moved tag instead of the digest-pinned image: %q", restored)
	}
	if !strings.Contains(string(restored), "](deps/skills/generic-review/SKILL.md)") {
		t.Fatalf("reinstalled content is not the self-contained payload: %q", restored)
	}
	if _, err := os.Stat(installer.SelfContainedCacheDir("pr-review", "1.0.0")); err != nil {
		t.Errorf("self-contained cache not restored: %v", err)
	}
}

func TestInspect_SelfContainedImage(t *testing.T) {
	home, layout, _ := installSelfContainedFixture(t)
	_ = home

	out, err := runCLI(t, "inspect", "oci:"+layout+":pr-review:1.0.0")
	if err != nil {
		t.Fatalf("inspect self-contained: %v", err)
	}
	for _, want := range []string{
		"Self-contained: v1 (dev.striatum.self-contained)",
		"Bundled dependencies:",
		"- Skill generic-review@1.0.0 from reg.invalid/skills/generic-review:1.0.0",
		"- Prompt review-policy@1.0.0 from reg.invalid/prompts/review-policy:1.0.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("inspect output missing %q:\n%s", want, out)
		}
	}
}

func TestPull_SelfContainedImage_RelativeOutputDir(t *testing.T) {
	_, _, installRef := installSelfContainedFixture(t)

	cwd := t.TempDir()
	t.Chdir(cwd)
	if _, err := runCLI(t, "pull", "-o", "./out", installRef); err != nil {
		t.Fatalf("pull exported with relative -o: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "out", "pr-review", "SKILL.md")); err != nil {
		t.Errorf("pulled export missing with relative output dir: %v", err)
	}
}

func TestPull_SelfContainedImage_NoCacheFlag(t *testing.T) {
	// --no-cache must populate only the output directory, per the flag help.
	_, _, installRef := installSelfContainedFixture(t)

	outDir := t.TempDir()
	if _, err := runCLI(t, "pull", "--no-cache", "-o", outDir, installRef); err != nil {
		t.Fatalf("pull self-contained with --no-cache: %v", err)
	}
	for _, f := range []string{
		"SKILL.md",
		"deps/skills/generic-review/SKILL.md",
		"deps/prompts/review-policy/PROMPT.md",
	} {
		if _, err := os.Stat(filepath.Join(outDir, "pr-review", filepath.FromSlash(f))); err != nil {
			t.Errorf("pulled file %q missing: %v", f, err)
		}
	}
	if _, err := os.Stat(installer.SelfContainedCacheDir("pr-review", "1.0.0")); !os.IsNotExist(err) {
		t.Errorf("--no-cache wrote the self-contained cache: %v", err)
	}
}

func TestInstall_OrdinaryImageUnaffectedByExportPaths(t *testing.T) {
	// An ordinary (non-export) image still installs through the normal path
	// even when an self-contained cache entry with the same name@version exists.
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	project := t.TempDir()
	t.Chdir(project)
	writeLocalSkill(t, project, "plain-skill", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: plain-skill\n---\nplain\n",
	})
	layout := filepath.Join(project, "build")
	if out, err := runCLI(t, "pack", "-o", layout); err != nil {
		t.Fatalf("pack: %v (out: %s)", err, out)
	}
	// Pre-existing self-contained cache entry with the same name@version.
	exportDir := installer.SelfContainedCacheDir("plain-skill", "1.0.0")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLocalSkill(t, exportDir, "plain-skill", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: plain-skill\n---\nEXPORT POISON\n",
	})

	if _, err := runCLI(t, "install", "--target", "claude", "oci:"+layout+":plain-skill:1.0.0"); err != nil {
		t.Fatalf("install ordinary: %v", err)
	}
	installed, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "plain-skill", "SKILL.md"))
	if strings.Contains(string(installed), "EXPORT POISON") {
		t.Fatalf("self-contained cache substituted for ordinary install: %q", installed)
	}
}
