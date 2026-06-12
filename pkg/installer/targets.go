package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var kindToSubdir = map[string]string{
	"Skill":    "skills",
	"Prompt":   "prompts",
	"Workflow": "workflows",
}

// Targets returns the absolute path of the install directory for the given target, project path, and artifact kind.
// target must be "cursor" or "claude". kind selects the subdirectory via kindToSubdir.
// Workflow artifacts only support target "claude".
// projectPath should already be absolute when provided; if relative, it is resolved via filepath.Abs.
// When empty, the user's home directory is used.
func Targets(target, projectPath, kind string) (string, error) {
	base, ok := kindToSubdir[kind]
	if !ok {
		return "", fmt.Errorf("kind %q is not installable; installable kinds: Skill, Prompt, Workflow", kind)
	}

	switch target {
	case "cursor", "claude":
		// ok
	default:
		return "", errors.New("target must be cursor or claude")
	}

	if kind == "Workflow" && target != "claude" {
		return "", fmt.Errorf("workflow artifacts only support --target claude")
	}

	subdir := "." + target
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

// CreateWorkflowSymlink creates a relative symlink at targetDir/<name>.js -> <name>/<entrypoint>.
// If a symlink already exists at that path it is replaced (idempotent). If a regular file or
// directory occupies the path an error is returned.
func CreateWorkflowSymlink(targetDir, name, entrypoint string) error {
	linkPath := filepath.Join(targetDir, name+".js")
	linkTarget := filepath.Join(name, entrypoint)

	info, err := os.Lstat(linkPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if removeErr := os.Remove(linkPath); removeErr != nil {
				return fmt.Errorf("remove existing workflow symlink %s: %w", linkPath, removeErr)
			}
		} else {
			return fmt.Errorf("cannot create workflow symlink: %s already exists and is not a symlink", linkPath)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.Symlink(linkTarget, linkPath)
}

// RemoveWorkflowSymlink removes the symlink at targetDir/<name>.js if it exists and is a symlink.
// If the path does not exist or is not a symlink, it is left unchanged and nil is returned.
func RemoveWorkflowSymlink(targetDir, name string) error {
	linkPath := filepath.Join(targetDir, name+".js")
	info, err := os.Lstat(linkPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	return os.Remove(linkPath)
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
