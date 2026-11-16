//go:build unix && !darwin

package selfcontained

import (
	"fmt"
	"os"
	"path/filepath"
)

// fdRealPath returns the filesystem path an open descriptor refers to via
// the procfs/devfs descriptor directory.
func fdRealPath(f *os.File) (string, error) {
	for _, base := range []string{"/proc/self/fd", "/dev/fd"} {
		if p, err := os.Readlink(filepath.Join(base, fmt.Sprint(f.Fd()))); err == nil {
			return filepath.Clean(p), nil
		}
	}
	return "", fmt.Errorf("descriptor path resolution unavailable")
}
