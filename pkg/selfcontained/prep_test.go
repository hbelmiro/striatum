package selfcontained

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/resolver"
)

// stubFetcher serves manifests from a map keyed by CanonicalRef and records
// every fetch so tests can assert source pins were honored.
type stubFetcher struct {
	manifests map[string]*artifact.Manifest
	fetched   []string
}

func (f *stubFetcher) FetchManifest(_ context.Context, dep artifact.Dependency) (*artifact.Manifest, error) {
	ref := dep.CanonicalRef()
	f.fetched = append(f.fetched, ref)
	m, ok := f.manifests[ref]
	if !ok {
		return nil, os.ErrNotExist
	}
	return m, nil
}

// depFixture describes a dependency to stage on disk for MaterializeDep.
type depFixture struct {
	manifest *artifact.Manifest
	files    map[string]string
}

// materializerFor stages dependency files in temp dirs and returns a
// DepMaterializer that records resolved source identities.
func materializerFor(t *testing.T, deps map[string]depFixture) (DepMaterializer, func(name, version string) string) {
	t.Helper()
	cacheDirs := map[string]string{}
	for key, fx := range deps {
		dir := t.TempDir()
		for rel, content := range fx.files {
			p := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		cacheDirs[key] = dir
	}
	return func(_ context.Context, r resolver.ResolvedArtifact) (DepMaterialization, error) {
		key := r.Name + "@" + r.Version
		dir, ok := cacheDirs[key]
		if !ok {
			return DepMaterialization{}, os.ErrNotExist
		}
		src := sourceFromDependency(r.Dependency)
		return DepMaterialization{CacheDir: dir, Source: src}, nil
	}, func(name, version string) string { return cacheDirs[name+"@"+version] }
}

func rootSkillManifest(name string, deps ...artifact.Dependency) *artifact.Manifest {
	return &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha2",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: name, Version: "1.0.0"},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: deps,
	}
}

func depManifest(kind, name string, files []string, deps ...artifact.Dependency) *artifact.Manifest {
	return &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha2",
		Kind:         kind,
		Metadata:     artifact.Metadata{Name: name, Version: "1.0.0"},
		Spec:         artifact.Spec{Entrypoint: files[0], Files: files},
		Dependencies: deps,
	}
}

func ociDep(repo, tag, digest string) *artifact.OCIDependency {
	return &artifact.OCIDependency{RegistryHost: "reg.io", Repository: repo, Tag: tag, Digest: digest}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPrepare_StandaloneSkill(t *testing.T) {
	root := rootSkillManifest("solo")
	root.Spec.Files = []string{"SKILL.md", "helpers/rubric.md"}
	project := writeProject(t, map[string]string{
		"SKILL.md":          "---\nname: solo\n---\n# Solo\n",
		"helpers/rubric.md": "rubric",
	})
	p, err := Prepare(context.Background(), Options{Manifest: root, ProjectRoot: project, StagingParent: t.TempDir()})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer p.Cleanup()
	for _, f := range []string{"SKILL.md", "helpers/rubric.md", ".striatum/provenance.json"} {
		if _, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("staged file %q missing: %v", f, err)
		}
	}
	if len(p.Files) != 3 {
		t.Errorf("Files = %v, want 3 entries", p.Files)
	}
	if p.Provenance == nil || len(p.Provenance.Dependencies) != 0 {
		t.Errorf("Provenance = %+v, want no dependencies", p.Provenance)
	}
}

func TestPrepare_MultiLevelClosure(t *testing.T) {
	promptDep := depManifest("Prompt", "review-policy", []string{"PROMPT.md", "examples/case.md"})
	skillDep := depManifest("Skill", "generic-review", []string{"SKILL.md"}, ociDep("prompts/review-policy", "1.0.0", ""))
	root := rootSkillManifest("pr-review", ociDep("skills/generic-review", "1.0.0", ""))
	root.Spec.Files = []string{"SKILL.md"}

	fetcher := &stubFetcher{manifests: map[string]*artifact.Manifest{
		"reg.io/skills/generic-review:1.0.0": skillDep,
		"reg.io/prompts/review-policy:1.0.0": promptDep,
	}}
	mat, _ := materializerFor(t, map[string]depFixture{
		"generic-review@1.0.0": {manifest: skillDep, files: map[string]string{
			"SKILL.md": "---\nname: generic-review\n---\nUses [policy](../../prompts/review-policy/PROMPT.md).\n",
		}},
		"review-policy@1.0.0": {manifest: promptDep, files: map[string]string{
			"PROMPT.md":        "policy",
			"examples/case.md": "case",
		}},
	})

	project := writeProject(t, map[string]string{
		"SKILL.md": "---\nname: pr-review\n---\nSee [gr](../generic-review/SKILL.md).\n",
	})
	p, err := Prepare(context.Background(), Options{
		Manifest: root, ProjectRoot: project, Fetcher: fetcher, MaterializeDep: mat,
		StagingParent: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer p.Cleanup()

	// Dep files staged at deps/<kinddir>/<name>/...
	for _, f := range []string{
		"SKILL.md",
		"deps/skills/generic-review/SKILL.md",
		"deps/prompts/review-policy/PROMPT.md",
		"deps/prompts/review-policy/examples/case.md",
		".striatum/provenance.json",
	} {
		if _, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("staged file %q missing: %v", f, err)
		}
	}

	// Root link rewritten into the image tree.
	rootMD, _ := os.ReadFile(filepath.Join(p.Dir, "SKILL.md"))
	if !strings.Contains(string(rootMD), "](deps/skills/generic-review/SKILL.md)") {
		t.Errorf("root SKILL.md links not rewritten: %q", rootMD)
	}
	// Transitive dep link rewritten.
	depMD, _ := os.ReadFile(filepath.Join(p.Dir, "deps", "skills", "generic-review", "SKILL.md"))
	if !strings.Contains(string(depMD), "](../../prompts/review-policy/PROMPT.md)") {
		t.Errorf("dep SKILL.md links not rewritten: %q", depMD)
	}

	// Provenance keeps declarations and resolved sources.
	if len(p.Provenance.Dependencies) != 2 {
		t.Fatalf("Provenance deps = %d, want 2", len(p.Provenance.Dependencies))
	}
	wantPaths := map[string][]string{
		"generic-review": {"deps/skills/generic-review/SKILL.md"},
		"review-policy":  {"deps/prompts/review-policy/PROMPT.md", "deps/prompts/review-policy/examples/case.md"},
	}
	for _, d := range p.Provenance.Dependencies {
		if got, want := d.ImagePaths, wantPaths[d.Name]; !equalStrings(got, want) {
			t.Errorf("dep %s imagePaths = %v, want %v", d.Name, got, want)
		}
	}
	prov, _ := json.Marshal(p.Provenance)
	for _, want := range []string{"reg.io/skills/generic-review:1.0.0", "reg.io/prompts/review-policy:1.0.0", "pr-review"} {
		if !strings.Contains(string(prov), want) {
			t.Errorf("provenance %s missing %q", prov, want)
		}
	}
}

func TestPrepare_NonSkillRootRejected(t *testing.T) {
	root := rootSkillManifest("x")
	root.Kind = "Workflow"
	project := writeProject(t, map[string]string{"SKILL.md": "x"})
	_, err := Prepare(context.Background(), Options{Manifest: root, ProjectRoot: project, StagingParent: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "Skill") {
		t.Fatalf("err = %v, want non-Skill root rejection", err)
	}
}

func TestPrepare_UnsupportedDependencyKindRejected(t *testing.T) {
	workflowDep := depManifest("Workflow", "wf", []string{"flow.js"})
	root := rootSkillManifest("x", ociDep("workflows/wf", "1.0.0", ""))
	fetcher := &stubFetcher{manifests: map[string]*artifact.Manifest{
		"reg.io/workflows/wf:1.0.0": workflowDep,
	}}
	mat, _ := materializerFor(t, map[string]depFixture{"wf@1.0.0": {manifest: workflowDep, files: map[string]string{"flow.js": "x"}}})
	project := writeProject(t, map[string]string{"SKILL.md": "---\nname: x\n---\n"})
	_, err := Prepare(context.Background(), Options{
		Manifest: root, ProjectRoot: project, Fetcher: fetcher, MaterializeDep: mat,
		StagingParent: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "Workflow") {
		t.Fatalf("err = %v, want unsupported dependency kind rejection", err)
	}
}

func TestPrepare_CycleSurfacesResolverError(t *testing.T) {
	a := depManifest("Skill", "skill-a", []string{"SKILL.md"}, ociDep("skills/skill-b", "1.0.0", ""))
	b := depManifest("Skill", "skill-b", []string{"SKILL.md"}, ociDep("skills/skill-a", "1.0.0", ""))
	root := rootSkillManifest("x", ociDep("skills/skill-a", "1.0.0", ""))
	fetcher := &stubFetcher{manifests: map[string]*artifact.Manifest{
		"reg.io/skills/skill-a:1.0.0": a,
		"reg.io/skills/skill-b:1.0.0": b,
	}}
	project := writeProject(t, map[string]string{"SKILL.md": "---\nname: x\n---\n"})
	_, err := Prepare(context.Background(), Options{
		Manifest: root, ProjectRoot: project, Fetcher: fetcher, StagingParent: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("err = %v, want cycle error", err)
	}
}

func TestPrepare_MissingDepFileFailsClearly(t *testing.T) {
	dep := depManifest("Skill", "ghost-files", []string{"SKILL.md", "missing.md"})
	root := rootSkillManifest("x", ociDep("skills/ghost-files", "1.0.0", ""))
	fetcher := &stubFetcher{manifests: map[string]*artifact.Manifest{
		"reg.io/skills/ghost-files:1.0.0": dep,
	}}
	mat, _ := materializerFor(t, map[string]depFixture{
		"ghost-files@1.0.0": {manifest: dep, files: map[string]string{"SKILL.md": "x"}}, // missing.md absent
	})
	project := writeProject(t, map[string]string{"SKILL.md": "---\nname: x\n---\n"})
	_, err := Prepare(context.Background(), Options{
		Manifest: root, ProjectRoot: project, Fetcher: fetcher, MaterializeDep: mat,
		StagingParent: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("err = %v, want missing-file error naming missing.md", err)
	}
}

func TestPrepare_SourcePinsHonored(t *testing.T) {
	pinnedDigest := "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	dep := depManifest("Skill", "pinned", []string{"SKILL.md"})
	root := rootSkillManifest("x", ociDep("skills/pinned", "1.0.0", pinnedDigest))
	fetcher := &stubFetcher{manifests: map[string]*artifact.Manifest{
		"reg.io/skills/pinned:1.0.0@" + pinnedDigest: dep,
	}}
	mat, _ := materializerFor(t, map[string]depFixture{
		"pinned@1.0.0": {manifest: dep, files: map[string]string{"SKILL.md": "x"}},
	})
	project := writeProject(t, map[string]string{"SKILL.md": "---\nname: x\n---\n"})
	p, err := Prepare(context.Background(), Options{
		Manifest: root, ProjectRoot: project, Fetcher: fetcher, MaterializeDep: mat,
		StagingParent: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer p.Cleanup()
	if len(fetcher.fetched) == 0 || !strings.Contains(fetcher.fetched[0], pinnedDigest) {
		t.Errorf("fetched = %v, want pinned canonical ref with digest", fetcher.fetched)
	}
	// Provenance records the pin.
	prov, _ := json.Marshal(p.Provenance)
	if !strings.Contains(string(prov), pinnedDigest) {
		t.Errorf("provenance %s missing pinned digest", prov)
	}
}

func TestPrepare_DoesNotMutateSources(t *testing.T) {
	root := rootSkillManifest("solo")
	root.Spec.Files = []string{"SKILL.md"}
	skillContent := "---\nname: solo\n---\n# Solo\n"
	project := writeProject(t, map[string]string{"SKILL.md": skillContent})
	before, _ := os.ReadFile(filepath.Join(project, "SKILL.md"))

	p, err := Prepare(context.Background(), Options{Manifest: root, ProjectRoot: project, StagingParent: t.TempDir()})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer p.Cleanup()
	after, _ := os.ReadFile(filepath.Join(project, "SKILL.md"))
	if string(before) != string(after) {
		t.Fatalf("source file mutated: %q -> %q", before, after)
	}
}

func TestPrepare_CleansUpStagingOnFailure(t *testing.T) {
	root := rootSkillManifest("x")
	root.Spec.Files = []string{"SKILL.md", "gone.md"} // gone.md missing
	project := writeProject(t, map[string]string{"SKILL.md": "x"})
	parent := t.TempDir()
	_, err := Prepare(context.Background(), Options{Manifest: root, ProjectRoot: project, StagingParent: parent})
	if err == nil {
		t.Fatal("expected error for missing root file")
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 0 {
		t.Errorf("staging left behind: %v", entries)
	}
}

func TestConfigWithMarker(t *testing.T) {
	m := rootSkillManifest("x")
	data, err := ConfigWithMarker(m)
	if err != nil {
		t.Fatalf("ConfigWithMarker: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if generic["dev.striatum.self-contained"] != "v1" {
		t.Errorf("marker = %v, want v1", generic["dev.striatum.self-contained"])
	}
	// The manifest still round-trips through artifact.Manifest.
	var back artifact.Manifest
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal as manifest: %v", err)
	}
	if back.Metadata.Name != "x" || back.Kind != "Skill" {
		t.Errorf("round-trip lost fields: %+v", back)
	}
}

func TestPrepare_NormalizesSpecFiles(t *testing.T) {
	// A manifest declaring non-canonical paths like "./helpers/rubric.md"
	// must still export: links resolve against normalized paths.
	root := rootSkillManifest("norm")
	root.Spec.Files = []string{"SKILL.md", "./helpers/rubric.md"}
	project := writeProject(t, map[string]string{
		"SKILL.md":          "---\nname: norm\n---\n[r](helpers/rubric.md)\n",
		"helpers/rubric.md": "rubric",
	})
	p, err := Prepare(context.Background(), Options{Manifest: root, ProjectRoot: project, StagingParent: t.TempDir()})
	if err != nil {
		t.Fatalf("Prepare with non-canonical spec.files: %v", err)
	}
	defer p.Cleanup()
	for _, want := range []string{"SKILL.md", "helpers/rubric.md"} {
		if _, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(want))); err != nil {
			t.Errorf("staged file %q missing: %v", want, err)
		}
	}
}

// A hostile registry can serve a dependency manifest whose name or version
// contains path separators; Prepare must reject it before building
// deps/<kinddir>/<name> image paths.
func TestPrepare_HostileDependencyNameRejected(t *testing.T) {
	for _, tc := range []struct{ name, version string }{
		{"../x", "1.0.0"},
		{"evil/../../x", "1.0.0"},
		{".", "1.0.0"},
		{"ok", "1/0"},
		{"", "1.0.0"},
		{"  ", "1.0.0"},
		{"ok", ""},
	} {
		t.Run(tc.name+"@"+tc.version, func(t *testing.T) {
			dep := depManifest("Skill", tc.name, []string{"SKILL.md"})
			dep.Metadata.Version = tc.version
			root := rootSkillManifest("hostile-root", ociDep("skills/"+tc.name, "1.0.0", ""))
			fetcher := &stubFetcher{manifests: map[string]*artifact.Manifest{
				"reg.io/skills/" + tc.name + ":1.0.0": dep,
			}}
			mat, _ := materializerFor(t, map[string]depFixture{
				tc.name + "@1.0.0": {manifest: dep, files: map[string]string{"SKILL.md": "x"}},
			})
			project := writeProject(t, map[string]string{"SKILL.md": "---\nname: hostile-root\n---\nx\n"})
			parent := t.TempDir()
			_, err := Prepare(context.Background(), Options{
				Manifest: root, ProjectRoot: project, Fetcher: fetcher, MaterializeDep: mat,
				StagingParent: parent,
			})
			if err == nil || !strings.Contains(err.Error(), "unsafe") {
				t.Fatalf("err = %v, want unsafe name/version rejection", err)
			}
			entries, _ := os.ReadDir(parent)
			if len(entries) != 0 {
				t.Errorf("staging left behind after failure: %v", entries)
			}
		})
	}
}

// The consumer-constraint gate inside Prepare must fire: an over-budget
// export fails and the staging directory is cleaned up.
func TestPrepare_ConstraintGateFires(t *testing.T) {
	root := rootSkillManifest("big")
	root.Spec.Files = []string{"SKILL.md", "big.bin"}
	project := writeProject(t, map[string]string{
		"SKILL.md": "---\nname: big\n---\nx\n",
		"big.bin":  strings.Repeat("x", 1024),
	})
	parent := t.TempDir()
	_, err := Prepare(context.Background(), Options{
		Manifest: root, ProjectRoot: project, StagingParent: parent,
		Constraints: MLflowOptions{MaxSizeBytes: 512},
	})
	if err == nil || !strings.Contains(err.Error(), "MLflow compatibility") {
		t.Fatalf("err = %v, want constraint gate failure", err)
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 0 {
		t.Errorf("staging left behind after constraint failure: %v", entries)
	}
}

// IsMarked is the routing predicate for install/pull/inspect: only the
// exact "v1" marker on a parseable config routes an image to the
// self-contained flows.
func TestIsMarked(t *testing.T) {
	plain, err := json.Marshal(rootSkillManifest("x"))
	if err != nil {
		t.Fatal(err)
	}
	marked, err := ConfigWithMarker(rootSkillManifest("x"))
	if err != nil {
		t.Fatal(err)
	}
	var v2 map[string]json.RawMessage
	if err := json.Unmarshal(marked, &v2); err != nil {
		t.Fatal(err)
	}
	v2[MarkerKey] = json.RawMessage(`"v2"`)
	v2Bytes, _ := json.Marshal(v2)
	var nonString map[string]json.RawMessage
	if err := json.Unmarshal(marked, &nonString); err != nil {
		t.Fatal(err)
	}
	nonString[MarkerKey] = json.RawMessage(`42`)
	nonStringBytes, _ := json.Marshal(nonString)

	tests := []struct {
		name    string
		config  []byte
		wantMar bool
	}{
		{"marked config", marked, true},
		{"ordinary manifest", plain, false},
		{"invalid JSON", []byte("{not json"), false},
		{"empty", nil, false},
		{"future marker version", v2Bytes, false},
		{"non-string marker", nonStringBytes, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMarked(tt.config); got != tt.wantMar {
				t.Errorf("IsMarked(%s) = %v, want %v", tt.config, got, tt.wantMar)
			}
		})
	}
}

// A dependency whose name collides with the root artifact's name is rejected
// by the resolver's version-conflict detection (same name, two versions);
// the duplicate-virtual-root guard in Prepare is defense-in-depth on top.
func TestPrepare_RootAndDependencyNameCollisionRejected(t *testing.T) {
	dep := depManifest("Skill", "shared", []string{"SKILL.md"})
	dep.Metadata.Version = "2.0.0"
	root := rootSkillManifest("shared", ociDep("skills/shared", "2.0.0", ""))
	root.Metadata.Version = "1.0.0"
	fetcher := &stubFetcher{manifests: map[string]*artifact.Manifest{
		"reg.io/skills/shared:2.0.0": dep,
	}}
	mat, _ := materializerFor(t, map[string]depFixture{
		"shared@2.0.0": {manifest: dep, files: map[string]string{"SKILL.md": "x"}},
	})
	project := writeProject(t, map[string]string{"SKILL.md": "---\nname: shared\n---\nx\n"})
	_, err := Prepare(context.Background(), Options{
		Manifest: root, ProjectRoot: project, Fetcher: fetcher, MaterializeDep: mat,
		StagingParent: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "version conflict") {
		t.Fatalf("err = %v, want version conflict for root/dependency name collision", err)
	}
}
