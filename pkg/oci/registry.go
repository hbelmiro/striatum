package oci

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// registryCredentialStore returns the credential store used for remote registry
// authentication. Today this is Docker-style config ($DOCKER_CONFIG or ~/.docker/config.json).
// Extend here for Podman (e.g. REGISTRY_AUTH_FILE) or composite stores.
func registryCredentialStore() (credentials.Store, error) {
	return credentials.NewStoreFromDocker(credentials.StoreOptions{})
}

// bareHost returns the hostname part of hostport (e.g. "reg:5000" -> "reg").
// If hostport has no port, it is returned unchanged.
func bareHost(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

// credentialServerCandidates lists Docker config / credential-helper lookup keys to try
// for the host ORAS passes (from the HTTP Host), e.g. "quay.io". Docker and registries
// may store logins under https URL forms or only match credHelpers for those forms;
// native helpers also key off the server URL passed on stdin.
func credentialServerCandidates(hostport string) []string {
	hp := strings.TrimSpace(hostport)
	if hp == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	add(hp)
	if bare := bareHost(hp); bare != hp {
		add(bare)
	}

	switch bareHost(hp) {
	case "registry-1.docker.io":
		add("docker.io")
		add("https://index.docker.io/v1/")
	}

	add("https://" + hp + "/v1/")
	add("https://" + bareHost(hp) + "/v1/")
	add("https://" + hp + "/")
	add("https://" + bareHost(hp) + "/")

	return out
}

func lookupCredentials(ctx context.Context, store credentials.Store, hostport string) (auth.Credential, error) {
	for _, addr := range credentialServerCandidates(hostport) {
		cred, err := store.Get(ctx, addr)
		if err != nil {
			return auth.Credential{}, err
		}
		if cred != auth.EmptyCredential {
			return cred, nil
		}
	}
	return auth.EmptyCredential, nil
}

// newRegistryAuthClient uses registryCredentialStore for each client so DOCKER_CONFIG
// and store init errors are not pinned across calls (tests, config refresh).
func newRegistryAuthClient() (*auth.Client, error) {
	store, err := registryCredentialStore()
	if err != nil {
		return nil, err
	}
	var hdr http.Header
	if auth.DefaultClient.Header != nil {
		hdr = auth.DefaultClient.Header.Clone()
	}
	if hdr == nil {
		hdr = http.Header{}
	}
	return &auth.Client{
		Client: retry.DefaultClient,
		Header: hdr,
		Cache:  auth.NewCache(),
		Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
			return lookupCredentials(ctx, store, hostport)
		},
	}, nil
}

// NewRepository creates a remote.Repository, automatically enabling PlainHTTP
// for localhost registries (localhost, 127.0.0.1, ::1).
func NewRepository(repo string) (*remote.Repository, error) {
	reg, err := remote.NewRepository(repo)
	if err != nil {
		return nil, err
	}
	host := strings.Split(reg.Reference.Host(), ":")[0]
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		reg.PlainHTTP = true
	}

	authClient, err := newRegistryAuthClient()
	if err != nil {
		return nil, fmt.Errorf("registry credentials: %w", err)
	}
	reg.Client = authClient
	return reg, nil
}
