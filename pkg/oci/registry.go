package oci

import (
	"strings"

	"oras.land/oras-go/v2/registry/remote"
)

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
	return reg, nil
}
