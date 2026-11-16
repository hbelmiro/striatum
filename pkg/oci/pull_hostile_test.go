package oci

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

// buildHostileImage packs an image whose layer title annotations are
// attacker-controlled, bypassing the pack-side checkLayerPath checks (a
// hostile registry does not use our packer).
func buildHostileImage(t *testing.T, layerTitles []string, name string) *memory.Store {
	t.Helper()
	ctx := context.Background()
	mem := memory.New()

	configBytes := []byte(`{"apiVersion":"striatum.dev/v1alpha2","kind":"Skill","metadata":{"name":"` + name + `","version":"1.0.0"},"spec":{"entrypoint":"SKILL.md","files":["SKILL.md"]}}`)
	configDesc := content.NewDescriptorFromBytes(ConfigMediaType, configBytes)
	if err := pushBlob(ctx, mem, configDesc, newByteReader(configBytes)); err != nil {
		t.Fatal(err)
	}

	var layers []ocispec.Descriptor
	for _, title := range layerTitles {
		data := []byte("evil")
		if title == "" {
			data = []byte("untitled")
		}
		layerDesc := content.NewDescriptorFromBytes(LayerMediaType, data)
		if title != "" {
			layerDesc.Annotations = map[string]string{annotationTitle: title}
		}
		if err := pushBlob(ctx, mem, layerDesc, newByteReader(data)); err != nil {
			t.Fatal(err)
		}
		layers = append(layers, layerDesc)
	}

	opts := oras.PackManifestOptions{ConfigDescriptor: &configDesc, Layers: layers}
	manifestDesc, err := oras.PackManifest(ctx, mem, oras.PackManifestVersion1_1, ArtifactTypeForKind("Skill"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.Tag(ctx, manifestDesc, "evil:1.0.0"); err != nil {
		t.Fatal(err)
	}
	return mem
}

// The layer title annotation is attacker-controlled content; Pull must
// reject titles that escape the artifact directory and must not write
// anything outside outputDir/<name>/.
func TestPull_HostileLayerTitlesRejected(t *testing.T) {
	for _, title := range []string{
		"../../../../.ssh/evil",
		"/abs/escape",
		`a\b.md`,
		"a/../../escape",
	} {
		t.Run(title, func(t *testing.T) {
			mem := buildHostileImage(t, []string{title}, "hostile")
			outDir := t.TempDir()
			if err := Pull(context.Background(), mem, "evil:1.0.0", outDir); err == nil {
				t.Fatalf("Pull accepted hostile layer title %q", title)
			}
			// Nothing outside outputDir/<name>/ was written.
			entries, err := os.ReadDir(outDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if e.Name() != "hostile" {
					t.Errorf("file %q written outside the artifact dir", e.Name())
				}
			}
			// And nothing escaped upward: the parent of outDir gained no evil files.
			parentEntries, err := os.ReadDir(filepath.Dir(outDir))
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range parentEntries {
				if e.Name() == ".ssh" || e.Name() == "escape" || e.Name() == "abs" {
					t.Errorf("hostile layer title %q escaped to parent as %q", title, e.Name())
				}
			}
		})
	}
}

// A layer with no title annotation falls back to its digest hex name and
// extracts inside the artifact directory.
func TestPull_UntitledLayerUsesDigestFallback(t *testing.T) {
	mem := buildHostileImage(t, []string{""}, "fallback")
	outDir := t.TempDir()
	if err := Pull(context.Background(), mem, "evil:1.0.0", outDir); err != nil {
		t.Fatalf("Pull with untitled layer: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(outDir, "fallback"))
	if err != nil {
		t.Fatal(err)
	}
	// artifact.json + the digest-named layer.
	if len(entries) != 2 {
		t.Fatalf("extracted entries = %d, want 2: %v", len(entries), entries)
	}
	for _, e := range entries {
		if e.Name() != "artifact.json" && len(e.Name()) != 64 {
			t.Errorf("unexpected extracted file %q (want artifact.json or a 64-hex digest name)", e.Name())
		}
	}
}
