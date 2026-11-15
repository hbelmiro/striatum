//go:build integration

package cli

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
)

// startIntegrationRegistry starts a disposable registry:2 container on a
// random port and returns the "localhost:<port>" host prefix.
func startIntegrationRegistry(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not in PATH, skipping integration test")
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker not available: %v (%s)", err, out)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	portStr := strconv.Itoa(port)
	name := "striatum-export-test-registry-" + portStr
	if out, err := exec.Command("docker", "run", "-d", "--rm", "--name", name,
		"-p", "127.0.0.1:"+portStr+":5000", "registry:2").CombinedOutput(); err != nil {
		t.Skipf("start registry: %v (%s)", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })

	baseURL := "http://127.0.0.1:" + portStr
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/v2/")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				return "localhost:" + portStr
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("registry did not become ready within 30s")
	return ""
}

// TestIntegration_ExportPushPrintDigestInstallByDigest covers the full
// publish-and-consume loop: push --self-contained --print-digest emits exactly one
// canonical immutable reference; that reference installs through striatum
// install with dependency sources unavailable; and it keeps selecting the
// same image after the mutable tag moves.
func TestIntegration_ExportPushPrintDigestInstallByDigest(t *testing.T) {
	host := startIntegrationRegistry(t)
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	// Seed the dependency closure in the cache, then export pr-review.
	seedExportDep(t, home, "Prompt", "review-policy", "1.0.0", map[string]string{
		"PROMPT.md": "policy body",
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

	tagRef := host + "/skills/pr-review:1.0.0"
	out, err := runCLI(t, "push", "--self-contained", "--print-digest", tagRef)
	if err != nil {
		t.Fatalf("push --self-contained --print-digest: %v (out: %s)", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("stdout = %q, want exactly one line", out)
	}
	digestRef := lines[0]
	wantPrefix := host + "/skills/pr-review@sha256:"
	if !strings.HasPrefix(digestRef, wantPrefix) || len(digestRef) != len(wantPrefix)+64 {
		t.Fatalf("stdout line %q is not a canonical registry/repository@sha256:... reference", digestRef)
	}

	// Dependency sources become unavailable.
	for _, kind := range []string{"Skill", "Prompt"} {
		if err := os.RemoveAll(filepath.Join(home, "cache", kind)); err != nil {
			t.Fatal(err)
		}
	}

	// Install by digest-only reference.
	if _, err := runCLI(t, "install", "--target", "claude", digestRef); err != nil {
		t.Fatalf("install by digest: %v", err)
	}
	targetDir := filepath.Join(home, ".claude", "skills", "pr-review")
	installed, err := os.ReadFile(filepath.Join(targetDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
	if !strings.Contains(string(installed), "](deps/skills/generic-review/SKILL.md)") {
		t.Errorf("installed content is not the self-contained payload: %q", installed)
	}
	for _, f := range []string{
		"deps/skills/generic-review/SKILL.md",
		"deps/prompts/review-policy/PROMPT.md",
		".striatum/provenance.json",
	} {
		if _, err := os.Stat(filepath.Join(targetDir, filepath.FromSlash(f))); err != nil {
			t.Errorf("installed file %q missing: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "cache", "Skill", "pr-review@1.0.0")); !os.IsNotExist(err) {
		t.Errorf("ordinary cache entry created for export install: %v", err)
	}

	// The stored DB ref is the immutable digest reference.
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	var dbRef string
	for _, e := range entries {
		if e.Name == "pr-review" {
			dbRef = e.Registry
		}
	}
	if dbRef != digestRef {
		t.Errorf("DB registry ref = %q, want %q", dbRef, digestRef)
	}

	// The tag moves (different content pushed under the same tag); pulling by
	// the stored digest must still select the original export.
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: pr-review\n---\nMOVED tag content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	movedRoot := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "pr-review", Version: "2.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	movedJSON, _ := json.Marshal(movedRoot)
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), movedJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	movedOut, err := runCLI(t, "push", "--print-digest", tagRef)
	if err != nil {
		t.Fatalf("push moved tag: %v", err)
	}
	movedDigest := strings.TrimSpace(movedOut)
	if movedDigest == digestRef {
		t.Fatalf("tag did not move: push returned the same reference %q", movedDigest)
	}

	// Evict the self-contained cache and reinstall: the digest ref must win over the
	// moved tag.
	if err := os.RemoveAll(installer.SelfContainedCacheDir("pr-review", "1.0.0")); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "install", "--reinstall-all"); err != nil {
		t.Fatalf("reinstall-all: %v", err)
	}
	restored, err := os.ReadFile(filepath.Join(targetDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("reinstall did not restore export: %v", err)
	}
	if strings.Contains(string(restored), "MOVED tag content") {
		t.Errorf("reinstall picked up the moved tag instead of the digest: %q", restored)
	}
	if !strings.Contains(string(restored), "](deps/skills/generic-review/SKILL.md)") {
		t.Errorf("reinstalled content is not the self-contained payload: %q", restored)
	}

	// pull and inspect accept the digest-only reference too.
	pullDir := t.TempDir()
	if _, err := runCLI(t, "pull", "-o", pullDir, digestRef); err != nil {
		t.Fatalf("pull by digest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pullDir, "pr-review", "deps", "skills", "generic-review", "SKILL.md")); err != nil {
		t.Errorf("pulled self-contained payload missing deps: %v", err)
	}
	inspected, err := runCLI(t, "inspect", digestRef)
	if err != nil {
		t.Fatalf("inspect by digest: %v", err)
	}
	if !strings.Contains(inspected, "self-contained") {
		t.Errorf("inspect output missing export section:\n%s", inspected)
	}

	// Nested repository reference also works end-to-end.
	nestedRef := host + "/nested/deeper/other:1.0.0"
	nestedOut, err := runCLI(t, "push", "--print-digest", nestedRef)
	if err != nil {
		t.Fatalf("push --print-digest nested repo: %v", err)
	}
	nestedDigest := strings.TrimSpace(nestedOut)
	if !strings.HasPrefix(nestedDigest, host+"/nested/deeper/other@sha256:") {
		t.Errorf("nested stdout = %q, want canonical nested reference", nestedOut)
	}
}

// TestIntegration_ExportHonorsPinnedDependencyDigest verifies that a pinned
// OCI dependency is fetched by its digest: when the tag moves after the pin,
// the self-contained image still bundles the pinned content.
func TestIntegration_ExportHonorsPinnedDependencyDigest(t *testing.T) {
	host := startIntegrationRegistry(t)
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	// Publish dependency content A and capture its digest.
	depDir := t.TempDir()
	writeLocalSkill(t, depDir, "pinned-dep", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: pinned-dep\n---\nCONTENT A\n",
	})
	t.Chdir(depDir)
	depOut, err := runCLI(t, "push", "--print-digest", host+"/skills/pinned-dep:1.0.0")
	if err != nil {
		t.Fatalf("push dep A: %v", err)
	}
	pinnedRef := strings.TrimSpace(depOut)
	pinnedDigest := strings.TrimPrefix(pinnedRef, host+"/skills/pinned-dep@")

	// The tag moves to different content after the pin.
	if err := os.WriteFile(filepath.Join(depDir, "SKILL.md"), []byte("---\nname: pinned-dep\n---\nCONTENT B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "push", host+"/skills/pinned-dep:1.0.0"); err != nil {
		t.Fatalf("push dep B: %v", err)
	}

	// Root skill pins the dependency by digest; the dependency is not in any
	// cache, so the self-contained image must pull it fresh.
	t.Chdir(project)
	root := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "pin-root", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: host, Repository: "skills/pinned-dep", Tag: "1.0.0", Digest: pinnedDigest},
		},
	}
	rootJSON, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: pin-root\n---\nroot\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(project, "dist")
	if out, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatalf("pack --self-contained with pinned dep: %v (out: %s)", err, out)
	}
	store, titles := openExportLayout(t, layout, "pin-root:1.0.0")
	depContent := fetchLayerByTitle(t, store, titles, "deps/skills/pinned-dep/SKILL.md")
	if !strings.Contains(depContent, "CONTENT A") {
		t.Fatalf("pinned dep content = %q, want the pinned CONTENT A (tag moved to CONTENT B)", depContent)
	}
}

// TestIntegration_SelfContainedWarmCacheHonorsPin verifies that a pinned
// dependency is re-pulled by digest when a warm cache entry disagrees with
// the declared pin (e.g. the tag was pulled before it moved).
func TestIntegration_SelfContainedWarmCacheHonorsPin(t *testing.T) {
	host := startIntegrationRegistry(t)
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	// Publish content A and capture its digest.
	depDir := t.TempDir()
	writeLocalSkill(t, depDir, "warm-pin-dep", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: warm-pin-dep\n---\nCONTENT A\n",
	})
	t.Chdir(depDir)
	depOut, err := runCLI(t, "push", "--print-digest", host+"/skills/warm-pin-dep:1.0.0")
	if err != nil {
		t.Fatalf("push dep A: %v", err)
	}
	pinnedDigest := strings.TrimPrefix(strings.TrimSpace(depOut), host+"/skills/warm-pin-dep@")

	// The tag moves to content B, and the user pulls it: the warm cache now
	// holds content B. The root then pins content A's digest.
	if err := os.WriteFile(filepath.Join(depDir, "SKILL.md"), []byte("---\nname: warm-pin-dep\n---\nCONTENT B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(depDir)
	if _, err := runCLI(t, "push", host+"/skills/warm-pin-dep:1.0.0"); err != nil {
		t.Fatalf("push dep B: %v", err)
	}
	t.Chdir(home)
	if _, err := runCLI(t, "pull", host+"/skills/warm-pin-dep:1.0.0"); err != nil {
		t.Fatalf("pull dep to warm cache: %v", err)
	}

	t.Chdir(project)
	root := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "warm-pin-root", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{
			&artifact.OCIDependency{RegistryHost: host, Repository: "skills/warm-pin-dep", Tag: "1.0.0", Digest: pinnedDigest},
		},
	}
	rootJSON, _ := json.Marshal(root)
	if err := os.WriteFile(filepath.Join(project, "artifact.json"), rootJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: warm-pin-root\n---\nroot\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := filepath.Join(project, "dist")
	if out, err := runCLI(t, "pack", "--self-contained", "-o", layout); err != nil {
		t.Fatalf("pack --self-contained with warm cache: %v (out: %s)", err, out)
	}
	store, titles := openExportLayout(t, layout, "warm-pin-root:1.0.0")
	depContent := fetchLayerByTitle(t, store, titles, "deps/skills/warm-pin-dep/SKILL.md")
	if !strings.Contains(depContent, "CONTENT A") {
		t.Fatalf("warm-cache pinned dep content = %q, want the pinned CONTENT A (cache held pre-pin content)", depContent)
	}
}

// push --self-contained must respect -f/--manifest from an unrelated cwd:
// the pushed image contains the manifest project's files, not the cwd's.
func TestIntegration_PushSelfContainedWithManifestFlag(t *testing.T) {
	host := startIntegrationRegistry(t)
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd)

	writeLocalSkill(t, project, "flagged-push", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: flagged-push\n---\nFROM PROJECT\n",
	})
	// A decoy artifact in the cwd that must not be packed.
	writeLocalSkill(t, cwd, "flagged-push", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: flagged-push\n---\nFROM CWD\n",
	})

	digestOut, err := runCLI(t, "push", "--self-contained", "--print-digest", "-f", project, host+"/skills/flagged-push:1.0.0")
	if err != nil {
		t.Fatalf("push --self-contained -f: %v (out: %s)", err, digestOut)
	}
	digestRef := strings.TrimSpace(digestOut)

	// The pushed payload is the project's, not the cwd's.
	outDir := t.TempDir()
	if _, err := runCLI(t, "pull", "-o", outDir, digestRef); err != nil {
		t.Fatalf("pull pushed image: %v", err)
	}
	pulled, err := os.ReadFile(filepath.Join(outDir, "flagged-push", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pulled), "FROM PROJECT") {
		t.Fatalf("pushed payload is not the -f project's content: %q", pulled)
	}
}

// Installing a self-contained image by its registry TAG must not be shadowed
// by an ordinary cache entry for the same name@version.
func TestIntegration_InstallSelfContainedByRegistryTagNotShadowed(t *testing.T) {
	host := startIntegrationRegistry(t)
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "shadowed", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: shadowed\n---\nSELF-CONTAINED PAYLOAD\n",
	})
	tagRef := host + "/skills/shadowed:1.0.0"
	if out, err := runCLI(t, "push", "--self-contained", "--print-digest", tagRef); err != nil {
		t.Fatalf("push self-contained: %v (out: %s)", err, out)
	}

	// Poison the ordinary cache for the same name@version.
	poison := installer.CacheDir("Skill", "shadowed", "1.0.0")
	if err := os.MkdirAll(poison, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLocalSkill(t, poison, "shadowed", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: shadowed\n---\nPOISONED ordinary cache\n",
	})

	if _, err := runCLI(t, "install", "--target", "claude", tagRef); err != nil {
		t.Fatalf("install by registry tag: %v", err)
	}
	installed, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "shadowed", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(installed), "POISONED") {
		t.Fatalf("ordinary cache shadowed the self-contained image: %q", installed)
	}
	if !strings.Contains(string(installed), "SELF-CONTAINED PAYLOAD") {
		t.Fatalf("installed content is not the self-contained payload: %q", installed)
	}
	if _, err := os.Stat(installer.SelfContainedCacheDir("shadowed", "1.0.0")); err != nil {
		t.Errorf("self-contained cache not used: %v", err)
	}
	// The DB stores the immutable digest reference, not the mutable tag.
	entries, err := installer.LoadInstalled()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "shadowed" {
			wantPrefix := host + "/skills/shadowed@sha256:"
			if !strings.HasPrefix(e.Registry, wantPrefix) || len(e.Registry) != len(wantPrefix)+64 {
				t.Errorf("DB registry ref = %q, want %sskills/shadowed@sha256:<64hex>", e.Registry, host+"/")
			}
			if !e.SelfContained {
				t.Errorf("DB entry not marked self-contained: %+v", e)
			}
		}
	}
}

// Pulling a self-contained image by a registry TAG that has moved delivers
// the content the tag pointed at when the digest was resolved.
func TestIntegration_PullSelfContainedByMovedTag(t *testing.T) {
	host := startIntegrationRegistry(t)
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	project := t.TempDir()
	t.Chdir(project)

	writeLocalSkill(t, project, "movedtag", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: movedtag\n---\nCONTENT A\n",
	})
	tagRef := host + "/skills/movedtag:1.0.0"
	aOut, err := runCLI(t, "push", "--self-contained", "--print-digest", tagRef)
	if err != nil {
		t.Fatalf("push A: %v (out: %s)", err, aOut)
	}
	digestA := strings.TrimPrefix(strings.TrimSpace(aOut), host+"/skills/movedtag@")

	// The tag moves to different content.
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("---\nname: movedtag\n---\nCONTENT B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bOut, err := runCLI(t, "push", "--self-contained", "--print-digest", tagRef)
	if err != nil {
		t.Fatalf("push B: %v (out: %s)", err, bOut)
	}
	digestB := strings.TrimPrefix(strings.TrimSpace(bOut), host+"/skills/movedtag@")
	if digestA == digestB {
		t.Fatal("tag did not move: both pushes returned the same digest")
	}

	// Pulling by the tag resolves the digest at pull time (the tag points at
	// B) and pulls by that digest. The pulled content and the cache's stored
	// digest must both correspond to B: this pins that the pull resolves and
	// pulls the same immutable image instead of racing the tag mid-flight.
	outDir := t.TempDir()
	if _, err := runCLI(t, "pull", "-o", outDir, tagRef); err != nil {
		t.Fatalf("pull by tag: %v", err)
	}
	pulled, err := os.ReadFile(filepath.Join(outDir, "movedtag", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pulled), "CONTENT B") {
		t.Fatalf("pulled content = %q, want the tag's current content B", pulled)
	}
	storedDigest, err := installer.ReadDigest(installer.SelfContainedCacheDir("movedtag", "1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if storedDigest != digestB {
		t.Errorf("cache stored digest = %s, want the resolved digest %s (content and digest must match)", storedDigest, digestB)
	}
}
