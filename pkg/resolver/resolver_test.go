package resolver

import (
	"context"
	"errors"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// mockFetcher records FetchManifest calls and returns manifests from a map.
type mockFetcher struct {
	manifests map[string]*artifact.Manifest
	calls     []string
	err       error
}

func (m *mockFetcher) FetchManifest(ctx context.Context, reference string) (*artifact.Manifest, error) {
	m.calls = append(m.calls, reference)
	if m.err != nil {
		return nil, m.err
	}
	if m.manifests != nil {
		if manifest, ok := m.manifests[reference]; ok {
			return manifest, nil
		}
	}
	return nil, errors.New("not found")
}

func manifestWithDeps(name, version, registry string, deps []artifact.Dependency) *artifact.Manifest {
	return &artifact.Manifest{
		APIVersion:   "striatum.dev/v1alpha1",
		Kind:         "Skill",
		Metadata:     artifact.Metadata{Name: name, Version: version},
		Spec:         artifact.Spec{Entrypoint: "SKILL.md", Files: []string{"SKILL.md"}},
		Dependencies: deps,
	}
}

func TestResolve_NoDependencies(t *testing.T) {
	root := manifestWithDeps("root", "1.0.0", "", nil)
	fetcher := &mockFetcher{manifests: map[string]*artifact.Manifest{}}
	got, err := Resolve(context.Background(), root, "localhost:5000/skills", fetcher)
	if err != nil {
		t.Fatalf("Resolve() err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Name != "root" || got[0].Version != "1.0.0" || got[0].Registry != "localhost:5000/skills" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if len(fetcher.calls) != 0 {
		t.Errorf("fetcher called %d times, want 0", len(fetcher.calls))
	}
}

func TestResolve_OneDirectDependency(t *testing.T) {
	dep := manifestWithDeps("dep", "1.0.0", "", nil)
	root := manifestWithDeps("root", "1.0.0", "", []artifact.Dependency{{Name: "dep", Version: "1.0.0"}})
	fetcher := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"localhost:5000/skills/dep:1.0.0": dep,
	}}
	got, err := Resolve(context.Background(), root, "localhost:5000/skills", fetcher)
	if err != nil {
		t.Fatalf("Resolve() err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Name != "root" || got[1].Name != "dep" {
		t.Errorf("order: got[0].Name=%s got[1].Name=%s", got[0].Name, got[1].Name)
	}
	if len(fetcher.calls) != 1 || fetcher.calls[0] != "localhost:5000/skills/dep:1.0.0" {
		t.Errorf("fetcher.calls = %v", fetcher.calls)
	}
}

func TestResolve_Transitive(t *testing.T) {
	c := manifestWithDeps("c", "1.0.0", "", nil)
	b := manifestWithDeps("b", "1.0.0", "", []artifact.Dependency{{Name: "c", Version: "1.0.0"}})
	a := manifestWithDeps("a", "1.0.0", "", []artifact.Dependency{{Name: "b", Version: "1.0.0"}})
	fetcher := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"localhost:5000/skills/b:1.0.0": b,
		"localhost:5000/skills/c:1.0.0": c,
	}}
	got, err := Resolve(context.Background(), a, "localhost:5000/skills", fetcher)
	if err != nil {
		t.Fatalf("Resolve() err = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	names := []string{got[0].Name, got[1].Name, got[2].Name}
	if names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Errorf("order = %v", names)
	}
	if len(fetcher.calls) != 2 {
		t.Errorf("fetcher.calls = %v (want 2 calls for b and c)", fetcher.calls)
	}
}

func TestResolve_Cycle(t *testing.T) {
	a := manifestWithDeps("a", "1.0.0", "", []artifact.Dependency{{Name: "b", Version: "1.0.0"}})
	b := manifestWithDeps("b", "1.0.0", "", []artifact.Dependency{{Name: "a", Version: "1.0.0"}})
	fetcher := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"localhost:5000/skills/b:1.0.0": b,
		"localhost:5000/skills/a:1.0.0": a,
	}}
	_, err := Resolve(context.Background(), a, "localhost:5000/skills", fetcher)
	if err == nil {
		t.Error("Resolve() err = nil, want cycle error")
	}
}

func TestResolve_Dedupe(t *testing.T) {
	c := manifestWithDeps("c", "1.0.0", "", nil)
	b := manifestWithDeps("b", "1.0.0", "", []artifact.Dependency{{Name: "c", Version: "1.0.0"}})
	a := manifestWithDeps("a", "1.0.0", "", []artifact.Dependency{
		{Name: "b", Version: "1.0.0"},
		{Name: "c", Version: "1.0.0"},
	})
	fetcher := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"localhost:5000/skills/b:1.0.0": b,
		"localhost:5000/skills/c:1.0.0": c,
	}}
	got, err := Resolve(context.Background(), a, "localhost:5000/skills", fetcher)
	if err != nil {
		t.Fatalf("Resolve() err = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3 (a, b, c)", len(got))
	}
	// c should be fetched once (via b or via a)
	cCalls := 0
	for _, ref := range fetcher.calls {
		if ref == "localhost:5000/skills/c:1.0.0" {
			cCalls++
		}
	}
	if cCalls != 1 {
		t.Errorf("c fetched %d times, want 1", cCalls)
	}
}

func TestResolve_DefaultRegistry(t *testing.T) {
	dep := manifestWithDeps("dep", "1.0.0", "", nil)
	root := manifestWithDeps("root", "1.0.0", "", []artifact.Dependency{{Name: "dep", Version: "1.0.0"}}) // no registry on dep
	fetcher := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"localhost:5000/skills/dep:1.0.0": dep,
	}}
	_, err := Resolve(context.Background(), root, "localhost:5000/skills", fetcher)
	if err != nil {
		t.Fatalf("Resolve() err = %v", err)
	}
	if len(fetcher.calls) != 1 || fetcher.calls[0] != "localhost:5000/skills/dep:1.0.0" {
		t.Errorf("fetcher.calls = %v", fetcher.calls)
	}
}

func TestResolve_DepWithExplicitRegistry(t *testing.T) {
	dep := manifestWithDeps("dep", "1.0.0", "", nil)
	root := manifestWithDeps("root", "1.0.0", "", []artifact.Dependency{
		{Name: "dep", Version: "1.0.0", Registry: "other:5000/repo"},
	})
	fetcher := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"other:5000/repo/dep:1.0.0": dep,
	}}
	_, err := Resolve(context.Background(), root, "localhost:5000/skills", fetcher)
	if err != nil {
		t.Fatalf("Resolve() err = %v", err)
	}
	if len(fetcher.calls) != 1 || fetcher.calls[0] != "other:5000/repo/dep:1.0.0" {
		t.Errorf("fetcher.calls = %v", fetcher.calls)
	}
}

func TestResolve_FetcherError(t *testing.T) {
	root := manifestWithDeps("root", "1.0.0", "", []artifact.Dependency{{Name: "missing", Version: "1.0.0"}})
	fetcher := &mockFetcher{manifests: map[string]*artifact.Manifest{}}
	_, err := Resolve(context.Background(), root, "localhost:5000/skills", fetcher)
	if err == nil {
		t.Error("Resolve() err = nil, want error")
	}
}

func TestResolve_EmptyDefaultRegistryWithDeps(t *testing.T) {
	root := manifestWithDeps("root", "1.0.0", "", []artifact.Dependency{{Name: "dep", Version: "1.0.0"}})
	fetcher := &mockFetcher{}
	_, err := Resolve(context.Background(), root, "", fetcher)
	if err == nil {
		t.Error("Resolve() with empty defaultRegistry and deps: err = nil, want error")
	}
}

func TestResolve_NilRoot(t *testing.T) {
	fetcher := &mockFetcher{}
	_, err := Resolve(context.Background(), nil, "localhost:5000/skills", fetcher)
	if err == nil {
		t.Error("Resolve(nil root) err = nil, want error")
	}
}

func TestResolve_NilFetcher(t *testing.T) {
	root := manifestWithDeps("root", "1.0.0", "", []artifact.Dependency{{Name: "dep", Version: "1.0.0"}})
	_, err := Resolve(context.Background(), root, "localhost:5000/skills", nil)
	if err == nil {
		t.Error("Resolve(nil fetcher) err = nil, want error")
	}
}

func TestResolve_TrimTrailingSlash(t *testing.T) {
	dep := manifestWithDeps("dep", "1.0.0", "", nil)
	root := manifestWithDeps("root", "1.0.0", "", []artifact.Dependency{{Name: "dep", Version: "1.0.0"}})
	fetcher := &mockFetcher{manifests: map[string]*artifact.Manifest{
		"localhost:5000/skills/dep:1.0.0": dep,
	}}
	_, err := Resolve(context.Background(), root, "localhost:5000/skills/", fetcher)
	if err != nil {
		t.Fatalf("Resolve() err = %v", err)
	}
	if len(fetcher.calls) != 1 || fetcher.calls[0] != "localhost:5000/skills/dep:1.0.0" {
		t.Errorf("fetcher.calls = %v (trailing slash in registry should be trimmed)", fetcher.calls)
	}
}
