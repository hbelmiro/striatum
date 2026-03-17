package artifact

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const supportedAPIVersion = "striatum.dev/v1alpha1"
const supportedKindSkill = "Skill"

// Manifest is the root type for artifact.json.
type Manifest struct {
	APIVersion   string       `json:"apiVersion"`
	Kind         string       `json:"kind"`
	Metadata     Metadata     `json:"metadata"`
	Spec         Spec         `json:"spec"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
}

// Metadata holds artifact identity and optional metadata.
type Metadata struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	License     string   `json:"license,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// Spec holds the artifact content spec (entrypoint and file list).
type Spec struct {
	Entrypoint string   `json:"entrypoint"`
	Files      []string `json:"files"`
}

// Dependency describes a dependency artifact.
type Dependency struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Registry string `json:"registry,omitempty"`
}

// Load reads and parses an artifact.json file.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// Validate checks schema correctness (required fields, apiVersion, kind, entrypoint in files, dependencies).
func Validate(m *Manifest) error {
	if m == nil {
		return errors.New("manifest is nil")
	}
	if m.APIVersion != supportedAPIVersion {
		return fmt.Errorf("unsupported apiVersion %q, want %s", m.APIVersion, supportedAPIVersion)
	}
	if m.Kind != supportedKindSkill {
		return fmt.Errorf("unsupported kind %q, want %s", m.Kind, supportedKindSkill)
	}
	if strings.TrimSpace(m.Metadata.Name) == "" {
		return errors.New("metadata.name is required and must be non-empty")
	}
	if strings.TrimSpace(m.Metadata.Version) == "" {
		return errors.New("metadata.version is required and must be non-empty")
	}
	if strings.TrimSpace(m.Spec.Entrypoint) == "" {
		return errors.New("spec.entrypoint is required and must be non-empty")
	}
	if len(m.Spec.Files) == 0 {
		return errors.New("spec.files is required and must contain at least one file")
	}
	fileSet := make(map[string]bool)
	for _, f := range m.Spec.Files {
		fileSet[f] = true
	}
	if !fileSet[m.Spec.Entrypoint] {
		return fmt.Errorf("spec.entrypoint %q must be listed in spec.files", m.Spec.Entrypoint)
	}
	for i, d := range m.Dependencies {
		if strings.TrimSpace(d.Name) == "" {
			return fmt.Errorf("dependencies[%d].name is required and must be non-empty", i)
		}
		if strings.TrimSpace(d.Version) == "" {
			return fmt.Errorf("dependencies[%d].version is required and must be non-empty", i)
		}
	}
	return nil
}

// ValidateLocal checks that all spec.files exist under baseDir.
func ValidateLocal(m *Manifest, baseDir string) error {
	if m == nil {
		return errors.New("manifest is nil")
	}
	for _, f := range m.Spec.Files {
		if f == "" || strings.Contains(f, "..") || filepath.IsAbs(f) {
			return fmt.Errorf("invalid file path in spec.files: %q", f)
		}
		p := filepath.Join(baseDir, filepath.FromSlash(f))
		if _, err := os.Stat(p); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file %q not found (spec.files)", f)
			}
			return fmt.Errorf("file %q: %w", f, err)
		}
	}
	return nil
}
