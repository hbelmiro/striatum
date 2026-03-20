//go:build integration

package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
)

const registryImage = "registry:2"
const registryName = "striatum-registry-test"
const registryPort = "5000"

func TestIntegration_PushPullViaRegistry(t *testing.T) {
	t.Setenv("STRIATUM_HOME", t.TempDir())
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not in PATH, skipping integration test")
	}

	// Remove leftover container from a previous run
	_ = exec.Command("docker", "rm", "-f", registryName).Run()

	cmd := exec.Command("docker", "run", "-d", "-p", registryPort+":5000", "--name", registryName, registryImage)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not start registry container: %v (%s)", err, string(out))
	}
	defer func() {
		_ = exec.Command("docker", "rm", "-f", registryName).Run()
	}()

	// Wait for registry to be reachable
	baseURL := "http://localhost:" + registryPort
	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 30; i++ {
		resp, err := client.Get(baseURL + "/v2/")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				break
			}
		}
		if i == 29 {
			t.Skip("registry did not become reachable in time")
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Prepare artifact dir and pack
	baseDir := t.TempDir()
	manifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "integration-test", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# integration test"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(baseDir)

	ref := "localhost:" + registryPort + "/demo/integration-test:1.0.0"
	root := NewRootCommand()
	root.SetArgs([]string{"push", ref})
	if err := root.Execute(); err != nil {
		t.Fatalf("push to registry: %v", err)
	}

	outDir := t.TempDir()
	root2 := NewRootCommand()
	root2.SetArgs([]string{"pull", ref, "--output", outDir})
	if err := root2.Execute(); err != nil {
		t.Fatalf("pull from registry: %v", err)
	}

	artifactPath := filepath.Join(outDir, "integration-test", "artifact.json")
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("pulled artifact.json missing: %v", err)
	}
	data2, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	var m artifact.Manifest
	if err := json.Unmarshal(data2, &m); err != nil {
		t.Fatal(err)
	}
	if m.Metadata.Name != "integration-test" || m.Metadata.Version != "1.0.0" {
		t.Errorf("unexpected manifest: %+v", m.Metadata)
	}
	if _, err := os.Stat(filepath.Join(outDir, "integration-test", "SKILL.md")); err != nil {
		t.Errorf("pulled SKILL.md missing: %v", err)
	}
	cacheArtifact := filepath.Join(installer.CacheDir("integration-test", "1.0.0"), "artifact.json")
	if _, err := os.Stat(cacheArtifact); err != nil {
		t.Fatalf("expected pull to populate Striatum cache at %s: %v", cacheArtifact, err)
	}
}

func TestIntegration_PullWithDependencies_DefaultCacheAndNoCache(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not in PATH, skipping integration test")
	}

	_ = exec.Command("docker", "rm", "-f", registryName).Run()

	cmd := exec.Command("docker", "run", "-d", "-p", registryPort+":5000", "--name", registryName, registryImage)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not start registry container: %v (%s)", err, string(out))
	}
	defer func() {
		_ = exec.Command("docker", "rm", "-f", registryName).Run()
	}()

	baseURL := "http://localhost:" + registryPort
	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 30; i++ {
		resp, err := client.Get(baseURL + "/v2/")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				break
			}
		}
		if i == 29 {
			t.Skip("registry did not become reachable in time")
		}
		time.Sleep(200 * time.Millisecond)
	}

	depDir := t.TempDir()
	depManifest := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha1",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "pull-int-dep", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	depData, err := json.Marshal(depManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "artifact.json"), depData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "SKILL.md"), []byte("# dep"), 0o600); err != nil {
		t.Fatal(err)
	}

	rootDir := t.TempDir()
	rootManifest := &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha1",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: "pull-int-root", Version: "1.0.0"},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: []artifact.Dependency{{Name: "pull-int-dep", Version: "1.0.0"}},
	}
	rootData, err := json.Marshal(rootManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "artifact.json"), rootData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "SKILL.md"), []byte("# root"), 0o600); err != nil {
		t.Fatal(err)
	}

	depRef := "localhost:" + registryPort + "/demo/pull-int-dep:1.0.0"
	rootRef := "localhost:" + registryPort + "/demo/pull-int-root:1.0.0"

	t.Chdir(depDir)
	rootCmd := NewRootCommand()
	rootCmd.SetArgs([]string{"push", depRef})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("push dep: %v", err)
	}

	t.Chdir(rootDir)
	rootCmd2 := NewRootCommand()
	rootCmd2.SetArgs([]string{"push", rootRef})
	if err := rootCmd2.Execute(); err != nil {
		t.Fatalf("push root: %v", err)
	}

	t.Run("default_caches_root_and_deps", func(t *testing.T) {
		t.Setenv("STRIATUM_HOME", t.TempDir())
		outDir := t.TempDir()
		pullCmd := NewRootCommand()
		pullCmd.SetArgs([]string{"pull", rootRef, "--output", outDir})
		if err := pullCmd.Execute(); err != nil {
			t.Fatalf("pull: %v", err)
		}
		for _, name := range []string{"pull-int-root", "pull-int-dep"} {
			if _, err := os.Stat(filepath.Join(outDir, name, "artifact.json")); err != nil {
				t.Fatalf("output missing %s: %v", name, err)
			}
		}
		for _, tc := range []struct{ name, ver string }{{"pull-int-root", "1.0.0"}, {"pull-int-dep", "1.0.0"}} {
			p := filepath.Join(installer.CacheDir(tc.name, tc.ver), "artifact.json")
			if _, err := os.Stat(p); err != nil {
				t.Fatalf("expected cache %s: %v", p, err)
			}
		}
	})

	t.Run("no_cache_skips_striatum_cache", func(t *testing.T) {
		t.Setenv("STRIATUM_HOME", t.TempDir())
		outDir := t.TempDir()
		pullCmd := NewRootCommand()
		pullCmd.SetArgs([]string{"pull", "--no-cache", rootRef, "--output", outDir})
		if err := pullCmd.Execute(); err != nil {
			t.Fatalf("pull --no-cache: %v", err)
		}
		for _, name := range []string{"pull-int-root", "pull-int-dep"} {
			if _, err := os.Stat(filepath.Join(outDir, name, "artifact.json")); err != nil {
				t.Fatalf("output missing %s: %v", name, err)
			}
		}
		for _, tc := range []struct{ name, ver string }{{"pull-int-root", "1.0.0"}, {"pull-int-dep", "1.0.0"}} {
			p := filepath.Join(installer.CacheDir(tc.name, tc.ver), "artifact.json")
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				if err == nil {
					t.Fatalf("expected no cache at %s with --no-cache", p)
				}
				t.Fatalf("stat cache: %v", err)
			}
		}
	})
}
