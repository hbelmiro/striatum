//go:build integration

// Package examples_test contains end-to-end integration tests for the worked
// examples under examples/. Each test copies one example out of the repository
// (so pack never writes build/ into the repo), points any declared OCI
// dependencies at a disposable registry, and drives the compiled striatum
// binary through the full lifecycle: validate, pack, push, pull, inspect,
// install, update, and uninstall.
//
// The suite needs an OCI registry. Set STRIATUM_EXAMPLES_REGISTRY to a
// repository prefix on an already-running registry (e.g.
// localhost:5000/examples) to reuse it; otherwise a disposable registry:2
// Docker container is started on a random loopback port. When neither is
// possible (no Docker), the suite fails: a silent skip would hide a broken
// environment. Each test pushes into its own namespace under the registry,
// so tests never see each other's or earlier runs' state.
//
// Set STRIATUM_BIN to test a prebuilt binary; otherwise TestMain builds
// ./cmd/striatum once for the whole run.
package examples_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// striatumBin is the binary under test, built once in TestMain unless
// STRIATUM_BIN points at an existing build.
var striatumBin string

// Registry state, lazily initialized exactly once and shared by all tests.
var (
	registryOnce    sync.Once
	registryPrefix  string // e.g. "localhost:5050/examples"
	registryFailMsg string // non-empty when the suite must fail (no registry)
	registryCleanup func()
)

func TestMain(m *testing.M) {
	bin := os.Getenv("STRIATUM_BIN")
	if bin == "" {
		tmp, err := os.MkdirTemp("", "striatum-examples-bin-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "create temp dir for binary: %v\n", err)
			os.Exit(1)
		}
		bin = filepath.Join(tmp, "striatum")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/striatum")
		cmd.Dir = repoRoot()
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build striatum: %v\n%s", err, out)
			os.Exit(1)
		}
	}
	striatumBin = bin

	code := m.Run()

	// Bring down the disposable registry container, if one was started.
	if registryCleanup != nil {
		registryCleanup()
	}
	os.Exit(code)
}

// repoRoot returns the repository root, derived from this test file's
// location (the test runs with examples/ as its package directory).
func repoRoot() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed; cannot locate repo root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}

// examplesRegistry returns the per-test repository prefix every test pushes
// to (for example "localhost:5050/examples/t01skillbasic-9f2c1a") and fails
// the calling test when no registry can be provided: a silent skip would
// hide a broken environment. Each test gets its own namespace under the
// shared registry, so tests are independent of each other and of leftover
// state from earlier runs against the same registry.
func examplesRegistry(t *testing.T) string {
	t.Helper()
	registryOnce.Do(initRegistry)
	if registryFailMsg != "" {
		t.Fatalf("%s", registryFailMsg)
	}
	return registryPrefix + "/" + testNamespace(t)
}

// testNamespace derives a unique repository namespace from the test name:
// letters and digits are kept, everything else becomes "-", and a random
// suffix guards against re-runs within the same registry.
func testNamespace(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, r := range strings.ToLower(t.Name()) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("generate namespace suffix: %v", err)
	}
	return b.String() + "-" + hex.EncodeToString(suffix[:])
}

// registryHost returns the host[:port] part of a repository prefix.
func registryHost(prefix string) string {
	i := strings.Index(prefix, "/")
	if i < 0 {
		return prefix
	}
	return prefix[:i]
}

// initRegistry either adopts STRIATUM_EXAMPLES_REGISTRY or starts a
// disposable registry:2 container on a random loopback port. Results are
// stored in the package-level registry vars.
func initRegistry() {
	if prefix := os.Getenv("STRIATUM_EXAMPLES_REGISTRY"); prefix != "" {
		registryPrefix = prefix
		return
	}

	if !dockerAvailable() {
		registryFailMsg = "docker not available and STRIATUM_EXAMPLES_REGISTRY not set; the examples suite requires a registry (start Docker or point STRIATUM_EXAMPLES_REGISTRY at one)"
		fmt.Fprintln(os.Stderr, registryFailMsg)
		return
	}

	port, cleanup, err := startTestRegistry()
	if err != nil {
		registryFailMsg = fmt.Sprintf("could not start disposable registry: %v", err)
		fmt.Fprintln(os.Stderr, registryFailMsg)
		return
	}
	registryPrefix = fmt.Sprintf("127.0.0.1:%d/examples", port)
	registryCleanup = cleanup
}

// dockerAvailable reports whether the docker CLI is present and the daemon
// answers `docker info`.
func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	info := exec.Command("docker", "info", "--format", "{{.ServerVersion}}")
	return info.Run() == nil
}

// startTestRegistry runs a registry:2 container bound to a random loopback
// port and waits until its /v2/ endpoint answers. The port is reserved via a
// probe listener before `docker run` (and the run is retried once) so another
// process grabbing the port between the two is not fatal. The returned
// cleanup function removes the container.
func startTestRegistry() (port int, cleanup func(), err error) {
	noop := func() {}
	// Two attempts: if docker run loses the reserved port, retry with a new one.
	for attempt := 0; attempt < 2; attempt++ {
		// Reserve an ephemeral port, then release it for docker to bind.
		l, listenErr := net.Listen("tcp", "127.0.0.1:0")
		if listenErr != nil {
			return 0, noop, fmt.Errorf("reserve port: %w", listenErr)
		}
		port = l.Addr().(*net.TCPAddr).Port
		_ = l.Close()

		name := fmt.Sprintf("striatum-examples-registry-%d", port)
		_ = exec.Command("docker", "rm", "-f", name).Run()

		bind := fmt.Sprintf("127.0.0.1:%d:5000", port)
		out, runErr := exec.Command("docker", "run", "-d", "-p", bind, "--name", name, "registry:2").CombinedOutput()
		if runErr != nil {
			if attempt == 0 {
				continue
			}
			return 0, noop, fmt.Errorf("docker run registry:2: %v (%s)", runErr, strings.TrimSpace(string(out)))
		}

		cleanup = func() { _ = exec.Command("docker", "rm", "-f", name).Run() }

		if waitErr := waitForRegistry(fmt.Sprintf("http://127.0.0.1:%d", port)); waitErr != nil {
			cleanup()
			return 0, noop, waitErr
		}
		return port, cleanup, nil
	}
	return 0, noop, fmt.Errorf("could not start registry after retry")
}

// waitForRegistry polls GET <baseURL>/v2/ until the registry answers with
// 200 or 401, or times out (~15s).
func waitForRegistry(baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error
	for i := 0; i < 75; i++ {
		resp, err := client.Get(baseURL + "/v2/")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				return nil
			}
			lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("registry did not become reachable at %s: %w", baseURL, lastErr)
}

// setupTestEnv isolates HOME and STRIATUM_HOME in a temp dir so installs land
// in a scratch ~/.claude and the cache in a scratch ~/.striatum.
func setupTestEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("STRIATUM_HOME", filepath.Join(home, ".striatum"))
	return home
}

// copyExample copies examples/<rel> into a temp dir and returns the copy.
// Tests always work on the copy so commands like pack (which writes build/
// next to the manifest) never touch the repository.
func copyExample(t *testing.T, rel string) string {
	t.Helper()
	src := filepath.Join(repoRoot(), "examples", filepath.FromSlash(rel))
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatalf("copy example %s: %v", rel, err)
	}
	return dst
}

// rewriteManifestRefs points every artifact.json under dir at the test's
// registry namespace: the placeholder "localhost:5000" registry host becomes
// the test registry host, and repository paths under the committed
// "examples/" prefix move into the per-test namespace, so each test works
// only with state it created and leftover registry content from other runs
// cannot interfere.
func rewriteManifestRefs(t *testing.T, dir, prefix string) {
	t.Helper()
	host := registryHost(prefix)
	ns := strings.TrimPrefix(strings.TrimPrefix(prefix, host), "/") // e.g. "examples/t05ocideps-9f2c1a"
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "artifact.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rewritten := strings.ReplaceAll(string(data), "localhost:5000", host)
		rewritten = strings.ReplaceAll(rewritten, `"repository": "examples/`, `"repository": "`+ns+`/`)
		if rewritten == string(data) {
			return nil
		}
		return os.WriteFile(path, []byte(rewritten), 0o644)
	})
	if err != nil {
		t.Fatalf("rewrite manifest refs in %s: %v", dir, err)
	}
}

// runStriatum executes the striatum binary and fails the test on a non-zero
// exit, returning the combined stdout/stderr output.
func runStriatum(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runStriatumErr(args...)
	if err != nil {
		t.Fatalf("striatum %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// runStriatumErr executes the striatum binary and reports whether it failed,
// for tests that assert on errors.
func runStriatumErr(args ...string) (string, error) {
	cmd := exec.Command(striatumBin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// digestFromPushOutput extracts the OCI manifest digest from push output
// (a line of the form "Digest: sha256:<64 hex chars>").
func digestFromPushOutput(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if d, ok := strings.CutPrefix(strings.TrimSpace(line), "Digest: "); ok {
			if len(d) == len("sha256:")+64 && strings.HasPrefix(d, "sha256:") {
				return d
			}
		}
	}
	t.Fatalf("no digest line in push output:\n%s", out)
	return ""
}

// mustExist fails the test when path does not exist.
func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to exist: %v", path, err)
	}
}

// mustNotExist fails the test when path exists.
func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		t.Errorf("expected %s to be gone, but it exists", path)
	}
}

// editManifest loads an artifact.json into a generic map, applies fn, and
// writes it back. Used for surgical edits (versions, digests) without
// reshaping unrelated fields.
func editManifest(t *testing.T, manifestPath string, fn func(m map[string]any)) {
	t.Helper()
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	fn(m)
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setManifestVersion rewrites metadata.version in an artifact.json.
func setManifestVersion(t *testing.T, manifestPath, version string) {
	t.Helper()
	editManifest(t, manifestPath, func(m map[string]any) {
		meta, _ := m["metadata"].(map[string]any)
		if meta == nil {
			t.Fatalf("%s has no metadata object", manifestPath)
		}
		meta["version"] = version
	})
}

// setupGitFixture creates a bare git repository containing a Skill artifact
// (name, version) at its root on branch main, and returns the file:// URL of
// the bare repo. Modeled on setupLocalGitRepo in internal/cli.
func setupGitFixture(t *testing.T, name, version string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git not in PATH: %v", err)
	}

	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "bare.git")

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init", "--bare", "-b", "main", bareDir)
	run("git", "clone", bareDir, workDir)

	manifest := fmt.Sprintf(`{
  "apiVersion": "striatum.dev/v1alpha2",
  "kind": "Skill",
  "metadata": {"name": %q, "version": %q},
  "spec": {"entrypoint": "SKILL.md", "files": ["SKILL.md"]}
}`, name, version)
	if err := os.WriteFile(filepath.Join(workDir, "artifact.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("# "+name), 0o644); err != nil {
		t.Fatal(err)
	}

	run("git", "-C", workDir, "add", "-A")
	run("git", "-C", workDir, "commit", "-m", "init")
	run("git", "-C", workDir, "push", "origin", "HEAD:main")

	p := filepath.ToSlash(bareDir)
	if len(p) > 0 && p[0] != '/' {
		p = "/" + p
	}
	return "file://" + p
}

// memorySlug mirrors installer.ProjectPathToSlug: every non-alphanumeric
// character becomes "-", with truncation plus a short hash past 128 chars.
// Mirrored here (instead of imported) to keep the test a black-box test of
// the compiled binary.
func memorySlug(absPath string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9]`)
	slug := re.ReplaceAllString(absPath, "-")
	if len(slug) > 128 {
		h := sha256.Sum256([]byte(absPath))
		slug = slug[:111] + "-" + fmt.Sprintf("%x", h[:8])
	}
	return slug
}

// pushExample pushes examples/<rel> (already copied to dir) as the given ref
// and returns the push output.
func pushExample(t *testing.T, dir, ref string) string {
	t.Helper()
	run := exec.Command(striatumBin, "push", ref)
	run.Dir = dir
	var buf bytes.Buffer
	run.Stdout = &buf
	run.Stderr = &buf
	if err := run.Run(); err != nil {
		t.Fatalf("striatum push %s (in %s): %v\n%s", ref, dir, err, buf.String())
	}
	return buf.String()
}

// pushSelfContained pushes the project in dir as one self-contained image and
// returns the single canonical <repo>@sha256:... reference printed to stdout
// by --print-digest (diagnostics go to stderr, so stdout holds exactly the
// reference).
func pushSelfContained(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command(striatumBin, "push", "--self-contained", "--print-digest", ref)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("striatum push --self-contained --print-digest %s (in %s): %v\n%s%s", ref, dir, err, stdout.String(), stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if !regexp.MustCompile(`^\S+@sha256:[0-9a-f]{64}$`).MatchString(out) {
		t.Fatalf("--print-digest stdout is not a single canonical reference line:\n%q", out)
	}
	return out
}

func TestExample_SkillBasic(t *testing.T) {
	prefix := examplesRegistry(t)
	home := setupTestEnv(t)

	dir := copyExample(t, "01-skill-basic/my-skill")
	rewriteManifestRefs(t, dir, prefix)
	t.Chdir(dir)

	runStriatum(t, "validate")
	runStriatum(t, "pack")
	mustExist(t, filepath.Join(dir, "build", "index.json"))

	ref := prefix + "/my-skill:1.0.0"
	runStriatum(t, "push", ref)

	// inspect shows the remote artifact's metadata.
	inspectOut := runStriatum(t, "inspect", ref)
	if !strings.Contains(inspectOut, "my-skill") || !strings.Contains(inspectOut, "Skill") {
		t.Errorf("inspect output does not describe the pushed Skill:\n%s", inspectOut)
	}

	outDir := t.TempDir()
	runStriatum(t, "pull", ref, "-o", outDir)
	mustExist(t, filepath.Join(outDir, "my-skill", "artifact.json"))
	mustExist(t, filepath.Join(outDir, "my-skill", "SKILL.md"))

	runStriatum(t, "install", "--target", "claude", ref)
	skillFile := filepath.Join(home, ".claude", "skills", "my-skill", "SKILL.md")
	mustExist(t, skillFile)

	// The same artifact installs into other supported targets (codex).
	runStriatum(t, "install", "--target", "codex", ref)
	codexFile := filepath.Join(home, ".codex", "skills", "my-skill", "SKILL.md")
	mustExist(t, codexFile)
	runStriatum(t, "uninstall", "--target", "codex", "my-skill")
	mustNotExist(t, codexFile)

	runStriatum(t, "uninstall", "--target", "claude", "my-skill")
	mustNotExist(t, skillFile)
}

func TestExample_PromptBasic(t *testing.T) {
	prefix := examplesRegistry(t)
	home := setupTestEnv(t)

	dir := copyExample(t, "02-prompt-basic/severity-rubric")
	rewriteManifestRefs(t, dir, prefix)
	t.Chdir(dir)

	runStriatum(t, "validate")
	runStriatum(t, "pack")

	ref := prefix + "/severity-rubric:1.0.0"
	runStriatum(t, "push", ref)

	inspectOut := runStriatum(t, "inspect", ref)
	if !strings.Contains(inspectOut, "severity-rubric") || !strings.Contains(inspectOut, "Prompt") {
		t.Errorf("inspect output does not describe the pushed Prompt:\n%s", inspectOut)
	}

	outDir := t.TempDir()
	runStriatum(t, "pull", ref, "-o", outDir)
	mustExist(t, filepath.Join(outDir, "severity-rubric", "severity-rubric.md"))

	runStriatum(t, "install", "--target", "claude", ref)
	promptFile := filepath.Join(home, ".claude", "prompts", "severity-rubric", "severity-rubric.md")
	mustExist(t, promptFile)

	runStriatum(t, "uninstall", "--target", "claude", "severity-rubric")
	mustNotExist(t, promptFile)
}

func TestExample_WorkflowClaude(t *testing.T) {
	prefix := examplesRegistry(t)
	home := setupTestEnv(t)

	dir := copyExample(t, "03-workflow-claude/pr-review")
	rewriteManifestRefs(t, dir, prefix)
	t.Chdir(dir)

	runStriatum(t, "validate")
	runStriatum(t, "pack")

	ref := prefix + "/pr-review:1.0.0"
	runStriatum(t, "push", ref)

	inspectOut := runStriatum(t, "inspect", ref)
	if !strings.Contains(inspectOut, "pr-review") || !strings.Contains(inspectOut, "Workflow") {
		t.Errorf("inspect output does not describe the pushed Workflow:\n%s", inspectOut)
	}

	// Workflows only support --target claude.
	if out, err := runStriatumErr("install", "--target", "cursor", ref); err == nil {
		t.Fatalf("expected install --target cursor to fail for a Workflow, got success:\n%s", out)
	}

	runStriatum(t, "install", "--target", "claude", ref)
	reviewFile := filepath.Join(home, ".claude", "workflows", "pr-review", "review.js")
	mustExist(t, reviewFile)

	// A pr-review.js symlink next to the workflow directory points at the entrypoint.
	linkPath := filepath.Join(home, ".claude", "workflows", "pr-review.js")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected workflow symlink %s: %v", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is not a symlink", linkPath)
	}
	if target, err := os.Readlink(linkPath); err != nil || target != filepath.Join("pr-review", "review.js") {
		t.Errorf("symlink target = %q, %v; want pr-review/review.js", target, err)
	}

	runStriatum(t, "uninstall", "--target", "claude", "pr-review")
	mustNotExist(t, reviewFile)
	mustNotExist(t, linkPath)
}

func TestExample_Memory(t *testing.T) {
	prefix := examplesRegistry(t)
	home := setupTestEnv(t)

	dir := copyExample(t, "04-memory/team-conventions")
	rewriteManifestRefs(t, dir, prefix)
	t.Chdir(dir)

	// The documented lifecycle: validate, pack, push, then inspect and pull.
	runStriatum(t, "validate")
	runStriatum(t, "pack")

	ref := prefix + "/team-conventions:1.0.0"
	runStriatum(t, "push", ref)

	inspectOut := runStriatum(t, "inspect", ref)
	if !strings.Contains(inspectOut, "team-conventions") || !strings.Contains(inspectOut, "Memory") {
		t.Errorf("inspect output does not describe the pushed Memory artifact:\n%s", inspectOut)
	}

	pullDir := t.TempDir()
	runStriatum(t, "pull", ref, "-o", pullDir)
	mustExist(t, filepath.Join(pullDir, "team-conventions", "team-conventions.md"))

	// Memory artifacts are project-scoped and require --project (or
	// --all-existing-projects); without a flag install must fail.
	if out, err := runStriatumErr("install", "--target", "claude", ref); err == nil {
		t.Fatalf("expected install without --project to fail for a Memory artifact, got success:\n%s", out)
	}

	project := t.TempDir()
	runStriatum(t, "install", "--target", "claude", "--project", project, ref)

	// Memory files land in ~/.claude/projects/<slug>/memory/<artifact>/ and
	// the MEMORY.md index in that directory references them.
	memoryDir := filepath.Join(home, ".claude", "projects", memorySlug(project), "memory")
	memoryFile := filepath.Join(memoryDir, "team-conventions", "team-conventions.md")
	mustExist(t, memoryFile)

	index, err := os.ReadFile(filepath.Join(memoryDir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md index: %v", err)
	}
	if !strings.Contains(string(index), "team-conventions/team-conventions.md") {
		t.Errorf("MEMORY.md index does not reference the memory file:\n%s", index)
	}

	// The installed Memory artifact shows up in list --installed.
	listOut := runStriatum(t, "list", "--installed", "--target", "claude")
	if !strings.Contains(listOut, "team-conventions") || !strings.Contains(listOut, "Memory") {
		t.Errorf("installed list does not show the Memory row for team-conventions:\n%s", listOut)
	}

	// --all-existing-projects targets every existing Claude Code project
	// directory. Pre-create a second project directory so the flag has more
	// than one target; a no-op implementation would otherwise go unnoticed.
	secondProject := t.TempDir()
	secondMemoryDir := filepath.Join(home, ".claude", "projects", memorySlug(secondProject), "memory")
	if err := os.MkdirAll(secondMemoryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runStriatum(t, "install", "--target", "claude", "--all-existing-projects", ref)
	mustExist(t, memoryFile)
	secondMemoryFile := filepath.Join(secondMemoryDir, "team-conventions", "team-conventions.md")
	mustExist(t, secondMemoryFile)

	// Uninstalling removes the merged file and its MEMORY.md index entry.
	runStriatum(t, "uninstall", "--target", "claude", "--project", project, "--kind", "Memory", "team-conventions")
	mustNotExist(t, memoryFile)
	indexAfter, err := os.ReadFile(filepath.Join(memoryDir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md index after uninstall: %v", err)
	}
	if strings.Contains(string(indexAfter), "team-conventions") {
		t.Errorf("MEMORY.md index still references the uninstalled memory:\n%s", indexAfter)
	}

	// The all-projects form of uninstall cleans the remaining project.
	runStriatum(t, "uninstall", "--target", "claude", "--all-existing-projects", "--kind", "Memory", "team-conventions")
	mustNotExist(t, secondMemoryFile)
}

func TestExample_OCIDependencies(t *testing.T) {
	prefix := examplesRegistry(t)
	home := setupTestEnv(t)

	base := copyExample(t, "05-oci-dependencies")
	rewriteManifestRefs(t, base, prefix)

	// Push the helpers first so the root's dependencies resolve.
	pushExample(t, filepath.Join(base, "greeting-styles"), prefix+"/greeting-styles:1.0.0")
	pushExample(t, filepath.Join(base, "greeting-tone"), prefix+"/greeting-tone:1.0.0")

	greeterDir := filepath.Join(base, "greeter")
	run := func(args ...string) string {
		cmd := exec.Command(striatumBin, args...)
		cmd.Dir = greeterDir
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		if err := cmd.Run(); err != nil {
			t.Fatalf("striatum %s (in %s): %v\n%s", strings.Join(args, " "), greeterDir, err, buf.String())
		}
		return buf.String()
	}

	// --check-deps verifies the OCI dependencies resolve from the registry.
	run("validate", "--check-deps")

	ref := prefix + "/greeter:1.0.0"
	run("push", ref)

	outDir := t.TempDir()
	runStriatum(t, "pull", ref, "-o", outDir)
	for _, name := range []string{"greeter", "greeting-styles", "greeting-tone"} {
		mustExist(t, filepath.Join(outDir, name, "artifact.json"))
	}

	runStriatum(t, "install", "--target", "claude", ref)
	greeterFile := filepath.Join(home, ".claude", "skills", "greeter", "SKILL.md")
	mustExist(t, greeterFile)
	mustExist(t, filepath.Join(home, ".claude", "skills", "greeting-styles", "SKILL.md"))
	mustExist(t, filepath.Join(home, ".claude", "skills", "greeting-tone", "SKILL.md"))

	// Uninstalling the root also removes its now-orphaned dependencies.
	runStriatum(t, "uninstall", "--target", "claude", "greeter")
	mustNotExist(t, greeterFile)
	mustNotExist(t, filepath.Join(home, ".claude", "skills", "greeting-styles", "SKILL.md"))
	mustNotExist(t, filepath.Join(home, ".claude", "skills", "greeting-tone", "SKILL.md"))
}

func TestExample_GitDependency(t *testing.T) {
	prefix := examplesRegistry(t)
	// Isolate HOME/STRIATUM_HOME before any command runs: pull and install
	// populate the striatum cache, which must not touch the real one.
	home := setupTestEnv(t)

	// Build a local bare git repo standing in for the placeholder
	// https://github.com/example/release-notes-helper.git dependency.
	helperURL := setupGitFixture(t, "release-notes-helper", "1.0.0")

	base := copyExample(t, "06-git-dependency")
	rewriteManifestRefs(t, base, prefix)

	// Point the git dependency at the local fixture.
	dir := filepath.Join(base, "release-notes")
	manifestPath := filepath.Join(dir, "artifact.json")
	editManifest(t, manifestPath, func(m map[string]any) {
		deps, _ := m["dependencies"].([]any)
		if len(deps) != 1 {
			t.Fatalf("expected exactly one dependency in %s, got %d", manifestPath, len(deps))
		}
		dep, _ := deps[0].(map[string]any)
		dep["url"] = helperURL
	})

	t.Chdir(dir)
	runStriatum(t, "validate")
	runStriatum(t, "pack")

	ref := prefix + "/release-notes:1.0.0"
	runStriatum(t, "push", ref)

	outDir := t.TempDir()
	runStriatum(t, "pull", ref, "-o", outDir)

	// The pulled tree contains both the root skill and the git dependency.
	mustExist(t, filepath.Join(outDir, "release-notes", "artifact.json"))
	mustExist(t, filepath.Join(outDir, "release-notes-helper", "SKILL.md"))

	// Install resolves the same closure: the root skill and the git-hosted
	// helper each land in the target's skills directory.
	runStriatum(t, "install", "--target", "claude", ref)
	mustExist(t, filepath.Join(home, ".claude", "skills", "release-notes", "SKILL.md"))
	mustExist(t, filepath.Join(home, ".claude", "skills", "release-notes-helper", "SKILL.md"))
}

func TestExample_SelfContained(t *testing.T) {
	prefix := examplesRegistry(t)
	home := setupTestEnv(t)

	base := copyExample(t, "07-self-contained")
	rewriteManifestRefs(t, base, prefix)

	// Push the helpers first so the bundle's dependencies resolve.
	pushExample(t, filepath.Join(base, "helper-a"), prefix+"/helper-a:1.0.0")
	pushExample(t, filepath.Join(base, "helper-b"), prefix+"/helper-b:1.0.0")

	// --self-contained publishes one image bundling the closure;
	// --print-digest prints the immutable canonical reference.
	ref := pushSelfContained(t, filepath.Join(base, "bundle-skill"), prefix+"/bundle-skill:1.0.0")

	// Install by the immutable digest-only reference: the bundled closure
	// installs as one unit, with dependency content under deps/skills/<name>/.
	runStriatum(t, "install", "--target", "claude", ref)
	mustExist(t, filepath.Join(home, ".claude", "skills", "bundle-skill", "SKILL.md"))
	mustExist(t, filepath.Join(home, ".claude", "skills", "bundle-skill", "deps", "skills", "helper-a", "SKILL.md"))
	mustExist(t, filepath.Join(home, ".claude", "skills", "bundle-skill", "deps", "skills", "helper-b", "SKILL.md"))

	inspectOut := runStriatum(t, "inspect", ref)
	if !strings.Contains(inspectOut, "Self-contained: v1 (dev.striatum.self-contained)") {
		t.Errorf("inspect output does not report the self-contained payload:\n%s", inspectOut)
	}
}

func TestExample_DigestPinning(t *testing.T) {
	prefix := examplesRegistry(t)
	home := setupTestEnv(t)

	base := copyExample(t, "08-digest-pinning")
	rewriteManifestRefs(t, base, prefix)

	// Push the helper and pin its digest in the root's dependency.
	pushOut := pushExample(t, filepath.Join(base, "pinned-helper"), prefix+"/pinned-helper:1.0.0")
	digest := digestFromPushOutput(t, pushOut)

	dir := filepath.Join(base, "pinned-skill")
	manifestPath := filepath.Join(dir, "artifact.json")
	editManifest(t, manifestPath, func(m map[string]any) {
		deps, _ := m["dependencies"].([]any)
		if len(deps) != 1 {
			t.Fatalf("expected exactly one dependency in %s, got %d", manifestPath, len(deps))
		}
		dep, _ := deps[0].(map[string]any)
		dep["digest"] = digest
	})

	t.Chdir(dir)
	// --check-deps resolves the dependency, now pinned to the exact digest.
	runStriatum(t, "validate", "--check-deps")
	runStriatum(t, "pack")

	ref := prefix + "/pinned-skill:1.0.0"
	runStriatum(t, "push", ref)
	runStriatum(t, "install", "--target", "claude", ref)
	mustExist(t, filepath.Join(home, ".claude", "skills", "pinned-skill", "SKILL.md"))

	// The pinned digest keeps resolution immutable: overwrite the helper's
	// tag with different content, and the pinned dependency still resolves
	// to the original bytes the digest identifies. The tampered push also
	// adds a file so the two versions' manifests diverge.
	helperSkill := filepath.Join(base, "pinned-helper", "SKILL.md")
	original, err := os.ReadFile(helperSkill)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helperSkill, append(original, []byte("\nTampered content.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	helperManifest := filepath.Join(base, "pinned-helper", "artifact.json")
	editManifest(t, helperManifest, func(m map[string]any) {
		spec, _ := m["spec"].(map[string]any)
		files, _ := spec["files"].([]any)
		spec["files"] = append(files, "EXTRA.md")
	})
	if err := os.WriteFile(filepath.Join(base, "pinned-helper", "EXTRA.md"), []byte("# Tampered extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pushExample(t, filepath.Join(base, "pinned-helper"), prefix+"/pinned-helper:1.0.0")

	pulledDir := t.TempDir()
	runStriatum(t, "pull", ref, "-o", pulledDir)
	pulledHelper, err := os.ReadFile(filepath.Join(pulledDir, "pinned-helper", "SKILL.md"))
	if err != nil {
		t.Fatalf("pinned helper not present in pulled tree: %v", err)
	}
	if !bytes.Equal(pulledHelper, original) {
		t.Errorf("pinned dependency resolved to tampered content despite the digest pin:\n%s", pulledHelper)
	}

	// A warm cache holding the tampered content under the same tag must not
	// leak into closure traversal either: pulling the helper by its (now
	// moved) tag caches the tampered version, and a subsequent install of
	// the pinned root still has to resolve and install the original bytes.
	warmDir := t.TempDir()
	runStriatum(t, "pull", prefix+"/pinned-helper:1.0.0", "-o", warmDir)
	mustExist(t, filepath.Join(warmDir, "pinned-helper", "EXTRA.md"))

	runStriatum(t, "install", "--target", "claude", ref)
	installedHelper := filepath.Join(home, ".claude", "skills", "pinned-helper", "SKILL.md")
	installedBytes, err := os.ReadFile(installedHelper)
	if err != nil {
		t.Fatalf("installed pinned helper missing: %v", err)
	}
	if !bytes.Equal(installedBytes, original) {
		t.Errorf("installed pinned helper is the tampered content despite the digest pin:\n%s", installedBytes)
	}
	mustNotExist(t, filepath.Join(home, ".claude", "skills", "pinned-helper", "EXTRA.md"))
}

func TestExample_UpdateVersions(t *testing.T) {
	prefix := examplesRegistry(t)
	home := setupTestEnv(t)

	base := copyExample(t, "09-update-versions")
	rewriteManifestRefs(t, base, prefix)
	dir := filepath.Join(base, "changelog-skill")
	manifestPath := filepath.Join(dir, "artifact.json")

	// Publish 1.0.0, then a newer 1.1.0.
	pushExample(t, dir, prefix+"/changelog-skill:1.0.0")
	setManifestVersion(t, manifestPath, "1.1.0")
	pushExample(t, dir, prefix+"/changelog-skill:1.1.0")

	// Install the old version, then update.
	runStriatum(t, "install", "--target", "claude", prefix+"/changelog-skill:1.0.0")
	mustExist(t, filepath.Join(home, ".claude", "skills", "changelog-skill", "SKILL.md"))

	// --yes skips the interactive confirmation prompt.
	runStriatum(t, "update", "changelog-skill", "--target", "claude", "--yes")

	listOut := runStriatum(t, "list", "--installed", "--target", "claude")
	if !strings.Contains(listOut, "changelog-skill") || !strings.Contains(listOut, "1.1.0") {
		t.Errorf("installed list does not show changelog-skill at 1.1.0:\n%s", listOut)
	}

	// The new version is cached and installed.
	mustExist(t, filepath.Join(home, ".striatum", "cache", "Skill", "changelog-skill@1.1.0", "artifact.json"))
	installedManifest := filepath.Join(home, ".claude", "skills", "changelog-skill", "artifact.json")
	data, err := os.ReadFile(installedManifest)
	if err != nil {
		t.Fatalf("read installed manifest: %v", err)
	}
	if !strings.Contains(string(data), `"1.1.0"`) {
		t.Errorf("installed manifest at %s does not reference version 1.1.0:\n%s", installedManifest, data)
	}
}

func TestExample_MLflowSkillRegistry(t *testing.T) {
	prefix := examplesRegistry(t)
	home := setupTestEnv(t)

	if _, err := exec.LookPath("mlflow"); err != nil {
		t.Fatalf("mlflow not in PATH: %v", err)
	}

	base := copyExample(t, "10-mlflow-skill-registry")
	rewriteManifestRefs(t, base, prefix)

	// Push the helper, then the root as one self-contained image: MLflow's
	// OCI fetcher extracts only the referenced image, so it needs the full
	// closure bundled inside it. The printed digest-only ref is what MLflow
	// registers and striatum installs.
	pushExample(t, filepath.Join(base, "mlf-helper"), prefix+"/mlf-helper:1.0.0")
	ref := pushSelfContained(t, filepath.Join(base, "mlf-skill"), prefix+"/mlf-skill:1.0.0")

	// Point MLflow at a local sqlite tracking store (the filesystem backend
	// is in maintenance mode in mlflow 3.x) and register the OCI image as a skill.
	t.Setenv("MLFLOW_TRACKING_URI", "sqlite:///"+filepath.ToSlash(filepath.Join(t.TempDir(), "mlflow.db")))
	t.Setenv("MLFLOW_DISABLE_AGENT_HINT", "1")

	runMLflow := func(args ...string) (string, error) {
		cmd := exec.Command("mlflow", args...)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		// mlflow is slow to start (the first run imports a lot), so every
		// invocation gets a generous timeout instead of the default.
		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			return buf.String(), err
		}
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			return buf.String(), err
		case <-time.After(5 * time.Minute):
			_ = cmd.Process.Kill()
			return buf.String(), fmt.Errorf("mlflow %s timed out after 5m", strings.Join(args, " "))
		}
	}

	registerOut, err := runMLflow("skills", "register", "oci", "--image", ref, "--name", "mlf-skill")
	if err != nil {
		t.Fatalf("mlflow skills register: %v\n%s", err, registerOut)
	}

	pulled := t.TempDir()
	pullOut, err := runMLflow("skills", "pull", "skills:/mlf-skill", "-d", pulled)
	if err != nil {
		t.Fatalf("mlflow skills pull: %v\n%s", err, pullOut)
	}
	pulledSkill := filepath.Join(pulled, "SKILL.md")
	if _, err := os.Stat(pulledSkill); err != nil {
		// Depending on CLI version the pull may nest the artifact one level.
		nested := filepath.Join(pulled, "mlf-skill", "SKILL.md")
		if _, nestedErr := os.Stat(nested); nestedErr != nil {
			t.Fatalf("pulled SKILL.md not found under %s: %v / %v", pulled, err, nestedErr)
		}
		pulledSkill = nested
	}
	if data, err := os.ReadFile(pulledSkill); err != nil {
		t.Fatal(err)
	} else if len(data) == 0 {
		t.Errorf("pulled SKILL.md is empty")
	}

	// The bundled closure came along: the helper is part of the pulled tree.
	pulledRoot := filepath.Dir(pulledSkill)
	mustExist(t, filepath.Join(pulledRoot, "deps", "skills", "mlf-helper", "SKILL.md"))

	// The same immutable reference installs through striatum, closure and all.
	runStriatum(t, "install", "--target", "claude", ref)
	mustExist(t, filepath.Join(home, ".claude", "skills", "mlf-skill", "SKILL.md"))
	mustExist(t, filepath.Join(home, ".claude", "skills", "mlf-skill", "deps", "skills", "mlf-helper", "SKILL.md"))
}
