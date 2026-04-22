// Package registry provides a pluggable backend abstraction for artifact
// storage. CLI commands and the resolver dispatch through a Router that
// selects the right backend (OCI, Git, local OCI layout) based on locator type.
package registry

import (
	"context"
	"fmt"

	"github.com/hbelmiro/striatum/pkg/artifact"
)

// Locator identifies where an artifact lives.
// artifact.OCIDependency and artifact.GitDependency satisfy this interface.
type Locator interface {
	Source() string
	CanonicalRef() string
}

// OCILayoutLocator references an artifact in a local OCI Image Layout directory.
type OCILayoutLocator struct {
	LayoutPath string
	Tag        string
}

func (l *OCILayoutLocator) Source() string       { return "oci-layout" }
func (l *OCILayoutLocator) CanonicalRef() string { return "oci:" + l.LayoutPath + ":" + l.Tag }

// Registry provides Inspect and Pull operations dispatched by locator type.
type Registry interface {
	Inspect(ctx context.Context, loc Locator) (*artifact.Manifest, error)
	Pull(ctx context.Context, loc Locator, outputDir string) error
}

// OCIBackend handles remote OCI registry operations.
type OCIBackend interface {
	Inspect(ctx context.Context, dep *artifact.OCIDependency) (*artifact.Manifest, error)
	Pull(ctx context.Context, dep *artifact.OCIDependency, outputDir string) error
}

// GitBackend handles Git repository operations (read-only: inspect + pull).
type GitBackend interface {
	Inspect(ctx context.Context, dep *artifact.GitDependency) (*artifact.Manifest, error)
	Pull(ctx context.Context, dep *artifact.GitDependency, outputDir string) error
}

// OCILayoutBackend handles local OCI Image Layout operations.
type OCILayoutBackend interface {
	Inspect(ctx context.Context, loc *OCILayoutLocator) (*artifact.Manifest, error)
	Pull(ctx context.Context, loc *OCILayoutLocator, outputDir string) error
}

// Router dispatches Inspect and Pull to the appropriate backend.
type Router struct {
	OCI       OCIBackend
	Git       GitBackend
	OCILayout OCILayoutBackend
}

var _ Registry = (*Router)(nil)

func (r *Router) Inspect(ctx context.Context, loc Locator) (*artifact.Manifest, error) {
	switch l := loc.(type) {
	case *artifact.OCIDependency:
		if r.OCI == nil {
			return nil, fmt.Errorf("OCI backend not configured")
		}
		return r.OCI.Inspect(ctx, l)
	case *artifact.GitDependency:
		if r.Git == nil {
			return nil, fmt.Errorf("git backend not configured")
		}
		return r.Git.Inspect(ctx, l)
	case *OCILayoutLocator:
		if r.OCILayout == nil {
			return nil, fmt.Errorf("OCI layout backend not configured")
		}
		return r.OCILayout.Inspect(ctx, l)
	default:
		return nil, fmt.Errorf("unsupported locator type: %T", loc)
	}
}

func (r *Router) Pull(ctx context.Context, loc Locator, outputDir string) error {
	switch l := loc.(type) {
	case *artifact.OCIDependency:
		if r.OCI == nil {
			return fmt.Errorf("OCI backend not configured")
		}
		return r.OCI.Pull(ctx, l, outputDir)
	case *artifact.GitDependency:
		if r.Git == nil {
			return fmt.Errorf("git backend not configured")
		}
		return r.Git.Pull(ctx, l, outputDir)
	case *OCILayoutLocator:
		if r.OCILayout == nil {
			return fmt.Errorf("OCI layout backend not configured")
		}
		return r.OCILayout.Pull(ctx, l, outputDir)
	default:
		return fmt.Errorf("unsupported locator type: %T", loc)
	}
}
