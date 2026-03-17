package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hbelmiro/striatum/pkg/artifact"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
)

// Layer annotation for file path (OCI image-spec convention).
const annotationTitle = "org.opencontainers.image.title"

// Pack builds an OCI image from the artifact manifest and writes it to the
// OCI Image Layout at layoutPath. The manifest must be valid; all spec.files
// must exist under baseDir.
func Pack(m *artifact.Manifest, baseDir string, layoutPath string) error {
	if m == nil {
		return errors.New("manifest is nil")
	}
	store, err := oci.New(layoutPath)
	if err != nil {
		return fmt.Errorf("create OCI store: %w", err)
	}
	tag := m.Metadata.Name + ":" + m.Metadata.Version
	return packToTarget(context.Background(), m, baseDir, store, tag)
}

// packToTarget pushes the artifact (config + layers + manifest) to target and tags it.
func packToTarget(ctx context.Context, m *artifact.Manifest, baseDir string, target oras.Target, tag string) error {
	if m == nil {
		return errors.New("manifest is nil")
	}
	if err := artifact.Validate(m); err != nil {
		return fmt.Errorf("validate manifest: %w", err)
	}
	if err := artifact.ValidateLocal(m, baseDir); err != nil {
		return err
	}

	configBytes, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	configDesc := content.NewDescriptorFromBytes(ConfigMediaType, configBytes)
	if err := target.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil {
		return fmt.Errorf("push config: %w", err)
	}

	var layers []ocispec.Descriptor
	for _, name := range m.Spec.Files {
		p := filepath.Join(baseDir, filepath.FromSlash(name))
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read file %q: %w", name, err)
		}
		layerDesc := content.NewDescriptorFromBytes(LayerMediaType, data)
		layerDesc.Annotations = map[string]string{annotationTitle: name}
		if err := target.Push(ctx, layerDesc, bytes.NewReader(data)); err != nil {
			return fmt.Errorf("push layer %q: %w", name, err)
		}
		layers = append(layers, layerDesc)
	}

	opts := oras.PackManifestOptions{
		ConfigDescriptor: &configDesc,
		Layers:           layers,
	}
	manifestDesc, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, ArtifactType, opts)
	if err != nil {
		return fmt.Errorf("pack manifest: %w", err)
	}

	if err := target.Tag(ctx, manifestDesc, tag); err != nil {
		return fmt.Errorf("tag manifest: %w", err)
	}

	return nil
}
