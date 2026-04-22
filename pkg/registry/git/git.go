// Package git implements the Git registry backend for Striatum.
// It supports Inspect (read manifest) and Pull (clone files) from Git repos.
package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/registry"
)

var _ registry.GitBackend = (*Backend)(nil)

// Backend implements registry.GitBackend using go-git.
type Backend struct{}

func (b *Backend) Inspect(ctx context.Context, dep *artifact.GitDependency) (*artifact.Manifest, error) {
	var m *artifact.Manifest
	err := withTree(ctx, dep, func(tree *object.Tree) error {
		var readErr error
		m, readErr = readManifestFromTree(tree, dep.Path)
		return readErr
	})
	return m, err
}

func (b *Backend) Pull(ctx context.Context, dep *artifact.GitDependency, outputDir string) error {
	return withTree(ctx, dep, func(tree *object.Tree) error {
		m, err := readManifestFromTree(tree, dep.Path)
		if err != nil {
			return err
		}

		if err := validateFilePaths(m.Spec.Files); err != nil {
			return err
		}

		destDir := filepath.Join(outputDir, m.Metadata.Name)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}

		base := dep.Path
		for _, f := range m.Spec.Files {
			srcPath := f
			if base != "" {
				srcPath = base + "/" + f
			}
			if err := extractFile(tree, srcPath, filepath.Join(destDir, f)); err != nil {
				return fmt.Errorf("extract %s: %w", f, err)
			}
		}

		manifestSrc := "artifact.json"
		if base != "" {
			manifestSrc = base + "/" + manifestSrc
		}
		return extractFile(tree, manifestSrc, filepath.Join(destDir, "artifact.json"))
	})
}

// withTree clones the repo, resolves the ref, and calls fn with the resulting tree.
// The cloned repo is cleaned up after fn returns.
func withTree(ctx context.Context, dep *artifact.GitDependency, fn func(*object.Tree) error) error {
	tmpDir, err := os.MkdirTemp("", "striatum-git-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	repo, err := gogit.PlainCloneContext(ctx, tmpDir, false, &gogit.CloneOptions{
		URL: dep.URL,
	})
	if err != nil {
		return fmt.Errorf("clone %s: %w", dep.URL, err)
	}

	hash, err := repo.ResolveRevision(plumbing.Revision(dep.Ref))
	if err != nil {
		return fmt.Errorf("resolve ref %q in %s: %w", dep.Ref, dep.URL, err)
	}

	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return fmt.Errorf("read commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("read tree: %w", err)
	}

	return fn(tree)
}

func readManifestFromTree(tree *object.Tree, basePath string) (*artifact.Manifest, error) {
	manifestPath := "artifact.json"
	if basePath != "" {
		manifestPath = basePath + "/" + manifestPath
	}
	f, err := tree.File(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("artifact.json not found at %q: %w", manifestPath, err)
	}
	reader, err := f.Reader()
	if err != nil {
		return nil, fmt.Errorf("read artifact.json: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read artifact.json: %w", err)
	}
	var m artifact.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse artifact.json: %w", err)
	}
	return &m, nil
}

// validateFilePaths rejects paths that would escape the destination directory.
func validateFilePaths(files []string) error {
	for _, f := range files {
		cleaned := filepath.Clean(filepath.FromSlash(f))
		if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
			return fmt.Errorf("unsafe file path in spec.files: %q", f)
		}
	}
	return nil
}

func extractFile(tree *object.Tree, srcPath, destPath string) error {
	f, err := tree.File(srcPath)
	if err != nil {
		return fmt.Errorf("file %q not found in tree: %w", srcPath, err)
	}
	reader, err := f.Reader()
	if err != nil {
		return fmt.Errorf("open reader for %q: %w", srcPath, err)
	}
	defer func() {
		_ = reader.Close()
	}()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create parent dirs for %q: %w", destPath, err)
	}
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create %q: %w", destPath, err)
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err = io.Copy(out, reader); err != nil {
		return fmt.Errorf("write %q: %w", destPath, err)
	}
	return nil
}
