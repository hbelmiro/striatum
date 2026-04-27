package resolver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

type mockFetcher struct {
	manifests map[string]*artifact.Manifest // keyed by CanonicalRef()
	calls     []string
	err       error
}

func (m *mockFetcher) FetchManifest(_ context.Context, dep artifact.Dependency) (*artifact.Manifest, error) {
	ref := dep.CanonicalRef()
	m.calls = append(m.calls, ref)
	if m.err != nil {
		return nil, m.err
	}
	if mf, ok := m.manifests[ref]; ok {
		return mf, nil
	}
	return nil, errors.New("not found: " + ref)
}

func ociDep(host, repo, tag string) artifact.Dependency {
	return &artifact.OCIDependency{RegistryHost: host, Repository: repo, Tag: tag}
}

func gitDep(url, ref, path string) artifact.Dependency {
	return &artifact.GitDependency{URL: url, Ref: ref, Path: path}
}

func mf(name, version string, deps ...artifact.Dependency) *artifact.Manifest {
	return &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha2",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: name, Version: version},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: deps,
	}
}

func names(resolved []ResolvedArtifact) string {
	n := make([]string, len(resolved))
	for i, r := range resolved {
		n[i] = r.Name
	}
	return strings.Join(n, ",")
}

// --- OCI-only trees ---

func TestResolve_NoDependencies(t *testing.T) {
	got, err := Resolve(context.Background(), mf("root", "1.0.0"), &mockFetcher{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || got[0].Name != "root" {
		t.Errorf("got = %s", names(got))
	}
	if got[0].Dependency != nil {
		t.Error("root Dependency should be nil")
	}
}

func TestResolve_OneOCIDep(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/dep:1.0.0": mf("dep", "1.0.0"),
	}}
	got, err := Resolve(context.Background(), mf("root", "1.0.0", ociDep("reg", "skills/dep", "1.0.0")), f)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if names(got) != "root,dep" {
		t.Errorf("got = %s", names(got))
	}
	if got[1].Dependency == nil || got[1].Dependency.Source() != "oci" {
		t.Error("dep[1] should carry OCI dependency")
	}
}

func TestResolve_TransitiveOCI(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/b:1.0.0": mf("b", "1.0.0", ociDep("reg", "skills/c", "1.0.0")),
		"reg/skills/c:1.0.0": mf("c", "1.0.0"),
	}}
	got, err := Resolve(context.Background(), mf("a", "1.0.0", ociDep("reg", "skills/b", "1.0.0")), f)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if names(got) != "a,b,c" {
		t.Errorf("order = %s", names(got))
	}
}

// --- Git-only trees ---

func TestResolve_OneGitDep(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"git:https://example.com/r.git@v2.0.0": mf("git-dep", "2.0.0"),
	}}
	got, err := Resolve(context.Background(), mf("root", "1.0.0", gitDep("https://example.com/r.git", "v2.0.0", "")), f)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if names(got) != "root,git-dep" {
		t.Errorf("got = %s", names(got))
	}
	if got[1].Dependency.Source() != "git" {
		t.Error("dep should be git")
	}
}

func TestResolve_TransitiveGit(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"git:https://gh.com/a.git@v1": mf("mid", "1.0.0",
			gitDep("https://gh.com/b.git", "v1", "")),
		"git:https://gh.com/b.git@v1": mf("leaf", "1.0.0"),
	}}
	got, err := Resolve(context.Background(), mf("root", "1.0.0",
		gitDep("https://gh.com/a.git", "v1", "")), f)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if names(got) != "root,mid,leaf" {
		t.Errorf("got = %s", names(got))
	}
}

// --- Mixed-backend trees ---

func TestResolve_MixedOCIAndGit(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/oci-child:1.0.0":           mf("oci-child", "1.0.0"),
		"git:https://example.com/r.git@v1.0.0": mf("git-child", "1.0.0"),
	}}
	got, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/oci-child", "1.0.0"),
		gitDep("https://example.com/r.git", "v1.0.0", ""),
	), f)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if names(got) != "root,oci-child,git-child" {
		t.Errorf("got = %s", names(got))
	}
}

func TestResolve_MixedTransitive(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"git:https://gh.com/r.git@v1": mf("mid", "1.0.0",
			ociDep("reg", "skills/leaf", "1.0.0")),
		"reg/skills/leaf:1.0.0": mf("leaf", "1.0.0"),
	}}
	got, err := Resolve(context.Background(), mf("root", "1.0.0",
		gitDep("https://gh.com/r.git", "v1", "")), f)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if names(got) != "root,mid,leaf" {
		t.Errorf("got = %s", names(got))
	}
}

// --- Dedup and cycles ---

func TestResolve_Cycle(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/b:1.0.0": mf("b", "1.0.0", ociDep("reg", "skills/a", "1.0.0")),
		"reg/skills/a:1.0.0": mf("a", "1.0.0", ociDep("reg", "skills/b", "1.0.0")),
	}}
	_, err := Resolve(context.Background(), mf("a", "1.0.0", ociDep("reg", "skills/b", "1.0.0")), f)
	if err == nil {
		t.Fatal("want cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error %q should mention cycle", err.Error())
	}
}

func TestResolve_Dedupe(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/b:1.0.0": mf("b", "1.0.0", ociDep("reg", "skills/c", "1.0.0")),
		"reg/skills/c:1.0.0": mf("c", "1.0.0"),
	}}
	got, err := Resolve(context.Background(), mf("a", "1.0.0",
		ociDep("reg", "skills/b", "1.0.0"),
		ociDep("reg", "skills/c", "1.0.0"),
	), f)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (a,b,c)", len(got))
	}
	cCalls := 0
	for _, ref := range f.calls {
		if ref == "reg/skills/c:1.0.0" {
			cCalls++
		}
	}
	if cCalls != 1 {
		t.Errorf("c fetched %d times, want 1", cCalls)
	}
}

func TestResolve_DedupeAcrossBackends(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/shared:1.0.0":              mf("shared", "1.0.0"),
		"git:https://gh.com/shared.git@v1.0.0": mf("shared", "1.0.0"),
	}}
	got, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/shared", "1.0.0"),
		gitDep("https://gh.com/shared.git", "v1.0.0", ""),
	), f)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// Same name@version from two backends: first wins, second is deduped by name@version.
	if names(got) != "root,shared" {
		t.Errorf("got = %s, want root,shared", names(got))
	}
}

// --- Error cases ---

func TestResolve_NilRoot(t *testing.T) {
	_, err := Resolve(context.Background(), nil, &mockFetcher{})
	if err == nil {
		t.Error("want error for nil root")
	}
}

func TestResolve_NilFetcher(t *testing.T) {
	_, err := Resolve(context.Background(), mf("x", "1"), nil)
	if err == nil {
		t.Error("want error for nil fetcher")
	}
}

func TestResolve_FetchError(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{}}
	_, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/missing", "1.0.0")), f)
	if err == nil {
		t.Error("want error for missing dep")
	}
}

func TestResolve_FetcherReturnsNilManifest(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{}}
	f.manifests["reg/skills/dep:1.0.0"] = nil
	_, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/dep", "1.0.0")), f)
	if err == nil {
		t.Error("want error when fetcher returns nil manifest")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil: %v", err)
	}
}

func TestResolve_FetcherReturnsError(t *testing.T) {
	f := &mockFetcher{err: errors.New("network error")}
	_, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/dep", "1.0.0")), f)
	if err == nil {
		t.Error("want error")
	}
	if !strings.Contains(err.Error(), "network error") {
		t.Errorf("error %q should contain 'network error'", err.Error())
	}
}

// --- Version conflict detection tests ---

func TestResolve_VersionConflict_Transitive(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/alpha:1.0.0":  mf("alpha", "1.0.0", ociDep("reg", "skills/shared", "2.0.0")),
		"reg/skills/bravo:1.0.0":  mf("bravo", "1.0.0", ociDep("reg", "skills/shared", "3.0.0")),
		"reg/skills/shared:2.0.0": mf("shared", "2.0.0"),
		"reg/skills/shared:3.0.0": mf("shared", "3.0.0"),
	}}
	_, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/alpha", "1.0.0"),
		ociDep("reg", "skills/bravo", "1.0.0"),
	), f)
	if err == nil {
		t.Fatal("want error for transitive version conflict")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "dependency version conflict") {
		t.Errorf("error %q should contain 'dependency version conflict'", errMsg)
	}
	if !strings.Contains(errMsg, "shared@2.0.0") {
		t.Errorf("error should mention shared@2.0.0: %v", err)
	}
	if !strings.Contains(errMsg, "shared@3.0.0") {
		t.Errorf("error should mention shared@3.0.0: %v", err)
	}
	if !strings.Contains(errMsg, "alpha") {
		t.Errorf("error should mention alpha as parent: %v", err)
	}
	if !strings.Contains(errMsg, "bravo") {
		t.Errorf("error should mention bravo as parent: %v", err)
	}
}

func TestResolve_VersionConflict_RootVsDep(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/alpha:1.0.0":  mf("alpha", "1.0.0", ociDep("reg", "skills/shared", "2.0.0")),
		"reg/skills/shared:2.0.0": mf("shared", "2.0.0"),
	}}
	_, err := Resolve(context.Background(), mf("shared", "1.0.0",
		ociDep("reg", "skills/alpha", "1.0.0"),
	), f)
	if err == nil {
		t.Fatal("want error for root vs dep version conflict")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "dependency version conflict") {
		t.Errorf("error %q should contain 'dependency version conflict'", errMsg)
	}
	if !strings.Contains(errMsg, "shared@1.0.0") {
		t.Errorf("error should mention shared@1.0.0: %v", err)
	}
	if !strings.Contains(errMsg, "shared@2.0.0") {
		t.Errorf("error should mention shared@2.0.0: %v", err)
	}
	if !strings.Contains(errMsg, "(root)") {
		t.Errorf("error should mention (root) as parent for 1.0.0: %v", err)
	}
	if !strings.Contains(errMsg, "alpha") {
		t.Errorf("error should mention alpha as parent for 2.0.0: %v", err)
	}
}

func TestResolve_VersionConflict_ThreeVersions(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/alpha:1.0.0":  mf("alpha", "1.0.0", ociDep("reg", "skills/shared", "1.0.0")),
		"reg/skills/bravo:1.0.0":  mf("bravo", "1.0.0", ociDep("reg", "skills/shared", "2.0.0")),
		"reg/skills/gamma:1.0.0":  mf("gamma", "1.0.0", ociDep("reg", "skills/shared", "3.0.0")),
		"reg/skills/shared:1.0.0": mf("shared", "1.0.0"),
		"reg/skills/shared:2.0.0": mf("shared", "2.0.0"),
		"reg/skills/shared:3.0.0": mf("shared", "3.0.0"),
	}}
	_, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/alpha", "1.0.0"),
		ociDep("reg", "skills/bravo", "1.0.0"),
		ociDep("reg", "skills/gamma", "1.0.0"),
	), f)
	if err == nil {
		t.Fatal("want error for three-version conflict")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "dependency version conflict") {
		t.Errorf("error %q should contain 'dependency version conflict'", errMsg)
	}
	if !strings.Contains(errMsg, "shared@1.0.0") {
		t.Errorf("error should mention shared@1.0.0: %v", err)
	}
	if !strings.Contains(errMsg, "shared@2.0.0") {
		t.Errorf("error should mention shared@2.0.0: %v", err)
	}
	if !strings.Contains(errMsg, "shared@3.0.0") {
		t.Errorf("error should mention shared@3.0.0: %v", err)
	}
}

func TestResolve_VersionConflict_MixedBackends(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/shared:1.0.0":              mf("shared", "1.0.0"),
		"git:https://gh.com/shared.git@v2.0.0": mf("shared", "2.0.0"),
	}}
	_, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/shared", "1.0.0"),
		gitDep("https://gh.com/shared.git", "v2.0.0", ""),
	), f)
	if err == nil {
		t.Fatal("want error for mixed-backend version conflict")
	}
	if !strings.Contains(err.Error(), "dependency version conflict") {
		t.Errorf("error %q should contain 'dependency version conflict'", err.Error())
	}
	if !strings.Contains(err.Error(), "shared@1.0.0") {
		t.Errorf("error should mention shared@1.0.0: %v", err)
	}
	if !strings.Contains(err.Error(), "shared@2.0.0") {
		t.Errorf("error should mention shared@2.0.0: %v", err)
	}
}

func TestResolve_VersionConflict_MultipleNames(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/alpha:1.0.0": mf("alpha", "1.0.0", ociDep("reg", "skills/lib-x", "1.0.0"), ociDep("reg", "skills/lib-y", "1.0.0")),
		"reg/skills/bravo:1.0.0": mf("bravo", "1.0.0", ociDep("reg", "skills/lib-x", "2.0.0"), ociDep("reg", "skills/lib-y", "2.0.0")),
		"reg/skills/lib-x:1.0.0": mf("lib-x", "1.0.0"),
		"reg/skills/lib-x:2.0.0": mf("lib-x", "2.0.0"),
		"reg/skills/lib-y:1.0.0": mf("lib-y", "1.0.0"),
		"reg/skills/lib-y:2.0.0": mf("lib-y", "2.0.0"),
	}}
	_, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/alpha", "1.0.0"),
		ociDep("reg", "skills/bravo", "1.0.0"),
	), f)
	if err == nil {
		t.Fatal("want error for multiple conflicting names")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "dependency version conflict") {
		t.Errorf("error %q should contain 'dependency version conflict'", errMsg)
	}
	if !strings.Contains(errMsg, "lib-x") {
		t.Errorf("error should mention lib-x: %v", err)
	}
	if !strings.Contains(errMsg, "lib-y") {
		t.Errorf("error should mention lib-y: %v", err)
	}
}

func TestResolve_VersionConflict_ListsAllParents(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/alpha:1.0.0":  mf("alpha", "1.0.0", ociDep("reg", "skills/shared", "2.0.0")),
		"reg/skills/bravo:1.0.0":  mf("bravo", "1.0.0", ociDep("reg", "skills/shared", "2.0.0")),
		"reg/skills/gamma:1.0.0":  mf("gamma", "1.0.0", ociDep("reg", "skills/shared", "3.0.0")),
		"reg/skills/shared:2.0.0": mf("shared", "2.0.0"),
		"reg/skills/shared:3.0.0": mf("shared", "3.0.0"),
	}}
	_, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/alpha", "1.0.0"),
		ociDep("reg", "skills/bravo", "1.0.0"),
		ociDep("reg", "skills/gamma", "1.0.0"),
	), f)
	if err == nil {
		t.Fatal("want error for version conflict")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "dependency version conflict") {
		t.Errorf("error %q should contain 'dependency version conflict'", errMsg)
	}
	if !strings.Contains(errMsg, "required by alpha@1.0.0, bravo@1.0.0") {
		t.Errorf("error should list both parents for shared@2.0.0: %v", err)
	}
	if !strings.Contains(errMsg, "required by gamma@1.0.0") {
		t.Errorf("error should list gamma as parent for shared@3.0.0: %v", err)
	}
}

func TestResolve_VersionConflict_ParentVersionDisambiguation(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/alpha:1.0.0":  mf("alpha", "1.0.0", ociDep("reg", "skills/shared", "2.0.0")),
		"reg/skills/alpha:2.0.0":  mf("alpha", "2.0.0", ociDep("reg", "skills/shared", "3.0.0")),
		"reg/skills/shared:2.0.0": mf("shared", "2.0.0"),
		"reg/skills/shared:3.0.0": mf("shared", "3.0.0"),
	}}
	_, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/alpha", "1.0.0"),
		ociDep("reg", "skills/alpha", "2.0.0"),
	), f)
	if err == nil {
		t.Fatal("want error for version conflict")
	}
	errMsg := err.Error()
	// The shared conflict should attribute parents with version to disambiguate
	// alpha@1.0.0 → shared@2.0.0, alpha@2.0.0 → shared@3.0.0
	if !strings.Contains(errMsg, "shared@2.0.0 (required by alpha@1.0.0)") {
		t.Errorf("error should show versioned parent 'alpha@1.0.0' for shared@2.0.0: %v", err)
	}
	if !strings.Contains(errMsg, "shared@3.0.0 (required by alpha@2.0.0)") {
		t.Errorf("error should show versioned parent 'alpha@2.0.0' for shared@3.0.0: %v", err)
	}
}

func TestResolve_NoConflict_SameVersionDifferentParents(t *testing.T) {
	f := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"reg/skills/alpha:1.0.0":  mf("alpha", "1.0.0", ociDep("reg", "skills/shared", "1.0.0")),
		"reg/skills/bravo:1.0.0":  mf("bravo", "1.0.0", ociDep("reg", "skills/shared", "1.0.0")),
		"reg/skills/shared:1.0.0": mf("shared", "1.0.0"),
	}}
	got, err := Resolve(context.Background(), mf("root", "1.0.0",
		ociDep("reg", "skills/alpha", "1.0.0"),
		ociDep("reg", "skills/bravo", "1.0.0"),
	), f)
	if err != nil {
		t.Fatalf("want no error for same version, got %v", err)
	}
	// shared should appear exactly once (deduped)
	sharedCount := 0
	for _, r := range got {
		if r.Name == "shared" {
			sharedCount++
		}
	}
	if sharedCount != 1 {
		t.Errorf("shared should appear exactly once, got %d", sharedCount)
	}
}
