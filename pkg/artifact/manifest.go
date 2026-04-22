package artifact

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const supportedAPIVersion = "striatum.dev/v1alpha2"

var supportedKinds = map[string]bool{
	"Skill": true,
}

// IsSupportedKind reports whether kind is a recognized artifact kind.
func IsSupportedKind(kind string) bool {
	return supportedKinds[kind]
}

// SupportedKindsList returns a comma-separated list of supported artifact kinds (e.g. "Skill").
func SupportedKindsList() string {
	return supportedKindsList()
}

// Dependency is the interface all dependency types implement.
// Each backend (OCI, Git, ...) provides a concrete struct.
type Dependency interface {
	Source() string
	CanonicalRef() string
	Validate() error
}

var (
	_ Dependency = (*OCIDependency)(nil)
	_ Dependency = (*GitDependency)(nil)
)

// OCIDependency is a dependency hosted in an OCI registry.
type OCIDependency struct {
	RegistryHost string `json:"registry"`
	Repository   string `json:"repository"`
	Tag          string `json:"tag"`
}

func (d *OCIDependency) Source() string { return "oci" }

func (d *OCIDependency) CanonicalRef() string {
	return d.RegistryHost + "/" + d.Repository + ":" + d.Tag
}

func (d *OCIDependency) Validate() error {
	if strings.TrimSpace(d.RegistryHost) == "" {
		return errors.New("oci dependency: registry is required")
	}
	if strings.TrimSpace(d.Repository) == "" {
		return errors.New("oci dependency: repository is required")
	}
	if strings.TrimSpace(d.Tag) == "" {
		return errors.New("oci dependency: tag is required")
	}
	return nil
}

func (d *OCIDependency) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Source     string `json:"source"`
		Registry   string `json:"registry"`
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
	}{
		Source:     "oci",
		Registry:   d.RegistryHost,
		Repository: d.Repository,
		Tag:        d.Tag,
	})
}

// GitDependency is a dependency hosted in a Git repository.
type GitDependency struct {
	URL  string `json:"url"`
	Ref  string `json:"ref"`
	Path string `json:"path,omitempty"`
}

func (d *GitDependency) Source() string { return "git" }

func (d *GitDependency) CanonicalRef() string {
	s := "git:" + d.URL + "@" + d.Ref
	if d.Path != "" {
		s += "#" + d.Path
	}
	return s
}

func (d *GitDependency) Validate() error {
	if strings.TrimSpace(d.URL) == "" {
		return errors.New("git dependency: url is required")
	}
	if strings.TrimSpace(d.Ref) == "" {
		return errors.New("git dependency: ref is required")
	}
	return nil
}

func (d *GitDependency) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Source string `json:"source"`
		URL    string `json:"url"`
		Ref    string `json:"ref"`
		Path   string `json:"path,omitempty"`
	}{
		Source: "git",
		URL:    d.URL,
		Ref:    d.Ref,
		Path:   d.Path,
	})
}

// Manifest is the root type for artifact.json (v1alpha2).
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

func (m *Manifest) UnmarshalJSON(data []byte) error {
	var raw struct {
		APIVersion   string            `json:"apiVersion"`
		Kind         string            `json:"kind"`
		Metadata     Metadata          `json:"metadata"`
		Spec         Spec              `json:"spec"`
		Dependencies []json.RawMessage `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.APIVersion = raw.APIVersion
	m.Kind = raw.Kind
	m.Metadata = raw.Metadata
	m.Spec = raw.Spec
	m.Dependencies = nil

	for i, rd := range raw.Dependencies {
		dep, err := unmarshalDependency(rd, i)
		if err != nil {
			return err
		}
		m.Dependencies = append(m.Dependencies, dep)
	}
	return nil
}

func unmarshalDependency(data json.RawMessage, index int) (Dependency, error) {
	var probe struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("dependencies[%d]: %w", index, err)
	}
	switch probe.Source {
	case "oci":
		var d OCIDependency
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("dependencies[%d]: %w", index, err)
		}
		return &d, nil
	case "git":
		var d GitDependency
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("dependencies[%d]: %w", index, err)
		}
		return &d, nil
	case "":
		return nil, fmt.Errorf("dependencies[%d]: source is required", index)
	default:
		return nil, fmt.Errorf("dependencies[%d]: unsupported source %q", index, probe.Source)
	}
}

func (m *Manifest) MarshalJSON() ([]byte, error) {
	var rawDeps []json.RawMessage
	for _, d := range m.Dependencies {
		b, err := json.Marshal(d)
		if err != nil {
			return nil, err
		}
		rawDeps = append(rawDeps, b)
	}
	return json.Marshal(struct {
		APIVersion   string            `json:"apiVersion"`
		Kind         string            `json:"kind"`
		Metadata     Metadata          `json:"metadata"`
		Spec         Spec              `json:"spec"`
		Dependencies []json.RawMessage `json:"dependencies,omitempty"`
	}{
		APIVersion:   m.APIVersion,
		Kind:         m.Kind,
		Metadata:     m.Metadata,
		Spec:         m.Spec,
		Dependencies: rawDeps,
	})
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
	if !supportedKinds[m.Kind] {
		return fmt.Errorf("unsupported kind %q; supported: %s", m.Kind, supportedKindsList())
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
		if err := d.Validate(); err != nil {
			return fmt.Errorf("dependencies[%d]: %w", i, err)
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

func supportedKindsList() string {
	kinds := make([]string, 0, len(supportedKinds))
	for k := range supportedKinds {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return strings.Join(kinds, ", ")
}
