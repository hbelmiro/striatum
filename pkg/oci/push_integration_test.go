//go:build integration

package oci

import (
	"context"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// startTestRegistry starts a disposable registry:2 container on 127.0.0.1 and
// returns the repository prefix ("localhost:<port>/striatum-test"). It fails
// the test when Docker is unavailable. The container is removed on cleanup.
func startTestRegistry(t *testing.T) string {
	t.Helper()

	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("docker not available (integration tests require Docker): %v: %s", err, out)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	portStr := strconv.Itoa(port)
	containerName := "striatum-test-registry-" + portStr

	// Docker container startup occasionally fails transiently when multiple
	// test binaries start registries in parallel; retry once before failing.
	var out []byte
	for attempt := 0; attempt < 2; attempt++ {
		out, err = exec.Command("docker", "run", "-d", "--rm", "--name", containerName,
			"-p", "127.0.0.1:"+portStr+":5000", "registry:2").CombinedOutput()
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("start registry container: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	})

	addr := "127.0.0.1:" + portStr
	// Probe the HTTP endpoint, not just the TCP port: docker's port forward
	// accepts connections before the registry process inside serves HTTP,
	// and pushing against that window fails with EOF.
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, getErr := client.Get("http://" + addr + "/v2/")
		if getErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				return "localhost:" + portStr + "/striatum-test"
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("registry did not become ready within 30s")
	return ""
}

func TestPush_ToRegistry_Succeeds(t *testing.T) {
	repoPrefix := startTestRegistry(t)
	manifest, baseDir := writePushFixture(t, "registry-push")
	ref := repoPrefix + "/registry-push:1.0.0"

	ctx := context.Background()
	desc, err := Push(ctx, manifest, baseDir, ref)
	if err != nil {
		t.Fatalf("Push() err = %v", err)
	}
	if desc.Size <= 0 {
		t.Errorf("descriptor size = %d, want > 0", desc.Size)
	}

	reg, err := NewRepository(repoPrefix + "/registry-push")
	if err != nil {
		t.Fatal(err)
	}
	resolvedDigest, err := ResolveDigest(ctx, reg, "1.0.0")
	if err != nil {
		t.Fatalf("ResolveDigest() err = %v", err)
	}
	if desc.Digest.String() != resolvedDigest {
		t.Errorf("Push descriptor digest = %q, ResolveDigest = %q; want equal", desc.Digest.String(), resolvedDigest)
	}

	m, err := Inspect(ctx, reg, "1.0.0")
	if err != nil {
		t.Fatalf("Inspect after push: %v", err)
	}
	if m.Metadata.Name != "registry-push" || m.Metadata.Version != "1.0.0" {
		t.Errorf("got name=%q version=%q", m.Metadata.Name, m.Metadata.Version)
	}
}
