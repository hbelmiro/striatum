package artifact

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var (
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// IsValidDigest reports whether s matches the OCI content digest format (sha256:<64 lowercase hex chars>).
func IsValidDigest(s string) bool { return digestPattern.MatchString(s) }

// IsValidCommitSHA reports whether s is a valid full-length Git commit SHA (40 lowercase hex chars).
func IsValidCommitSHA(s string) bool { return commitPattern.MatchString(s) }

const SupportedAPIVersion = "striatum.dev/v1alpha2"

var supportedKinds = map[string]bool{
	"Memory":   true,
	"Prompt":   true,
	"Skill":    true,
	"Workflow": true,
}

// IsSupportedKind reports whether kind is a recognized artifact kind.
func IsSupportedKind(kind string) bool {
	return supportedKinds[kind]
}

// SupportedKinds returns a sorted slice of all supported artifact kinds.
func SupportedKinds() []string {
	kinds := make([]string, 0, len(supportedKinds))
	for k := range supportedKinds {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
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
	Digest       string `json:"digest,omitempty"`
}

func (d *OCIDependency) Source() string { return "oci" }

func (d *OCIDependency) CanonicalRef() string {
	s := d.RegistryHost + "/" + d.Repository + ":" + d.Tag
	if d.Digest != "" {
		s += "@" + d.Digest
	}
	return s
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
	if d.Digest != "" && !IsValidDigest(d.Digest) {
		return fmt.Errorf("oci dependency: digest must match sha256:<64 lowercase hex chars>, got %q", d.Digest)
	}
	return nil
}

func (d *OCIDependency) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Source     string `json:"source"`
		Registry   string `json:"registry"`
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
		Digest     string `json:"digest,omitempty"`
	}{
		Source:     "oci",
		Registry:   d.RegistryHost,
		Repository: d.Repository,
		Tag:        d.Tag,
		Digest:     d.Digest,
	})
}

// GitDependency is a dependency hosted in a Git repository.
type GitDependency struct {
	URL        string   `json:"url"`
	Ref        string   `json:"ref"`
	Path       string   `json:"path,omitempty"`
	Commit     string   `json:"commit,omitempty"`
	Files      []string `json:"files,omitempty"`
	Name       string   `json:"name,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Entrypoint string   `json:"entrypoint,omitempty"`
}

func (d *GitDependency) Source() string { return "git" }

func (d *GitDependency) CanonicalRef() string {
	s := "git:" + d.URL + "@" + d.Ref
	if d.Path != "" {
		s += "#" + d.Path
	}
	if d.Commit != "" {
		s += "!" + d.Commit
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
	if d.Commit != "" && !IsValidCommitSHA(d.Commit) {
		return fmt.Errorf("git dependency: commit must be a 40-character lowercase hex SHA, got %q", d.Commit)
	}
	if d.Files != nil {
		if err := validateGitFiles(d.Files); err != nil {
			return err
		}
	}
	if d.Name != "" {
		if err := validateGitName(d.Name); err != nil {
			return err
		}
	}
	if d.Kind != "" && !IsSupportedKind(d.Kind) {
		return fmt.Errorf("git dependency: unsupported kind %q; supported: %s", d.Kind, supportedKindsList())
	}
	if err := validateGitEntrypoint(d.Entrypoint, d.Files); err != nil {
		return err
	}
	return nil
}

func validateGitFiles(files []string) error {
	if len(files) == 0 {
		return errors.New("git dependency: files must not be an empty list (omit the field or provide entries)")
	}
	seen := make(map[string]bool, len(files))
	for _, f := range files {
		if f == "" {
			return errors.New("git dependency: files must not contain empty strings")
		}
		if strings.Contains(f, "..") {
			return fmt.Errorf("git dependency: files entry %q contains path traversal", f)
		}
		if filepath.IsAbs(f) {
			return fmt.Errorf("git dependency: files entry %q is an absolute path", f)
		}
		if seen[f] {
			return fmt.Errorf("git dependency: duplicate files entry %q", f)
		}
		seen[f] = true
	}
	return nil
}

func validateGitName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("git dependency: name must not be whitespace-only")
	}
	if name != strings.TrimSpace(name) {
		return errors.New("git dependency: name must not have leading or trailing whitespace")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("git dependency: name %q must not contain slashes", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("git dependency: name %q is not allowed", name)
	}
	return nil
}

func validateGitEntrypoint(entrypoint string, files []string) error {
	if entrypoint == "" {
		return nil
	}
	if strings.Contains(entrypoint, "..") {
		return fmt.Errorf("git dependency: entrypoint %q contains path traversal", entrypoint)
	}
	if filepath.IsAbs(entrypoint) {
		return fmt.Errorf("git dependency: entrypoint %q is an absolute path", entrypoint)
	}
	if files != nil && !slices.Contains(files, entrypoint) {
		return fmt.Errorf("git dependency: entrypoint %q must be listed in files", entrypoint)
	}
	return nil
}

func (d *GitDependency) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Source     string   `json:"source"`
		URL       string   `json:"url"`
		Ref       string   `json:"ref"`
		Path      string   `json:"path,omitempty"`
		Commit    string   `json:"commit,omitempty"`
		Files     []string `json:"files,omitempty"`
		Name      string   `json:"name,omitempty"`
		Kind      string   `json:"kind,omitempty"`
		Entrypoint string  `json:"entrypoint,omitempty"`
	}{
		Source:     "git",
		URL:        d.URL,
		Ref:        d.Ref,
		Path:       d.Path,
		Commit:     d.Commit,
		Files:      d.Files,
		Name:       d.Name,
		Kind:       d.Kind,
		Entrypoint: d.Entrypoint,
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
	if m.APIVersion != SupportedAPIVersion {
		return fmt.Errorf("unsupported apiVersion %q, want %s", m.APIVersion, SupportedAPIVersion)
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
