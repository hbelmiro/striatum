package oci

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	orasoci "oras.land/oras-go/v2/content/oci"
)

// PreparedImage is a fully staged self-contained payload ready to be packed into an
// OCI image: a config blob, an artifact type, and the files (slash paths)
// under Dir. It is produced by pkg/selfcontained.Prepare and consumed here so that
// pkg/oci does not depend on pkg/selfcontained.
type PreparedImage struct {
	Config              []byte
	ArtifactType        string
	Dir                 string
	Files               []string
	ManifestAnnotations map[string]string
}

// PackPreparedToLayout packs a prepared self-contained image into the OCI Image Layout at
// layoutPath, tagged with the given tag, and returns the manifest descriptor.
func PackPreparedToLayout(ctx context.Context, pi PreparedImage, layoutPath, tag string) (ocispec.Descriptor, error) {
	store, err := orasoci.New(layoutPath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create OCI store: %w", err)
	}
	return packPreparedToTarget(ctx, pi, store, tag)
}

// PushPrepared packs a prepared self-contained image and pushes it to the OCI registry at
// reference ("host/repo/name:tag"). OCI layout references are rejected: push
// is registry-only; use PackPreparedToLayout to write a local layout.
// Returns the manifest descriptor of the pushed image.
func PushPrepared(ctx context.Context, pi PreparedImage, reference string) (ocispec.Descriptor, error) {
	if strings.HasPrefix(reference, "oci:") {
		return ocispec.Descriptor{}, fmt.Errorf("push is registry-only: %q is an OCI layout reference; use \"striatum pack --self-contained -o <dir>\" to write a local layout", reference)
	}
	repo, tag, _, err := SplitReference(reference)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parse reference: %w", err)
	}
	if tag == "" {
		return ocispec.Descriptor{}, fmt.Errorf("push requires a tag: %q", reference)
	}
	mem := memory.New()
	if _, err := packPreparedToTarget(ctx, pi, mem, tag); err != nil {
		return ocispec.Descriptor{}, err
	}
	reg, err := NewRepository(repo)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create repository: %w", err)
	}
	pushed, err := oras.Copy(ctx, mem, tag, reg, tag, oras.DefaultCopyOptions)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push to registry: %w", err)
	}
	return pushed, nil
}

// packPreparedToTarget pushes the prepared payload (config + one layer per
// file with a title annotation) to target and tags it.
func packPreparedToTarget(ctx context.Context, pi PreparedImage, target oras.Target, tag string) (ocispec.Descriptor, error) {
	if len(pi.Config) == 0 {
		return ocispec.Descriptor{}, fmt.Errorf("prepared image has no config blob")
	}
	if len(pi.Files) == 0 {
		return ocispec.Descriptor{}, fmt.Errorf("prepared image has no files")
	}
	configDesc := content.NewDescriptorFromBytes(ConfigMediaType, pi.Config)
	if err := pushBlob(ctx, target, configDesc, newByteReader(pi.Config)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push config: %w", err)
	}

	var layers []ocispec.Descriptor
	for _, name := range pi.Files {
		if err := checkLayerPath(name); err != nil {
			return ocispec.Descriptor{}, err
		}
		data, err := os.ReadFile(filepath.Join(pi.Dir, filepath.FromSlash(name)))
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("read prepared file %q: %w", name, err)
		}
		layerDesc := content.NewDescriptorFromBytes(LayerMediaType, data)
		layerDesc.Annotations = map[string]string{annotationTitle: name}
		if err := pushBlob(ctx, target, layerDesc, newByteReader(data)); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("push layer %q: %w", name, err)
		}
		layers = append(layers, layerDesc)
	}

	opts := oras.PackManifestOptions{
		ConfigDescriptor:    &configDesc,
		Layers:              layers,
		ManifestAnnotations: pi.ManifestAnnotations,
	}
	manifestDesc, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, pi.ArtifactType, opts)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack manifest: %w", err)
	}
	if err := target.Tag(ctx, manifestDesc, tag); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("tag manifest: %w", err)
	}
	return manifestDesc, nil
}

// checkLayerPath rejects layer title paths that could escape the image tree.
func checkLayerPath(name string) error {
	if name == "" || path.IsAbs(name) || strings.Contains(name, "\\") {
		return fmt.Errorf("unsafe layer path %q", name)
	}
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") || clean != name {
		return fmt.Errorf("unsafe layer path %q", name)
	}
	return nil
}

func newByteReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
