package oci

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oras.land/oras-go/v2/registry/remote/auth"
)

func TestNewRepository_usesCredentialsFromDOCKER_CONFIG(t *testing.T) {
	const user, pass = "striatum-user", "striatum-secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || !strings.Contains(r.URL.Path, "/manifests/") {
			http.NotFound(w, r)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Content-Length", "512")
		w.Header().Set("Docker-Content-Digest", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	hostPort := u.Host

	dockerDir := t.TempDir()
	authB64 := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	type authEntry struct {
		Auth string `json:"auth"`
	}
	cfg := struct {
		Auths map[string]authEntry `json:"auths"`
	}{
		Auths: map[string]authEntry{
			hostPort: {Auth: authB64},
		},
	}
	configBytes, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dockerDir, "config.json"), configBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dockerDir)

	repoRef := hostPort + "/striatum/test-repo"
	reg, err := NewRepository(repoRef)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}

	_, err = reg.Resolve(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("Resolve with docker auth: %v", err)
	}
}

func TestNewRepository_unauthenticatedFailsWhenServerRequiresBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	hostPort := u.Host

	dockerDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dockerDir, "config.json"), []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dockerDir)

	reg, err := NewRepository(hostPort + "/x/y")
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	_, err = reg.Resolve(context.Background(), "tag")
	if err == nil {
		t.Fatal("Resolve: want error without credentials, got nil")
	}
}

func TestNewRepository_invalidDockerConfigReturnsError(t *testing.T) {
	dockerDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dockerDir, "config.json"), []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dockerDir)

	_, err := NewRepository("127.0.0.1:5000/some/repo")
	if err == nil {
		t.Fatal("NewRepository: want error for invalid docker config, got nil")
	}
}

func TestCredentialServerCandidates_includesDockerHubAliases(t *testing.T) {
	got := credentialServerCandidates("registry-1.docker.io")
	var hasDockerIO, hasIndexV1 bool
	for _, c := range got {
		switch c {
		case "docker.io":
			hasDockerIO = true
		case "https://index.docker.io/v1/":
			hasIndexV1 = true
		}
	}
	if !hasDockerIO || !hasIndexV1 {
		t.Fatalf("candidates = %v, want docker.io and https://index.docker.io/v1/", got)
	}
}

type stubCredStore struct {
	hit map[string]auth.Credential
}

func (s *stubCredStore) Get(_ context.Context, serverAddress string) (auth.Credential, error) {
	if c, ok := s.hit[serverAddress]; ok {
		return c, nil
	}
	return auth.EmptyCredential, nil
}

func (s *stubCredStore) Put(_ context.Context, _ string, _ auth.Credential) error { return nil }

func (s *stubCredStore) Delete(_ context.Context, _ string) error { return nil }

func TestLookupCredentials_triesAlternateDockerKeys(t *testing.T) {
	want := auth.Credential{Username: "u", Password: "p"}
	store := &stubCredStore{
		hit: map[string]auth.Credential{
			"https://127.0.0.1:9/v1/": want,
		},
	}
	got, err := lookupCredentials(context.Background(), store, "127.0.0.1:9")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("lookupCredentials = %+v, want %+v", got, want)
	}
}
