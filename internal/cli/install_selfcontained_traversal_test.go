package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/installer"
	"github.com/hbelmiro/striatum/pkg/oci"
)

// buildMaliciousSelfContainedLayout packs a self-contained image-marked image whose config
// manifest carries a path-traversal name, mimicking a hostile registry.
func buildMaliciousSelfContainedLayout(t *testing.T, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".striatum"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: x\n---\nboom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".striatum", "provenance.json"), []byte(`{"apiVersion":"striatum.dev/self-contained/v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: name, Version: version},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	base, _ := json.Marshal(m)
	var generic map[string]json.RawMessage
	_ = json.Unmarshal(base, &generic)
	generic["dev.striatum.self-contained"] = json.RawMessage(`"v1"`)
	config, _ := json.Marshal(generic)

	layout := t.TempDir()
	pi := oci.PreparedImage{
		Config:              config,
		ArtifactType:        oci.ArtifactTypeForKind("Skill"),
		Dir:                 dir,
		Files:               []string{"SKILL.md", ".striatum/provenance.json"},
		ManifestAnnotations: map[string]string{"dev.striatum.self-contained": "v1"},
	}
	if _, err := oci.PackPreparedToLayout(context.Background(), pi, layout, "evil:1.0.0"); err != nil {
		t.Fatalf("pack malicious layout: %v", err)
	}
	return layout
}

func TestInstall_SelfContainedRejectsUnsafeNameAndVersion(t *testing.T) {
	for _, tc := range []struct {
		name, version string
	}{
		{"../escape", "1.0.0"},
		{"evil/../../escape", "1.0.0"},
		{".", "1.0.0"},
		{"evil", "../1.0.0"},
		{"evil", "1.0.0/../../x"},
	} {
		t.Run(tc.name+"@"+tc.version, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("STRIATUM_HOME", home)
			t.Setenv("HOME", home)
			layout := buildMaliciousSelfContainedLayout(t, tc.name, tc.version)

			_, err := runCLI(t, "install", "--target", "claude", "oci:"+layout+":evil:1.0.0")
			if err == nil || !strings.Contains(err.Error(), "unsafe") {
				t.Fatalf("err = %v, want unsafe name/version rejection", err)
			}
			// Nothing escaped the intended directories.
			if _, statErr := os.Stat(filepath.Join(home, "escape")); statErr == nil {
				t.Errorf("file escaped to %s", filepath.Join(home, "escape"))
			}
			if _, statErr := os.Stat(filepath.Join(home, ".claude", "escape")); statErr == nil {
				t.Errorf("file escaped to %s", filepath.Join(home, ".claude", "escape"))
			}
			if _, statErr := os.Stat(filepath.Join(installer.CacheRoot(), "escape@1.0.0")); statErr == nil {
				t.Errorf("file escaped to %s", filepath.Join(installer.CacheRoot(), "escape@1.0.0"))
			}
		})
	}
}

func TestPull_SelfContainedRejectsUnsafeName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	layout := buildMaliciousSelfContainedLayout(t, "../escape", "1.0.0")

	outDir := t.TempDir()
	_, err := runCLI(t, "pull", "-o", outDir, "oci:"+layout+":evil:1.0.0")
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("err = %v, want unsafe name/version rejection", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "escape")); statErr == nil {
		t.Errorf("file escaped to %s", filepath.Join(outDir, "escape"))
	}
}

func TestPull_SelfContainedRejectsEmptyName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)
	t.Setenv("HOME", home)
	// pullSelfContained never runs artifact.Validate, so the safety check must
	// reject empty names itself (an empty name produces degenerate paths).
	layout := buildMaliciousSelfContainedLayout(t, "", "1.0.0")

	outDir := t.TempDir()
	_, err := runCLI(t, "pull", "-o", outDir, "oci:"+layout+":evil:1.0.0")
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("err = %v, want unsafe name rejection for empty name", err)
	}
}
