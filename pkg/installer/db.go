package installer

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const installedFileName = "installed.yaml"

// InstalledEntry is one row in the install tracking database.
type InstalledEntry struct {
	Skill         string `yaml:"skill"`
	Version       string `yaml:"version"`
	Registry      string `yaml:"registry"`
	Target        string `yaml:"target"`
	ProjectPath   string `yaml:"-"`                        // NOT serialized; populated from YAML structure
	InstalledWith string `yaml:"installed_with,omitempty"` // empty = root install
	Status        string `yaml:"status"`
	LastError     string `yaml:"last_error,omitempty"`
	UpdatedAt     string `yaml:"updated_at"`
}

// installedFile is the on-disk format with explicit scope sections.
type installedFile struct {
	Global   []InstalledEntry            `yaml:"global"`
	Projects map[string][]InstalledEntry `yaml:"projects,omitempty"`
}

// InstalledPath returns the path to installed.yaml under the config root.
func InstalledPath() string {
	return filepath.Join(CacheRoot(), installedFileName)
}

// LoadInstalled loads the install tracking database.
// Missing file returns (nil, nil). Callers may treat nil entries as an empty list.
func LoadInstalled() ([]InstalledEntry, error) {
	path := InstalledPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var f installedFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}

	var result []InstalledEntry
	for i := range f.Global {
		f.Global[i].ProjectPath = ""
		result = append(result, f.Global[i])
	}
	for path, entries := range f.Projects {
		for i := range entries {
			entries[i].ProjectPath = path
			result = append(result, entries[i])
		}
	}

	return result, nil
}

// SaveInstalled writes the install tracking database atomically.
// entries may be nil (writes empty list).
func SaveInstalled(entries []InstalledEntry) error {
	path := InstalledPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Partition entries by ProjectPath
	f := installedFile{
		Global:   []InstalledEntry{},
		Projects: make(map[string][]InstalledEntry),
	}

	for _, e := range entries {
		if e.ProjectPath == "" {
			f.Global = append(f.Global, e)
		} else {
			f.Projects[e.ProjectPath] = append(f.Projects[e.ProjectPath], e)
		}
	}

	// Empty projects map should not be serialized
	if len(f.Projects) == 0 {
		f.Projects = nil
	}

	data, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		if rmErr := os.Remove(tmp); rmErr != nil {
			return fmt.Errorf("rename %s: %w (cleanup of temp file failed: %v)", path, err, rmErr)
		}
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}
