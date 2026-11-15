package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	orasoci "oras.land/oras-go/v2/content/oci"
)

func preparedFixture(t *testing.T) (PreparedImage, string) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"SKILL.md":                        "---\nname: x\n---\n# x\n",
		"deps/prompts/prompt-a/PROMPT.md": "prompt",
		".striatum/provenance.json":       `{"apiVersion":"striatum.dev/self-contained/v1"}`,
	}
	var list []string
	for rel, c := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
		list = append(list, rel)
	}
	config := []byte(`{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"x","version":"1.0.0"},"dev.striatum.self-contained":"v1"}`)
	return PreparedImage{
		Config:              config,
		ArtifactType:        ArtifactTypeForKind("Skill"),
		Dir:                 dir,
		Files:               list,
		ManifestAnnotations: map[string]string{"dev.striatum.self-contained": "v1"},
	}, dir
}

func fetchManifestByTag(t *testing.T, target *orasoci.Store, tag string) (ocispec.Descriptor, ocispec.Manifest) {
	t.Helper()
	ctx := context.Background()
	desc, err := target.Resolve(ctx, tag)
	if err != nil {
		t.Fatalf("resolve %q: %v", tag, err)
	}
	data, err := content.FetchAll(ctx, target, desc)
	if err != nil {
		t.Fatalf("fetch manifest: %v", err)
	}
	var m ocispec.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return desc, m
}

func TestPackPreparedToLayout(t *testing.T) {
	pi, _ := preparedFixture(t)
	layoutDir := t.TempDir()
	desc, err := PackPreparedToLayout(context.Background(), pi, layoutDir, "x:1.0.0")
	if err != nil {
		t.Fatalf("PackPreparedToLayout: %v", err)
	}
	store, err := orasoci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	gotDesc, m := fetchManifestByTag(t, store, "x:1.0.0")
	if gotDesc.Digest != desc.Digest {
		t.Errorf("tagged digest %s != returned %s", gotDesc.Digest, desc.Digest)
	}
	if m.ArtifactType != ArtifactTypeForKind("Skill") {
		t.Errorf("artifactType = %q", m.ArtifactType)
	}
	if m.Annotations["dev.striatum.self-contained"] != "v1" {
		t.Errorf("manifest annotations = %v, want self-contained marker", m.Annotations)
	}
	if len(m.Layers) != len(pi.Files) {
		t.Fatalf("layers = %d, want %d", len(m.Layers), len(pi.Files))
	}
	for _, l := range m.Layers {
		title := l.Annotations["org.opencontainers.image.title"]
		if title == "" {
			t.Fatalf("layer %s has no title annotation", l.Digest)
		}
		data, err := content.FetchAll(context.Background(), store, l)
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(pi.Dir, filepath.FromSlash(title)))
		if err != nil {
			t.Fatalf("layer title %q not in staged files: %v", title, err)
		}
		if string(data) != string(want) {
			t.Errorf("layer %q content mismatch", title)
		}
	}
	// Config blob round-trips with the marker.
	var cfg map[string]any
	cfgData, err := content.FetchAll(context.Background(), store, m.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["dev.striatum.self-contained"] != "v1" {
		t.Errorf("config marker = %v, want v1", cfg["dev.striatum.self-contained"])
	}
}

func TestPackPrepared_DeterministicDigest(t *testing.T) {
	pi, _ := preparedFixture(t)
	d1, err := PackPreparedToLayout(context.Background(), pi, t.TempDir(), "x:1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	d2, err := PackPreparedToLayout(context.Background(), pi, t.TempDir(), "x:1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if d1.Digest != d2.Digest {
		t.Errorf("digests differ across identical preps: %s vs %s", d1.Digest, d2.Digest)
	}
}

func TestPackPrepared_RejectsUnsafeFile(t *testing.T) {
	pi, _ := preparedFixture(t)
	for _, bad := range []string{"../escape.md", "/abs.md", `a\b.md`, "./SKILL.md", "a/./b.md", "a//b.md"} {
		pi2 := pi
		pi2.Files = append(append([]string{}, pi.Files...), bad)
		if _, err := PackPreparedToLayout(context.Background(), pi2, t.TempDir(), "x:1.0.0"); err == nil {
			t.Errorf("expected error for unsafe layer path %q", bad)
		}
	}
}

func TestPushPrepared_RejectsOCIReference(t *testing.T) {
	pi, _ := preparedFixture(t)
	if _, err := PushPrepared(context.Background(), pi, "oci:/tmp/layout:1.0.0"); err == nil {
		t.Error("PushPrepared with oci: ref: expected error")
	}
}

func TestPushPrepared_PacksPayloadForRegistry(t *testing.T) {
	// The registry push itself needs a real registry (integration test); here
	// we verify the prepared payload is packable to a plain oras target with
	// the reference tag, which is what PushPrepared uploads.
	pi, _ := preparedFixture(t)
	mem := memory.New()
	desc, err := packPreparedToTarget(context.Background(), pi, mem, "1.0.0")
	if err != nil {
		t.Fatalf("packPreparedToTarget: %v", err)
	}
	resolved, err := mem.Resolve(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("resolve tag: %v", err)
	}
	if resolved.Digest != desc.Digest {
		t.Errorf("resolved %s != returned %s", resolved.Digest, desc.Digest)
	}
}

// PushPrepared rejects tagless references before contacting any registry.
func TestPushPrepared_TaglessReferenceRejected(t *testing.T) {
	pi, _ := preparedFixture(t)
	for _, ref := range []string{
		"reg.io/repo/no-tag", // no tag, no digest
		"reg.io/repo@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", // digest-only: nothing to push a tag to
	} {
		if _, err := PushPrepared(context.Background(), pi, ref); err == nil {
			t.Errorf("PushPrepared(%q): expected tag-required error", ref)
		}
	}
}

// Empty payloads are rejected before constructing an image.
func TestPackPrepared_EmptyPayloadRejected(t *testing.T) {
	pi, _ := preparedFixture(t)
	pi.Config = nil
	if _, err := PackPreparedToLayout(context.Background(), pi, t.TempDir(), "x:1.0.0"); err == nil {
		t.Error("empty config: expected error")
	}
	pi2, _ := preparedFixture(t)
	pi2.Files = nil
	if _, err := PackPreparedToLayout(context.Background(), pi2, t.TempDir(), "x:1.0.0"); err == nil {
		t.Error("empty files: expected error")
	}
}

// FetchLayerFromManifest errors when no layer carries the requested title.
func TestFetchLayerFromManifest_NoSuchTitle(t *testing.T) {
	pi, _ := preparedFixture(t)
	layout := t.TempDir()
	if _, err := PackPreparedToLayout(context.Background(), pi, layout, "x:1.0.0"); err != nil {
		t.Fatal(err)
	}
	store, err := orasoci.New(layout)
	if err != nil {
		t.Fatal(err)
	}
	ociManifest, _, _, err := LoadImage(context.Background(), store, "x:1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FetchLayerFromManifest(context.Background(), store, ociManifest, "no/such/layer.md"); err == nil {
		t.Error("expected no-layer-titled error")
	}
}
