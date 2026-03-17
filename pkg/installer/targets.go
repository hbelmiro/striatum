package installer

import (
	"errors"
	"os"
	"path/filepath"
)

// Targets returns the absolute path of the skills directory for the given target and optional project path.
// target must be "cursor" or "claude". projectPath should already be absolute when provided;
// if relative, it is resolved via filepath.Abs. When empty, the user's home directory is used.
func Targets(target, projectPath string) (string, error) {
	switch target {
	case "cursor", "claude":
		// ok
	default:
		return "", errors.New("target must be cursor or claude")
	}
	subdir := "." + target
	base := "skills"
	var baseDir string
	if projectPath != "" {
		if !filepath.IsAbs(projectPath) {
			abs, err := filepath.Abs(projectPath)
			if err != nil {
				return "", err
			}
			projectPath = abs
		}
		baseDir = filepath.Join(projectPath, subdir, base)
	} else {
		home := os.Getenv("HOME")
		if home == "" {
			var err error
			home, err = os.UserHomeDir()
			if err != nil {
				return "", err
			}
		}
		baseDir = filepath.Join(home, subdir, base)
	}
	return filepath.Clean(baseDir), nil
}

// InstallToTarget copies the contents of cacheDir to targetDir/name/ (creates or overwrites).
func InstallToTarget(cacheDir, targetDir, name string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(targetDir, name)
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return copyDir(cacheDir, dest)
}

// RemoveFromTarget removes the directory targetDir/name. Returns nil if it does not exist.
// Returns an error if the path exists but is not a directory.
func RemoveFromTarget(targetDir, name string) error {
	p := filepath.Join(targetDir, name)
	info, err := os.Stat(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	return os.RemoveAll(p)
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			info, err := e.Info()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, info.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}
