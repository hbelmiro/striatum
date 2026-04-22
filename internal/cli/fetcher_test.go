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
	"github.com/hbelmiro/striatum/pkg/resolver"
)

// --- depToCacheCandidate ---

func TestDepToCacheCandidate(t *testing.T) {
	tests := []struct {
		name   string
		dep    artifact.Dependency
		wantN  string
		wantV  string
		wantOK bool
	}{
		{"oci simple", &artifact.OCIDependency{RegistryHost: "reg", Repository: "skill-a", Tag: "1.0.0"}, "skill-a", "1.0.0", true},
		{"oci nested repo", &artifact.OCIDependency{RegistryHost: "reg", Repository: "org/skills/foo", Tag: "2.0.0"}, "foo", "2.0.0", true},
		{"oci empty tag", &artifact.OCIDependency{RegistryHost: "reg", Repository: "repo", Tag: ""}, "repo", "", false},
		{"oci whitespace tag", &artifact.OCIDependency{RegistryHost: "reg", Repository: "repo", Tag: "  "}, "repo", "", false},
		{"oci empty repo segment", &artifact.OCIDependency{RegistryHost: "reg", Repository: "org/", Tag: "1.0"}, "", "1.0", false},
		{"git dep returns false", &artifact.GitDependency{URL: "https://gh.com/r.git", Ref: "v1"}, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, v, ok := depToCacheCandidate(tt.dep)
			if ok != tt.wantOK || n != tt.wantN || v != tt.wantV {
				t.Errorf("depToCacheCandidate() = %q, %q, %v; want %q, %q, %v",
					n, v, ok, tt.wantN, tt.wantV, tt.wantOK)
			}
		})
	}
}

// --- cacheFirstFetcher ---

func setupCachedManifest(t *testing.T, name, version string) {
	t.Helper()
	cacheDir := installer.CacheDir(name, version)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: name, Version: version},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

type stubFetcher struct {
	manifests map[string]*artifact.Manifest
	calls     []string
	err       error
}

func (f *stubFetcher) FetchManifest(_ context.Context, dep artifact.Dependency) (*artifact.Manifest, error) {
	ref := dep.CanonicalRef()
	f.calls = append(f.calls, ref)
	if f.err != nil {
		return nil, f.err
	}
	if m, ok := f.manifests[ref]; ok {
		return m, nil
	}
	return nil, nil
}

func TestCacheFirstFetcher_CacheHit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)

	setupCachedManifest(t, "hit-skill", "1.0.0")

	next := &stubFetcher{}
	fetcher := NewCacheFirstFetcher(next)
	dep := &artifact.OCIDependency{RegistryHost: "reg", Repository: "hit-skill", Tag: "1.0.0"}

	m, err := fetcher.FetchManifest(context.Background(), dep)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if m == nil || m.Metadata.Name != "hit-skill" {
		t.Errorf("manifest = %+v", m)
	}
	if len(next.calls) != 0 {
		t.Errorf("next should not be called on cache hit, got %d calls", len(next.calls))
	}
}

func TestCacheFirstFetcher_CacheMiss_RemoteOK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)

	remoteMf := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "remote-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	next := &stubFetcher{manifests: map[string]*artifact.Manifest{
		"reg/remote-skill:1.0.0": remoteMf,
	}}
	fetcher := NewCacheFirstFetcher(next)
	dep := &artifact.OCIDependency{RegistryHost: "reg", Repository: "remote-skill", Tag: "1.0.0"}

	m, err := fetcher.FetchManifest(context.Background(), dep)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if m == nil || m.Metadata.Name != "remote-skill" {
		t.Errorf("manifest = %+v", m)
	}
	if len(next.calls) != 1 {
		t.Errorf("next should be called once, got %d calls", len(next.calls))
	}
}

func TestCacheFirstFetcher_CacheMiss_RemoteFail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)

	next := &stubFetcher{err: os.ErrNotExist}
	fetcher := NewCacheFirstFetcher(next)
	dep := &artifact.OCIDependency{RegistryHost: "reg", Repository: "fail-skill", Tag: "1.0.0"}

	_, err := fetcher.FetchManifest(context.Background(), dep)
	if err == nil {
		t.Fatal("want error when remote fails")
	}
	if !strings.Contains(err.Error(), "cache miss") {
		t.Errorf("error should mention cache miss: %v", err)
	}
}

func TestCacheFirstFetcher_GitDep_SkipsCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)

	gitMf := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "git-skill", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	next := &stubFetcher{manifests: map[string]*artifact.Manifest{
		"git:https://gh.com/r.git@v1": gitMf,
	}}
	fetcher := NewCacheFirstFetcher(next)
	dep := &artifact.GitDependency{URL: "https://gh.com/r.git", Ref: "v1"}

	m, err := fetcher.FetchManifest(context.Background(), dep)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if m == nil || m.Metadata.Name != "git-skill" {
		t.Errorf("manifest = %+v", m)
	}
	if len(next.calls) != 1 {
		t.Errorf("next should be called directly for Git deps, got %d calls", len(next.calls))
	}
}

// --- remoteFetcher ---

func TestRemoteFetcher_NilDep(t *testing.T) {
	f := NewRemoteFetcher()
	_, err := f.FetchManifest(context.Background(), nil)
	if err == nil {
		t.Fatal("want error for nil dep")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil: %v", err)
	}
}

type customDep struct{}

func (d *customDep) Source() string       { return "custom" }
func (d *customDep) CanonicalRef() string { return "custom:x" }
func (d *customDep) Validate() error      { return nil }

func TestRemoteFetcher_UnsupportedDep(t *testing.T) {
	f := NewRemoteFetcher()
	_, err := f.FetchManifest(context.Background(), &customDep{})
	if err == nil {
		t.Fatal("want error for unsupported dep type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error should mention unsupported: %v", err)
	}
}

// --- loadCachedSkillManifest edge cases ---

func TestLoadCachedSkillManifest_CorruptJSON_Recovers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)

	cacheDir := installer.CacheDir("corrupt", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := loadCachedSkillManifest("corrupt", "1.0.0")
	if err != nil {
		t.Fatalf("should recover from corrupt cache, got err = %v", err)
	}
	if m != nil {
		t.Error("should return nil manifest for corrupt cache")
	}
}

func TestLoadCachedSkillManifest_WrongName_Recovers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)

	cacheDir := installer.CacheDir("wrong-name", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := &artifact.Manifest{
		APIVersion: "striatum.dev/v1alpha2",
		Kind:       "Skill",
		Metadata:   artifact.Metadata{Name: "different-name", Version: "1.0.0"},
		Spec:       artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadCachedSkillManifest("wrong-name", "1.0.0")
	if err != nil {
		t.Fatalf("should recover from mismatched name, got err = %v", err)
	}
	if got != nil {
		t.Error("should return nil manifest for mismatched name")
	}
}

func TestLoadCachedSkillManifest_WrongKind_Recovers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STRIATUM_HOME", home)

	cacheDir := installer.CacheDir("my-prompt", "1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"apiVersion":"striatum.dev/v1alpha2","kind":"Prompt","metadata":{"name":"my-prompt","version":"1.0.0"},"spec":{"entrypoint":"PROMPT.md","files":["PROMPT.md"]}}`)
	if err := os.WriteFile(filepath.Join(cacheDir, "artifact.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadCachedSkillManifest("my-prompt", "1.0.0")
	if err != nil {
		t.Fatalf("should recover from wrong kind, got err = %v", err)
	}
	if got != nil {
		t.Error("should return nil manifest for Kind != Skill")
	}
}

// Ensure the DependencyFetcher interface is satisfied at compile time.
var (
	_ resolver.DependencyFetcher = (*cacheFirstFetcher)(nil)
	_ resolver.DependencyFetcher = (*remoteFetcher)(nil)
)
