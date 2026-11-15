package selfcontained

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hbelmiro/striatum/pkg/artifact"
	"github.com/hbelmiro/striatum/pkg/resolver"
)

// MarkerKey is the top-level key added to the self-contained image's config
// blob (the artifact manifest JSON) to mark it as a Striatum export.
const MarkerKey = "dev.striatum.self-contained"

// MarkerValue is the current export marker value.
const MarkerValue = "v1"

// ProvenanceAPIVersion identifies the self-contained image provenance document format.
const ProvenanceAPIVersion = "striatum.dev/self-contained/v1"

// ProvenancePath is the image path of the provenance document.
const ProvenancePath = ReservedPrefix + "provenance.json"

// ResolvedSource records where a dependency was actually resolved from,
// preserving source pins and unpinned resolutions as provenance.
type ResolvedSource struct {
	Type   string `json:"type"` // "oci" or "git"
	Ref    string `json:"ref,omitempty"`
	Digest string `json:"digest,omitempty"`
	URL    string `json:"url,omitempty"`
	Commit string `json:"commit,omitempty"`
}

// DepProvenance records one bundled dependency: its declaration verbatim,
// the resolved source, and the files it occupies in the self-contained image.
type DepProvenance struct {
	Kind           string          `json:"kind"`
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	Declared       json.RawMessage `json:"declared"`
	ResolvedSource ResolvedSource  `json:"resolvedSource"`
	ImagePaths     []string        `json:"imagePaths"`
}

// RootProvenance records the root artifact and its original manifest.
type RootProvenance struct {
	Kind     string          `json:"kind"`
	Name     string          `json:"name"`
	Version  string          `json:"version"`
	Manifest json.RawMessage `json:"manifest"`
}

// Provenance is the self-contained image provenance document (striatum.dev/self-contained/v1),
// written to .striatum/provenance.json in the self-contained image.
// It deliberately contains no timestamps so identical inputs produce
// identical image digests.
type Provenance struct {
	APIVersion   string          `json:"apiVersion"`
	Root         RootProvenance  `json:"root"`
	Dependencies []DepProvenance `json:"dependencies"`
	Files        []string        `json:"files"`
}

// DepMaterialization is the result of materializing one dependency: the
// on-disk directory holding its declared files and its resolved source.
type DepMaterialization struct {
	CacheDir string
	Source   ResolvedSource
}

// DepMaterializer makes a resolved dependency's files available on disk
// (typically via the Striatum cache, honoring declared source pins).
type DepMaterializer func(ctx context.Context, r resolver.ResolvedArtifact) (DepMaterialization, error)

// Options configures Prepare.
type Options struct {
	Manifest    *artifact.Manifest
	ProjectRoot string
	Fetcher     resolver.DependencyFetcher
	// MaterializeDep provides dependency files for each resolved dependency.
	MaterializeDep DepMaterializer
	// StagingParent is the directory under which the staging directory is
	// created. When empty, the system temp dir is used.
	StagingParent string
	// Constraints bounds the self-contained payload; zero values use the
	// MLflow consumer defaults (the strictest audited consumer).
	Constraints MLflowOptions
}

// Prepared is a fully staged, validated export ready to be packed or pushed.
type Prepared struct {
	Dir         string
	Files       []string
	RootName    string
	RootVersion string
	Provenance  *Provenance
	// Cleanup removes the staging directory.
	Cleanup func()
}

// Prepare builds the self-contained image of a root Skill and its complete
// Skill/Prompt dependency closure: files are safely materialized into a
// staging directory, artifact-owned Markdown references are rewritten to
// resolve inside the tree, provenance is recorded, and the result is checked
// against the audited MLflow constraints. Source files and ordinary caches
// are never modified.
func Prepare(ctx context.Context, opts Options) (*Prepared, error) {
	m := opts.Manifest
	if m == nil {
		return nil, fmt.Errorf("manifest is nil")
	}
	if m.Kind != "Skill" {
		return nil, fmt.Errorf("export requires a Skill root artifact, got kind %q", m.Kind)
	}
	if err := artifact.Validate(m); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}

	resolved, err := resolveClosure(ctx, m, opts.Fetcher)
	if err != nil {
		return nil, err
	}
	for _, r := range resolved {
		if err := checkSafeNameVersion(r.Name, r.Version); err != nil {
			return nil, err
		}
	}

	parent := opts.StagingParent
	if parent == "" {
		parent = os.TempDir()
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create staging parent: %w", err)
	}
	staging, err := os.MkdirTemp(parent, ".export-staging-")
	if err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	fail := func(err error) (*Prepared, error) {
		_ = os.RemoveAll(staging)
		return nil, err
	}

	// Build the closure tree for link rewriting and compute destinations.
	type stagedArtifact struct {
		tree     TreeArtifact
		srcDir   string
		imageDir string // "" for root (files at image root)
	}
	var staged []stagedArtifact
	registry := NewDestRegistry()
	prov := &Provenance{
		APIVersion: ProvenanceAPIVersion,
		Root: RootProvenance{
			Kind:    m.Kind,
			Name:    m.Metadata.Name,
			Version: m.Metadata.Version,
		},
	}

	rootTree := TreeArtifact{Kind: m.Kind, Name: m.Metadata.Name, Files: normalizeFiles(m.Spec.Files), IsRoot: true}
	staged = append(staged, stagedArtifact{tree: rootTree, srcDir: opts.ProjectRoot})
	rootManifestJSON, err := json.Marshal(m)
	if err != nil {
		return fail(fmt.Errorf("marshal root manifest: %w", err))
	}
	prov.Root.Manifest = rootManifestJSON

	for _, r := range resolved[1:] {
		if r.Manifest == nil {
			return fail(fmt.Errorf("dependency %s@%s has no manifest", r.Name, r.Version))
		}
		kind := r.Manifest.Kind
		if kind != "Skill" && kind != "Prompt" {
			return fail(fmt.Errorf("dependency %s@%s has unsupported kind %q; export supports Skill and Prompt dependencies only", r.Name, r.Version, kind))
		}
		if opts.MaterializeDep == nil {
			return fail(fmt.Errorf("dependency %s@%s cannot be materialized: no materializer configured", r.Name, r.Version))
		}
		mat, err := opts.MaterializeDep(ctx, r)
		if err != nil {
			return fail(fmt.Errorf("materialize dependency %s@%s: %w", r.Name, r.Version, err))
		}
		kd, _ := kindDirFor(kind)
		// Defensive: the resolver dedups same name@version and rejects
		// same-name version conflicts, so two closure artifacts can never
		// share a virtual install root. Guard anyway: RewriteLinks is
		// exported, and a duplicate root would silently resolve every link
		// against the first (root) artifact.
		for _, prev := range staged {
			if prev.tree.virtualRoot() == path.Join(kd, r.Name) {
				return fail(fmt.Errorf("dependency %s@%s shares the install location of %s %s; artifact names must be unique per kind in the closure", r.Name, r.Version, prev.tree.Kind, prev.tree.Name))
			}
		}
		staged = append(staged, stagedArtifact{
			tree:     TreeArtifact{Kind: kind, Name: r.Name, Files: normalizeFiles(r.Manifest.Spec.Files)},
			srcDir:   mat.CacheDir,
			imageDir: path.Join("deps", kd, r.Name),
		})
		declared, err := json.Marshal(r.Dependency)
		if err != nil {
			return fail(fmt.Errorf("marshal dependency declaration for %s: %w", r.Name, err))
		}
		prov.Dependencies = append(prov.Dependencies, DepProvenance{
			Kind:           kind,
			Name:           r.Name,
			Version:        r.Version,
			Declared:       declared,
			ResolvedSource: mat.Source,
		})
	}

	// Materialize files. staged[1:] is in the same order as prov.Dependencies.
	var allFiles []string
	for i, sa := range staged {
		for _, f := range sa.tree.Files {
			data, err := ReadFileSafe(sa.srcDir, f)
			if err != nil {
				return fail(fmt.Errorf("read %s of %s: %w", f, sa.tree.Name, err))
			}
			dest := destOf(sa.imageDir, f)
			if err := registry.Add(dest); err != nil {
				return fail(err)
			}
			if err := writeStaged(staging, dest, data); err != nil {
				return fail(err)
			}
			allFiles = append(allFiles, dest)
			if i > 0 {
				prov.Dependencies[i-1].ImagePaths = append(prov.Dependencies[i-1].ImagePaths, dest)
			}
		}
	}

	// Rewrite links in staged Markdown files.
	var trees []TreeArtifact
	for _, sa := range staged {
		trees = append(trees, sa.tree)
	}
	for _, sa := range staged {
		for _, f := range sa.tree.Files {
			if !IsMarkdown(f) {
				continue
			}
			p := filepath.Join(staging, filepath.FromSlash(destOf(sa.imageDir, f)))
			data, err := os.ReadFile(p)
			if err != nil {
				return fail(fmt.Errorf("read staged %s: %w", f, err))
			}
			rewritten, err := RewriteLinks(data, sa.tree, f, trees)
			if err != nil {
				return fail(err)
			}
			if string(rewritten) != string(data) {
				if err := os.WriteFile(p, rewritten, 0o644); err != nil {
					return fail(fmt.Errorf("write staged %s: %w", f, err))
				}
			}
		}
	}

	// Provenance document.
	prov.Files = append(allFiles, ProvenancePath)
	sort.Strings(prov.Files)
	provJSON, err := json.MarshalIndent(prov, "", "  ")
	if err != nil {
		return fail(fmt.Errorf("marshal provenance: %w", err))
	}
	if err := writeStaged(staging, ProvenancePath, provJSON); err != nil {
		return fail(err)
	}

	files := append([]string(nil), prov.Files...)
	if err := CheckMLflowCompat(staging, files, opts.Constraints); err != nil {
		return fail(fmt.Errorf("MLflow compatibility: %w", err))
	}

	return &Prepared{
		Dir:         staging,
		Files:       files,
		RootName:    m.Metadata.Name,
		RootVersion: m.Metadata.Version,
		Provenance:  prov,
		Cleanup:     func() { _ = os.RemoveAll(staging) },
	}, nil
}

// normalizeFiles cleans declared spec.files paths so link resolution,
// staging, and provenance all agree ("./helpers/x.md" and "helpers/x.md"
// are the same file). Safety is enforced downstream: ReadFileSafe and
// DestRegistry reject paths that escape the artifact root.
func normalizeFiles(files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, path.Clean(f))
	}
	return out
}

func destOf(imageDir, f string) string {
	if imageDir == "" {
		return f
	}
	return path.Join(imageDir, f)
}

func resolveClosure(ctx context.Context, m *artifact.Manifest, fetcher resolver.DependencyFetcher) ([]resolver.ResolvedArtifact, error) {
	if len(m.Dependencies) == 0 {
		return []resolver.ResolvedArtifact{{
			Name:     m.Metadata.Name,
			Version:  m.Metadata.Version,
			Manifest: m,
		}}, nil
	}
	if fetcher == nil {
		return nil, fmt.Errorf("manifest declares %d dependencies but no fetcher is configured", len(m.Dependencies))
	}
	resolved, err := resolver.Resolve(ctx, m, fetcher)
	if err != nil {
		return nil, fmt.Errorf("resolve dependencies: %w", err)
	}
	return resolved, nil
}

func checkSafeNameVersion(name, version string) error {
	for _, v := range []string{name, version} {
		if strings.TrimSpace(v) == "" || v == "." || strings.ContainsAny(v, "/\\") || strings.Contains(v, "..") {
			return fmt.Errorf("unsafe artifact name or version %q / %q: must be non-empty, must not be '.', and must not contain path separators or '..'", name, version)
		}
	}
	return nil
}

func writeStaged(staging, dest string, data []byte) error {
	p := filepath.Join(staging, filepath.FromSlash(dest))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create dir for %q: %w", dest, err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", dest, err)
	}
	return nil
}

// sourceFromDependency derives a ResolvedSource from a dependency
// declaration, preserving declared pins (OCI digest, git commit).
func sourceFromDependency(dep artifact.Dependency) ResolvedSource {
	switch d := dep.(type) {
	case *artifact.OCIDependency:
		return ResolvedSource{
			Type:   "oci",
			Ref:    d.RegistryHost + "/" + d.Repository + ":" + d.Tag,
			Digest: d.Digest,
		}
	case *artifact.GitDependency:
		return ResolvedSource{
			Type:   "git",
			URL:    d.URL,
			Ref:    d.Ref,
			Commit: d.Commit,
		}
	default:
		return ResolvedSource{Type: "unknown"}
	}
}

// ConfigWithMarker returns the artifact manifest JSON with the self-contained image marker
// added as a top-level key. Unknown fields are ignored by older readers, so
// ordinary Striatum versions can still parse the config blob.
func ConfigWithMarker(m *artifact.Manifest) ([]byte, error) {
	base, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(base, &generic); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	generic[MarkerKey] = json.RawMessage(`"` + MarkerValue + `"`)
	out, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return out, nil
}

// IsMarked reports whether a config blob (artifact manifest JSON)
// carries the Striatum export marker.
func IsMarked(configBytes []byte) bool {
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(configBytes, &generic); err != nil {
		return false
	}
	v, ok := generic[MarkerKey]
	if !ok {
		return false
	}
	return strings.TrimSpace(string(v)) == `"`+MarkerValue+`"`
}
