package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// manifestFlagUsage is the description for -f / --manifest on validate, pack, and push.
const manifestFlagUsage = "path to a project directory (uses artifact.json inside) or to a manifest file whose basename is artifact.json (case-insensitive); any other basename is treated as a directory and artifact.json is appended; spec.files are relative to the manifest directory (default: ./artifact.json in cwd)"

const defaultManifestName = "artifact.json"

// resolveManifestAndProjectRoot returns the absolute path to artifact.json and the
// project root directory (its parent). Paths in spec.files are interpreted
// relative to projectRoot.
//
// If manifestFlag is empty, uses filepath.Join(cwd, defaultManifestName).
// When the final path component is not defaultManifestName (compared with
// strings.EqualFold), the path is treated as a project directory and
// defaultManifestName is appended (even when that directory does not exist yet),
// so load errors refer to the intended manifest file. A manifest file with any
// other basename is not supported via this flag.
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
	if !strings.EqualFold(filepath.Base(candidate), defaultManifestName) {
		candidate = filepath.Join(candidate, defaultManifestName)
	}
	manifestPath, err = filepath.Abs(candidate)
	if err != nil {
		return "", "", fmt.Errorf("resolve manifest path: %w", err)
	}
	projectRoot = filepath.Dir(manifestPath)
	return manifestPath, projectRoot, nil
}
