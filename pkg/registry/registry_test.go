package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// --- Interface conformance ---

var (
	_ Locator = (*artifact.OCIDependency)(nil)
	_ Locator = (*artifact.GitDependency)(nil)
	_ Locator = (*OCILayoutLocator)(nil)

	_ Registry   = (*Router)(nil)
	_ OCIBackend = (*OCIRemoteBackend)(nil)
)

// --- OCILayoutLocator ---

func TestOCILayoutLocator_Source(t *testing.T) {
	l := &OCILayoutLocator{LayoutPath: "./build", Tag: "my-skill:1.0.0"}
	if got := l.Source(); got != "oci-layout" {
		t.Errorf("Source() = %q, want %q", got, "oci-layout")
	}
}

func TestOCILayoutLocator_CanonicalRef(t *testing.T) {
	l := &OCILayoutLocator{LayoutPath: "./build", Tag: "1.0.0"}
	want := "oci:./build:1.0.0"
	if got := l.CanonicalRef(); got != want {
		t.Errorf("CanonicalRef() = %q, want %q", got, want)
	}
}

// --- ParseReference ---

func TestParseReference_RemoteOCI(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantHost string
		wantRepo string
		wantTag  string
	}{
		{"basic", "localhost:5000/skills/my-skill:1.0.0", "localhost:5000", "skills/my-skill", "1.0.0"},
		{"quay", "quay.io/org/skill:v2.1.0", "quay.io", "org/skill", "v2.1.0"},
		{"deep path", "reg.io/a/b/c:latest", "reg.io", "a/b/c", "latest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := ParseReference(tt.ref)
			if err != nil {
				t.Fatalf("ParseReference(%q) err = %v", tt.ref, err)
			}
			dep, ok := loc.(*artifact.OCIDependency)
			if !ok {
				t.Fatalf("type = %T, want *artifact.OCIDependency", loc)
			}
			if dep.RegistryHost != tt.wantHost {
				t.Errorf("RegistryHost = %q, want %q", dep.RegistryHost, tt.wantHost)
			}
			if dep.Repository != tt.wantRepo {
				t.Errorf("Repository = %q, want %q", dep.Repository, tt.wantRepo)
			}
			if dep.Tag != tt.wantTag {
				t.Errorf("Tag = %q, want %q", dep.Tag, tt.wantTag)
			}
		})
	}
}

func TestParseReference_OCILayout(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantPath string
		wantTag  string
	}{
		{"basic", "oci:./build:1.0.0", "./build", "1.0.0"},
		{"absolute", "oci:/tmp/layout:latest", "/tmp/layout", "latest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := ParseReference(tt.ref)
			if err != nil {
				t.Fatalf("ParseReference(%q) err = %v", tt.ref, err)
			}
			layout, ok := loc.(*OCILayoutLocator)
			if !ok {
				t.Fatalf("type = %T, want *OCILayoutLocator", loc)
			}
			if layout.LayoutPath != tt.wantPath {
				t.Errorf("LayoutPath = %q, want %q", layout.LayoutPath, tt.wantPath)
			}
			if layout.Tag != tt.wantTag {
				t.Errorf("Tag = %q, want %q", layout.Tag, tt.wantTag)
			}
		})
	}
}

func TestParseReference_Git(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantURL  string
		wantRef  string
		wantPath string
	}{
		{
			name:    "https",
			ref:     "git:https://github.com/org/repo.git@v1.0.0",
			wantURL: "https://github.com/org/repo.git", wantRef: "v1.0.0",
		},
		{
			name:    "ssh",
			ref:     "git:git@github.com:org/repo.git@v2.0.0",
			wantURL: "git@github.com:org/repo.git", wantRef: "v2.0.0",
		},
		{
			name:    "with path",
			ref:     "git:https://github.com/org/mono.git@main#packages/skill",
			wantURL: "https://github.com/org/mono.git", wantRef: "main",
			wantPath: "packages/skill",
		},
		{
			name:    "commit sha",
			ref:     "git:https://github.com/org/repo.git@abc123def",
			wantURL: "https://github.com/org/repo.git", wantRef: "abc123def",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := ParseReference(tt.ref)
			if err != nil {
				t.Fatalf("ParseReference(%q) err = %v", tt.ref, err)
			}
			dep, ok := loc.(*artifact.GitDependency)
			if !ok {
				t.Fatalf("type = %T, want *artifact.GitDependency", loc)
			}
			if dep.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", dep.URL, tt.wantURL)
			}
			if dep.Ref != tt.wantRef {
				t.Errorf("Ref = %q, want %q", dep.Ref, tt.wantRef)
			}
			if dep.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", dep.Path, tt.wantPath)
			}
		})
	}
}

func TestParseReference_OCILayout_WindowsDriveLetter(t *testing.T) {
	loc, err := ParseReference(`oci:C:\Users\me\build:1.0.0`)
	if err != nil {
		t.Fatalf("ParseReference() err = %v", err)
	}
	layout, ok := loc.(*OCILayoutLocator)
	if !ok {
		t.Fatalf("type = %T, want *OCILayoutLocator", loc)
	}
	if layout.LayoutPath != `C:\Users\me\build` {
		t.Errorf("LayoutPath = %q, want %q", layout.LayoutPath, `C:\Users\me\build`)
	}
	if layout.Tag != "1.0.0" {
		t.Errorf("Tag = %q, want %q", layout.Tag, "1.0.0")
	}
}

func TestParseReference_Errors(t *testing.T) {
	tests := []struct {
		name   string
		ref    string
		errMsg string
	}{
		{"empty", "", "empty reference"},
		{"whitespace", "  ", "empty reference"},
		{"no tag", "localhost:5000/repo", "missing tag"},
		{"no repo path", "my-skill:1.0.0", "missing repository path"},
		{"git no at", "git:https://example.com/repo.git", "missing @ref"},
		{"git empty url", "git:@v1.0.0", "empty URL"},
		{"git empty ref", "git:https://example.com/repo.git@", "empty ref"},
		{"oci no tag", "oci:./build", "missing tag"},
		{"oci empty path", "oci::tag", "empty path"},
		{"oci empty tag", "oci:./build:", "empty tag"},
		{"empty repo (trailing slash)", "host/:1.0.0", "empty host or repository"},
		{"empty host (leading slash)", "/repo:1.0.0", "empty host or repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseReference(tt.ref)
			if err == nil {
				t.Fatalf("ParseReference(%q) err = nil, want error", tt.ref)
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

// --- Router ---

type mockOCIBackend struct {
	inspectFn func(ctx context.Context, dep *artifact.OCIDependency) (*artifact.Manifest, error)
	pullFn    func(ctx context.Context, dep *artifact.OCIDependency, outputDir string) error
}

func (m *mockOCIBackend) Inspect(ctx context.Context, dep *artifact.OCIDependency) (*artifact.Manifest, error) {
	return m.inspectFn(ctx, dep)
}
func (m *mockOCIBackend) Pull(ctx context.Context, dep *artifact.OCIDependency, outputDir string) error {
	return m.pullFn(ctx, dep, outputDir)
}

type mockGitBackend struct {
	inspectFn func(ctx context.Context, dep *artifact.GitDependency) (*artifact.Manifest, error)
	pullFn    func(ctx context.Context, dep *artifact.GitDependency, outputDir string) error
}

func (m *mockGitBackend) Inspect(ctx context.Context, dep *artifact.GitDependency) (*artifact.Manifest, error) {
	return m.inspectFn(ctx, dep)
}
func (m *mockGitBackend) Pull(ctx context.Context, dep *artifact.GitDependency, outputDir string) error {
	return m.pullFn(ctx, dep, outputDir)
}

type mockOCILayoutBackend struct {
	inspectFn func(ctx context.Context, loc *OCILayoutLocator) (*artifact.Manifest, error)
	pullFn    func(ctx context.Context, loc *OCILayoutLocator, outputDir string) error
}

func (m *mockOCILayoutBackend) Inspect(ctx context.Context, loc *OCILayoutLocator) (*artifact.Manifest, error) {
	return m.inspectFn(ctx, loc)
}
func (m *mockOCILayoutBackend) Pull(ctx context.Context, loc *OCILayoutLocator, outputDir string) error {
	return m.pullFn(ctx, loc, outputDir)
}

func TestRouter_Inspect_OCI(t *testing.T) {
	want := &artifact.Manifest{Metadata: artifact.Metadata{Name: "oci-skill"}}
	r := &Router{OCI: &mockOCIBackend{
		inspectFn: func(_ context.Context, dep *artifact.OCIDependency) (*artifact.Manifest, error) {
			if dep.RegistryHost != "reg" || dep.Repository != "repo" || dep.Tag != "v1" {
				t.Errorf("dep = %+v", dep)
			}
			return want, nil
		},
	}}
	got, err := r.Inspect(context.Background(), &artifact.OCIDependency{RegistryHost: "reg", Repository: "repo", Tag: "v1"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Metadata.Name != want.Metadata.Name {
		t.Errorf("got name = %q", got.Metadata.Name)
	}
}

func TestRouter_Inspect_Git(t *testing.T) {
	want := &artifact.Manifest{Metadata: artifact.Metadata{Name: "git-skill"}}
	r := &Router{Git: &mockGitBackend{
		inspectFn: func(_ context.Context, dep *artifact.GitDependency) (*artifact.Manifest, error) {
			if dep.URL != "https://example.com/r.git" || dep.Ref != "v1" {
				t.Errorf("dep = %+v", dep)
			}
			return want, nil
		},
	}}
	got, err := r.Inspect(context.Background(), &artifact.GitDependency{URL: "https://example.com/r.git", Ref: "v1"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Metadata.Name != want.Metadata.Name {
		t.Errorf("got name = %q", got.Metadata.Name)
	}
}

func TestRouter_Inspect_OCILayout(t *testing.T) {
	want := &artifact.Manifest{Metadata: artifact.Metadata{Name: "layout-skill"}}
	r := &Router{OCILayout: &mockOCILayoutBackend{
		inspectFn: func(_ context.Context, loc *OCILayoutLocator) (*artifact.Manifest, error) {
			if loc.LayoutPath != "./build" || loc.Tag != "1.0.0" {
				t.Errorf("loc = %+v", loc)
			}
			return want, nil
		},
	}}
	got, err := r.Inspect(context.Background(), &OCILayoutLocator{LayoutPath: "./build", Tag: "1.0.0"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Metadata.Name != want.Metadata.Name {
		t.Errorf("got name = %q", got.Metadata.Name)
	}
}

func TestRouter_Inspect_NilBackend(t *testing.T) {
	r := &Router{}
	_, err := r.Inspect(context.Background(), &artifact.OCIDependency{RegistryHost: "reg", Repository: "repo", Tag: "v1"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("err = %v, want 'not configured'", err)
	}
}

type unsupportedLocator struct{}

func (u *unsupportedLocator) Source() string       { return "zip" }
func (u *unsupportedLocator) CanonicalRef() string { return "zip://a" }

func TestRouter_Inspect_UnsupportedLocator(t *testing.T) {
	r := &Router{}
	_, err := r.Inspect(context.Background(), &unsupportedLocator{})
	if err == nil || !strings.Contains(err.Error(), "unsupported locator") {
		t.Errorf("err = %v, want 'unsupported locator'", err)
	}
}

func TestRouter_Pull_OCI(t *testing.T) {
	var gotDir string
	r := &Router{OCI: &mockOCIBackend{
		pullFn: func(_ context.Context, dep *artifact.OCIDependency, outputDir string) error {
			gotDir = outputDir
			return nil
		},
	}}
	err := r.Pull(context.Background(), &artifact.OCIDependency{RegistryHost: "reg", Repository: "repo", Tag: "v1"}, "/tmp/out")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotDir != "/tmp/out" {
		t.Errorf("outputDir = %q, want /tmp/out", gotDir)
	}
}

func TestRouter_Pull_Git(t *testing.T) {
	called := false
	r := &Router{Git: &mockGitBackend{
		pullFn: func(_ context.Context, dep *artifact.GitDependency, _ string) error {
			called = true
			return nil
		},
	}}
	err := r.Pull(context.Background(), &artifact.GitDependency{URL: "u", Ref: "r"}, "/out")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !called {
		t.Error("Git backend was not called")
	}
}

func TestRouter_Pull_OCILayout(t *testing.T) {
	var gotDir string
	r := &Router{OCILayout: &mockOCILayoutBackend{
		pullFn: func(_ context.Context, loc *OCILayoutLocator, outputDir string) error {
			gotDir = outputDir
			if loc.LayoutPath != "./build" || loc.Tag != "1.0.0" {
				t.Errorf("loc = %+v", loc)
			}
			return nil
		},
	}}
	err := r.Pull(context.Background(), &OCILayoutLocator{LayoutPath: "./build", Tag: "1.0.0"}, "/tmp/out")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotDir != "/tmp/out" {
		t.Errorf("outputDir = %q, want /tmp/out", gotDir)
	}
}

func TestRouter_Inspect_NilGitBackend(t *testing.T) {
	r := &Router{}
	_, err := r.Inspect(context.Background(), &artifact.GitDependency{URL: "u", Ref: "r"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("err = %v, want 'not configured'", err)
	}
}

func TestRouter_Inspect_NilOCILayoutBackend(t *testing.T) {
	r := &Router{}
	_, err := r.Inspect(context.Background(), &OCILayoutLocator{LayoutPath: "p", Tag: "t"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("err = %v, want 'not configured'", err)
	}
}

func TestRouter_Pull_NilOCIBackend(t *testing.T) {
	r := &Router{}
	err := r.Pull(context.Background(), &artifact.OCIDependency{RegistryHost: "r", Repository: "p", Tag: "t"}, "/out")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("err = %v, want 'not configured'", err)
	}
}

func TestRouter_Pull_NilGitBackend(t *testing.T) {
	r := &Router{}
	err := r.Pull(context.Background(), &artifact.GitDependency{URL: "u", Ref: "r"}, "/out")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("err = %v, want 'not configured'", err)
	}
}

func TestRouter_Pull_NilOCILayoutBackend(t *testing.T) {
	r := &Router{}
	err := r.Pull(context.Background(), &OCILayoutLocator{LayoutPath: "p", Tag: "t"}, "/out")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("err = %v, want 'not configured'", err)
	}
}

func TestRouter_Pull_UnsupportedLocator(t *testing.T) {
	r := &Router{}
	err := r.Pull(context.Background(), &unsupportedLocator{}, "/out")
	if err == nil || !strings.Contains(err.Error(), "unsupported locator") {
		t.Errorf("err = %v, want 'unsupported locator'", err)
	}
}
