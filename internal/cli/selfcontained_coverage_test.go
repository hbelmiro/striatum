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

// pack --self-contained must respect -f/--manifest from an unrelated cwd and
// default to <project>/build/ with the name:version tag.
func TestPackSelfContained_ManifestFlagAndDefaultOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	project := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd)

	writeLocalSkill(t, project, "flagged", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: flagged\n---\nx\n",
	})

	if out, err := runCLI(t, "pack", "--self-contained", "-f", project); err != nil {
		t.Fatalf("pack --self-contained -f: %v (out: %s)", err, out)
	}
	layout := filepath.Join(project, "build")
	if _, err := os.Stat(filepath.Join(layout, "index.json")); err != nil {
		t.Fatalf("default layout at %s missing: %v", layout, err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "build")); !os.IsNotExist(err) {
		t.Errorf("layout written to cwd instead of project: %v", err)
	}
	store, titles := openExportLayout(t, layout, "flagged:1.0.0")
	if _, ok := titles["SKILL.md"]; !ok {
		t.Errorf("root SKILL.md missing from layout (have %v)", titles)
	}
	_ = store
}

// Self-contained install enforces version conflicts and honors --force.
func TestInstallSelfContained_VersionConflictAndForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "conflicty", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: conflicty\n---\nV1\n",
	})
	layout1 := filepath.Join(project, "dist1")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layout1); err != nil {
		t.Fatal(err)
	}
	writeLocalSkill(t, project, "conflicty", "2.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: conflicty\n---\nV2\n",
	})
	layout2 := filepath.Join(project, "dist2")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layout2); err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, "install", "--target", "claude", "oci:"+layout1+":conflicty:1.0.0"); err != nil {
		t.Fatalf("first install: %v", err)
	}
	_, err := runCLI(t, "install", "--target", "claude", "oci:"+layout2+":conflicty:2.0.0")
	if err == nil || !strings.Contains(err.Error(), "conflicts with installed") {
		t.Fatalf("err = %v, want version conflict", err)
	}
	if _, err := runCLI(t, "install", "--target", "claude", "--force", "oci:"+layout2+":conflicty:2.0.0"); err != nil {
		t.Fatalf("install with --force: %v", err)
	}
	installed, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "conflicty", "SKILL.md"))
	if !strings.Contains(string(installed), "V2") {
		t.Errorf("after --force, installed content = %q, want V2", installed)
	}
}

// Self-contained install works for non-claude targets and project scope.
func TestInstallSelfContained_CodexTargetAndProjectScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "codexy", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: codexy\n---\nx\n",
	})
	layout := filepath.Join(project, "dist")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, "install", "--target", "codex", "oci:"+layout+":codexy:1.0.0"); err != nil {
		t.Fatalf("install --target codex: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "codexy", "SKILL.md")); err != nil {
		t.Errorf("codex target missing: %v", err)
	}

	appProject := t.TempDir()
	if _, err := runCLI(t, "install", "--target", "claude", "--project", appProject, "oci:"+layout+":codexy:1.0.0"); err != nil {
		t.Fatalf("install --project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appProject, ".claude", "skills", "codexy", "SKILL.md")); err != nil {
		t.Errorf("project-scoped install missing: %v", err)
	}
}

// Reinstall failure paths: an unreachable source flags the DB entry as
// error without half-written cache, and an empty source ref is reported.
func TestReinstall_SelfContainedFailurePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "gone", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: gone\n---\nx\n",
	})
	layout := filepath.Join(project, "dist")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "install", "--target", "claude", "oci:"+layout+":gone:1.0.0"); err != nil {
		t.Fatal(err)
	}

	// Evict the cache and make the source unreachable (layout deleted).
	if err := os.RemoveAll(installer.SelfContainedCacheDir("gone", "1.0.0")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(layout); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "install", "--reinstall-all"); err == nil {
		t.Fatal("reinstall with unreachable source: expected error")
	}
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "gone" && e.Status != "error" {
			t.Errorf("DB entry not flagged error: %+v", e)
		}
	}
	// No half-written cache entry: no artifact.json may exist (an empty
	// directory left behind by the failed pull is harmless and treated as a
	// cache miss by the next attempt).
	if _, err := os.Stat(filepath.Join(installer.SelfContainedCacheDir("gone", "1.0.0"), "artifact.json")); !os.IsNotExist(err) {
		t.Errorf("half-written self-contained cache after failed reinstall: %v", err)
	}

	// Empty source ref: hand-written DB entry cannot be re-pulled.
	noRef := []installer.InstalledEntry{{
		Name: "norefs", Kind: "Skill", Version: "1.0.0", Target: "claude",
		Status: "ok", SelfContained: true,
	}}
	if err := installer.SaveInstalled(noRef); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "install", "--reinstall-all"); err == nil {
		t.Fatal("reinstall with empty source ref: expected error")
	}
}

// inspect warns on stderr (and still reports the marker on stdout) when the
// provenance layer is missing or corrupt.
func TestInspect_SelfContainedProvenanceWarnings(t *testing.T) {
	buildLayout := func(t *testing.T, provenance []byte) string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: provtest\n---\nx\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var files []string
		pi := oci.PreparedImage{
			Config:              mustMarkedConfig(t, "provtest"),
			ArtifactType:        oci.ArtifactTypeForKind("Skill"),
			Dir:                 dir,
			ManifestAnnotations: map[string]string{"dev.striatum.self-contained": "v1"},
		}
		if provenance != nil {
			if err := os.MkdirAll(filepath.Join(dir, ".striatum"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".striatum", "provenance.json"), provenance, 0o644); err != nil {
				t.Fatal(err)
			}
			files = append(files, ".striatum/provenance.json")
		}
		pi.Files = append(files, "SKILL.md")
		layout := t.TempDir()
		if _, err := oci.PackPreparedToLayout(t.Context(), pi, layout, "provtest:1.0.0"); err != nil {
			t.Fatal(err)
		}
		return layout
	}

	t.Run("missing provenance layer", func(t *testing.T) {
		layout := buildLayout(t, nil)
		out, errOut, err := runCLIWithErr(t, "inspect", "oci:"+layout+":provtest:1.0.0")
		if err != nil {
			t.Fatalf("inspect: %v", err)
		}
		if !strings.Contains(out, "Self-contained:") {
			t.Errorf("stdout missing marker line: %q", out)
		}
		if !strings.Contains(errOut, "could not fetch provenance layer") {
			t.Errorf("stderr missing warning: %q", errOut)
		}
	})
	t.Run("corrupt provenance JSON", func(t *testing.T) {
		layout := buildLayout(t, []byte("{corrupt"))
		out, errOut, err := runCLIWithErr(t, "inspect", "oci:"+layout+":provtest:1.0.0")
		if err != nil {
			t.Fatalf("inspect: %v", err)
		}
		if !strings.Contains(out, "Self-contained:") {
			t.Errorf("stdout missing marker line: %q", out)
		}
		if !strings.Contains(errOut, "could not parse provenance document") {
			t.Errorf("stderr missing warning: %q", errOut)
		}
	})
}

func mustMarkedConfig(t *testing.T, name string) []byte {
	t.Helper()
	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: name, Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	base, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(base, &generic); err != nil {
		t.Fatal(err)
	}
	generic["dev.striatum.self-contained"] = json.RawMessage(`"v1"`)
	out, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Installing a marked image whose root is not a Skill is rejected.
func TestInstall_SelfContainedNonSkillRootRejected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PROMPT.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pi := oci.PreparedImage{
		Config:              mustPromptMarkedConfig(t),
		ArtifactType:        oci.ArtifactTypeForKind("Prompt"),
		Dir:                 dir,
		Files:               []string{"PROMPT.md"},
		ManifestAnnotations: map[string]string{"dev.striatum.self-contained": "v1"},
	}
	layout := t.TempDir()
	if _, err := oci.PackPreparedToLayout(t.Context(), pi, layout, "notskill:1.0.0"); err != nil {
		t.Fatal(err)
	}

	_, err := runCLI(t, "install", "--target", "claude", "oci:"+layout+":notskill:1.0.0")
	if err == nil || !strings.Contains(err.Error(), "Skill") {
		t.Fatalf("err = %v, want non-Skill root rejection", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".claude", "skills")); !os.IsNotExist(statErr) {
		t.Errorf("target directory written despite rejection: %v", statErr)
	}
}

func mustPromptMarkedConfig(t *testing.T) []byte {
	t.Helper()
	base := []byte(`{"apiVersion":"striatum.dev/v1alpha2","kind":"Prompt","metadata":{"name":"notskill","version":"1.0.0"},"spec":{"entrypoint":"PROMPT.md","files":["PROMPT.md"]}}`)
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(base, &generic); err != nil {
		t.Fatal(err)
	}
	generic["dev.striatum.self-contained"] = json.RawMessage(`"v1"`)
	out, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Pulling a self-contained image from a missing tag fails cleanly with no
// partial output.
func TestPull_SelfContainedMissingTagFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "missing", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: missing\n---\nx\n",
	})
	layout := filepath.Join(project, "dist")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	if _, err := runCLI(t, "pull", "-o", outDir, "oci:"+layout+":no-such-tag:1.0.0"); err == nil {
		t.Fatal("pull with missing tag: expected error")
	}
	if entries, _ := os.ReadDir(outDir); len(entries) != 0 {
		t.Errorf("partial output written: %v", entries)
	}
}

// pack --self-contained fails cleanly when preparation fails after staging
// exists (a dependency resolves but a declared file is missing): no partial
// layout, no leftover staging directories under the cache root.
func TestPackSelfContained_PrepareFailureFailsCleanly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	// The dependency resolves from cache, but its declared file is absent.
	dep := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "half-there", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md", "missing.md"}},
	}
	depJSON, _ := json.Marshal(dep)
	depCache := installer.CacheDir("Skill", "half-there", "1.0.0")
	if err := os.MkdirAll(depCache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCache, "artifact.json"), depJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCache, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "unreach", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "reg.invalid", Repository: "skills/half-there", Tag: "1.0.0"},
		},
	}
	rootJSON, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: unreach\n---\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(project, "dist")
	if out, err := runCLI(t, "pack", "--self-contained", "-o", layout); err == nil {
		t.Fatal("pack with missing dependency file: expected error")
	} else if !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("err = %v, want error naming missing.md (out: %s)", err, out)
	}
	if _, err := os.Stat(filepath.Join(layout, "index.json")); !os.IsNotExist(err) {
		t.Errorf("partial layout written: %v", err)
	}
	// The staging parent exists (Prepare created it); no staging dirs leak.
	stagingParent := filepath.Join(home, "cache")
	entries, err := os.ReadDir(stagingParent)
	if err != nil {
		t.Fatalf("read staging parent: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".export-staging-") || strings.HasPrefix(e.Name(), ".staging-") {
			t.Errorf("staging directory %q leaked under cache root", e.Name())
		}
	}
}

// Two self-contained installs of the same name@version pinned to different
// digests share one cache directory; reinstall must give each entry the
// image its own immutable reference pins, not whatever the cache last held.
func TestReinstall_SelfContainedPerEntryDigest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "shared-nv", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: shared-nv\n---\nIMAGE A\n",
	})
	layoutA := filepath.Join(project, "distA")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layoutA); err != nil {
		t.Fatal(err)
	}
	writeLocalSkill(t, project, "shared-nv", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: shared-nv\n---\nIMAGE B\n",
	})
	layoutB := filepath.Join(project, "distB")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layoutB); err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, "install", "--target", "claude", "oci:"+layoutA+":shared-nv:1.0.0"); err != nil {
		t.Fatalf("install A: %v", err)
	}
	if _, err := runCLI(t, "install", "--target", "cursor", "oci:"+layoutB+":shared-nv:1.0.0"); err != nil {
		t.Fatalf("install B: %v", err)
	}
	// Remove the installed dirs so reinstall must re-copy from cache.
	if err := os.RemoveAll(filepath.Join(home, ".claude", "skills", "shared-nv")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(home, ".cursor", "skills", "shared-nv")); err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, "install", "--reinstall-all"); err != nil {
		t.Fatalf("reinstall-all: %v", err)
	}
	claudeContent, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "shared-nv", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	cursorContent, err := os.ReadFile(filepath.Join(home, ".cursor", "skills", "shared-nv", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claudeContent), "IMAGE A") {
		t.Errorf("claude target reinstalled %q, want IMAGE A (its own pinned digest)", claudeContent)
	}
	if !strings.Contains(string(cursorContent), "IMAGE B") {
		t.Errorf("cursor target reinstalled %q, want IMAGE B (its own pinned digest)", cursorContent)
	}
}

// Reinstall of a self-contained entry must succeed from the warm cache even
// when the original source (a local layout) has been deleted, provided the
// cached digest matches the entry's pinned reference.
func TestReinstall_SelfContainedWarmCacheWithDeletedLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "warmcache", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: warmcache\n---\nx\n",
	})
	layout := filepath.Join(project, "dist")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "install", "--target", "claude", "oci:"+layout+":warmcache:1.0.0"); err != nil {
		t.Fatal(err)
	}

	// Remove the installed dir and delete the layout; the cache still holds
	// the pinned payload.
	if err := os.RemoveAll(filepath.Join(home, ".claude", "skills", "warmcache")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(layout); err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, "install", "--reinstall-all"); err != nil {
		t.Fatalf("reinstall from warm cache with deleted layout: %v", err)
	}
	restored, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "warmcache", "SKILL.md"))
	if err != nil {
		t.Fatalf("reinstall did not restore from warm cache: %v", err)
	}
	if !strings.Contains(string(restored), "name: warmcache") {
		t.Fatalf("restored content = %q", restored)
	}
}

// A non-Skill ordinary cache entry (e.g. a Prompt) must not shadow a
// self-contained Skill image published under the same name@version: the
// marker check runs regardless of the cached entry's kind.
func TestInstall_SelfContainedNotShadowedByPromptCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "shadowed-prompt", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: shadowed-prompt\n---\nSELF-CONTAINED PAYLOAD\n",
	})
	layout := filepath.Join(project, "dist")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatal(err)
	}

	// Poison the ordinary cache with a PROMPT entry of the same name@version.
	poison := installer.CacheDir("Prompt", "shadowed-prompt", "1.0.0")
	if err := os.MkdirAll(poison, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLocalArtifact(t, poison, "Prompt", "shadowed-prompt", "1.0.0", "PROMPT.md", map[string]string{
		"PROMPT.md": "poisoned prompt",
	})

	if _, err := runCLI(t, "install", "--target", "claude", "oci:"+layout+":shadowed-prompt:1.0.0"); err != nil {
		t.Fatalf("install shadowed by Prompt cache: %v", err)
	}
	installed, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "shadowed-prompt", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installed), "SELF-CONTAINED PAYLOAD") {
		t.Fatalf("installed content = %q, want the self-contained payload", installed)
	}
}

// Promoting a previously-installed dependency to an explicit root install
// severs the InstalledWith relationship, so uninstalling the old parent no
// longer treats it as an orphaned dependency.
func TestInstall_SelfContainedPromotionClearsInstalledWith(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "promoted", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: promoted\n---\nx\n",
	})
	layout := filepath.Join(project, "dist")
	if _, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatal(err)
	}

	// The artifact was previously installed as a dependency of "rootx".
	seed := []installer.InstalledEntry{{
		Name: "promoted", Kind: "Skill", Version: "0.9.0", Target: "claude",
		InstalledWith: "rootx", Status: "ok", UpdatedAt: "2026-01-01T00:00:00Z",
	}}
	if err := installer.SaveInstalled(seed); err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, "install", "--target", "claude", "--force", "oci:"+layout+":promoted:1.0.0"); err != nil {
		t.Fatalf("install promoted: %v", err)
	}
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "promoted" {
			if e.InstalledWith != "" {
				t.Errorf("InstalledWith = %q after promotion to root, want empty", e.InstalledWith)
			}
			if !e.SelfContained {
				t.Errorf("entry not marked self-contained: %+v", e)
			}
		}
	}
}

// A pinned OCI dependency at an unreachable registry with a warm cache must
// still prepare: the pin-aware fetcher falls back to the cache-first
// fetcher, and the materializer verifies the cached payload against the pin.
func TestPackSelfContained_OfflinePinnedDepWithWarmCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	pin := "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	depManifest := `{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": "offline-dep", "version": "1.0.0"},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]}
}`
	depCache := installer.CacheDir("Skill", "offline-dep", "1.0.0")
	if err := os.MkdirAll(depCache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCache, "artifact.json"), []byte(depManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depCache, "SKILL.md"), []byte("cached content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installer.WriteDigest(depCache, pin); err != nil {
		t.Fatal(err)
	}

	root := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "offline-root", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: "reg.invalid", Repository: "skills/offline-dep", Tag: "1.0.0", Digest: pin},
		},
	}
	rootJSON, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: offline-root\n---\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(project, "dist")
	if out, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatalf("offline pack with warm pinned cache: %v (out: %s)", err, out)
	}
	store, titles := openExportLayout(t, layout, "offline-root:1.0.0")
	depContent := fetchLayerByTitle(t, store, titles, "deps/skills/offline-dep/SKILL.md")
	if !strings.Contains(depContent, "cached content") {
		t.Fatalf("dep content = %q, want cached content", depContent)
	}
}
