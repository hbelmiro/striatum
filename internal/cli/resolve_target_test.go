package cli

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveTargetAndRef_DigestOnly(t *testing.T) {
	digest := "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	target, ref, gotDigest, err := resolveTargetAndRef("localhost:5000/skills/my-skill@" + digest)
	if err != nil {
		t.Fatalf("resolveTargetAndRef digest-only: %v", err)
	}
	if target == nil {
		t.Fatal("resolveTargetAndRef digest-only: target is nil")
	}
	if ref != digest {
		t.Errorf("ref = %q, want the digest %q so oras resolves by digest", ref, digest)
	}
	if gotDigest != digest {
		t.Errorf("digest = %q, want %q", gotDigest, digest)
	}
}

func TestResolveTargetAndRef_DigestOnlyInvalid(t *testing.T) {
	if _, _, _, err := resolveTargetAndRef("localhost:5000/repo@sha256:abc"); err == nil {
		t.Error("resolveTargetAndRef with invalid digest: expected error")
	}
}

func TestResolveTargetAndRef_Tagged(t *testing.T) {
	target, ref, digest, err := resolveTargetAndRef("localhost:5000/skills/my-skill:1.0.0")
	if err != nil {
		t.Fatalf("resolveTargetAndRef tagged: %v", err)
	}
	if target == nil {
		t.Fatal("resolveTargetAndRef tagged: target is nil")
	}
	if ref != "1.0.0" {
		t.Errorf("ref = %q, want %q", ref, "1.0.0")
	}
	if digest != "" {
		t.Errorf("digest = %q, want empty", digest)
	}
}

func TestResolveTargetAndRef_OCILayoutDigestOnly(t *testing.T) {
	// Build a minimal layout with a tagged manifest, then resolve it by digest.
	dir := t.TempDir()
	t.Chdir(dir)
	writeLocalSkill(t, dir, "layout-digest", "1.0.0", "SKILL.md", map[string]string{
		"SKILL.md": "---\nname: layout-digest\n---\nx\n",
	})
	out := &strings.Builder{}
	root := NewRootCommand()
	root.SetOut(out)
	root.SetArgs([]string{"pack", "-o", filepath.Join(dir, "dist")})
	if err := root.Execute(); err != nil {
		t.Fatalf("pack: %v", err)
	}

	// Extract the manifest digest from the layout index.
	indexData, err := os.ReadFile(filepath.Join(dir, "dist", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Manifests) == 0 {
		t.Fatal("no manifests in layout index")
	}
	digest := index.Manifests[0].Digest

	target, ref, gotDigest, err := resolveTargetAndRef("oci:" + filepath.Join(dir, "dist") + "@" + digest)
	if err != nil {
		t.Fatalf("resolveTargetAndRef layout digest-only: %v", err)
	}
	if target == nil {
		t.Fatal("target is nil")
	}
	if ref != digest {
		t.Errorf("ref = %q, want the digest %q", ref, digest)
	}
	if gotDigest != digest {
		t.Errorf("digest = %q, want %q", gotDigest, digest)
	}
}
