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
)

const registryImage = "registry:2"
const registryName = "striatum-registry-test"
const registryPort = "5000"

func TestIntegration_PushPullViaRegistry(t *testing.T) {
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
	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL + "/v2/")
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
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(baseDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte("# integration test"), 0o600); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(baseDir); err != nil {
		t.Fatal(err)
	}

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
}
