package installer

import (
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

// SaveInstalled writes the install tracking database. entries may be nil (writes empty list).
func SaveInstalled(entries []InstalledEntry) error {
	path := InstalledPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
	return os.WriteFile(path, data, 0o600)
}
