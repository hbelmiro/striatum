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
	ProjectPath   string `yaml:"project_path,omitempty"`
	InstalledWith string `yaml:"installed_with,omitempty"` // empty = root install
	Status        string `yaml:"status"`
	LastError     string `yaml:"last_error,omitempty"`
	UpdatedAt     string `yaml:"updated_at"`
}

type installedFile struct {
	Entries []InstalledEntry `yaml:"entries"`
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
	if f.Entries == nil {
		return nil, nil
	}
	return f.Entries, nil
}

// SaveInstalled writes the install tracking database atomically. entries may be nil (writes empty list).
func SaveInstalled(entries []InstalledEntry) error {
	path := InstalledPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f := installedFile{Entries: entries}
	if f.Entries == nil {
		f.Entries = []InstalledEntry{}
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
