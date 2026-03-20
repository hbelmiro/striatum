package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// manifestFlagUsage is the description for -f / --manifest on validate, pack, and push.
const manifestFlagUsage = "path to artifact.json or project directory containing it; spec.files paths are relative to the manifest's directory (default: ./artifact.json in the current directory)"

// resolveManifestAndProjectRoot returns the absolute path to artifact.json and the
// project root directory (its parent). Paths in spec.files are interpreted
// relative to projectRoot.
//
// If manifestFlag is empty, uses filepath.Join(cwd, defaultManifestName).
// If manifestFlag names an existing directory, defaultManifestName is appended.
func resolveManifestAndProjectRoot(manifestFlag string) (manifestPath, projectRoot string, err error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("get working directory: %w", err)
	}
	rel := strings.TrimSpace(manifestFlag)
	var candidate string
	if rel == "" {
		candidate = filepath.Join(wd, defaultManifestName)
	} else if filepath.IsAbs(rel) {
		candidate = filepath.Clean(rel)
	} else {
		candidate = filepath.Clean(filepath.Join(wd, rel))
	}
	if fi, statErr := os.Stat(candidate); statErr == nil && fi.IsDir() {
		candidate = filepath.Join(candidate, defaultManifestName)
	}
	manifestPath, err = filepath.Abs(candidate)
	if err != nil {
		return "", "", fmt.Errorf("resolve manifest path: %w", err)
	}
	projectRoot = filepath.Dir(manifestPath)
	return manifestPath, projectRoot, nil
}
