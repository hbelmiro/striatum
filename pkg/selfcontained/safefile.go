// Package selfcontained prepares self-contained images of Skill artifacts: the
// root Skill plus its complete Skill/Prompt dependency closure, with
// artifact-owned references rewritten to resolve inside the self-contained tree.
package selfcontained

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ReservedPrefix is the image-path prefix reserved for Striatum export
// metadata (e.g. ".striatum/provenance.json").
const ReservedPrefix = ".striatum/"

// CheckRegularFile verifies that rel (a slash-separated path relative to root)
// refers to a regular file, and that every path component beneath root is a
// real directory (not a symlink). Symlinks, directories used as files, FIFOs,
// sockets, and device files are rejected. Hard-linked regular files are
// allowed. Because symlinks are rejected outright, no symlink-resolution
// containment check (which is fragile across platform root aliases such as
// macOS /var vs /private/var) is needed.
func CheckRegularFile(root, rel string) error {
	clean, err := cleanRelPath(rel)
	if err != nil {
		return err
	}
	parts := strings.Split(clean, "/")
	cur := root
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			return fmt.Errorf("stat %q: %w", rel, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%q: symlink components are not allowed in export inputs", rel)
		}
		if i == len(parts)-1 {
			if !fi.Mode().IsRegular() {
				return fmt.Errorf("%q is not a regular file (mode %s)", rel, fi.Mode())
			}
			return nil
		}
		if !fi.IsDir() {
			return fmt.Errorf("%q: %q is not a directory", rel, part)
		}
	}
	return fmt.Errorf("%q is empty", rel)
}

// ReadFileSafe reads the file at root/rel after validating it is a regular
// file with no symlink components. The read goes through os.OpenRoot, which
// enforces containment on the opened directory handle itself, so a path
// component swapped for an escaping symlink between the validation walk and
// the open cannot make the read follow it outside root. The opened file
// descriptor is stat'ed so a file swapped in as a FIFO cannot block the read.
func ReadFileSafe(root, rel string) ([]byte, error) {
	if err := CheckRegularFile(root, rel); err != nil {
		return nil, err
	}
	clean, err := cleanRelPath(rel)
	if err != nil {
		return nil, err
	}
	dirRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open root %q: %w", root, err)
	}
	defer func() { _ = dirRoot.Close() }()
	f, err := dirRoot.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", rel, err)
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", rel, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file (mode %s)", rel, fi.Mode())
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", rel, err)
	}
	return data, nil
}

// cleanRelPath validates rel as a portable slash path and returns it cleaned.
func cleanRelPath(rel string) (string, error) {
	if strings.Contains(rel, "\\") {
		return "", fmt.Errorf("path %q: backslashes are not allowed", rel)
	}
	if path.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("path %q: absolute paths are not allowed", rel)
	}
	clean := path.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q: must stay beneath the root", rel)
	}
	return clean, nil
}

// DestRegistry tracks destination paths inside the self-contained image and rejects
// duplicates, file/directory collisions, and unsafe or reserved destinations
// before the image is constructed.
type DestRegistry struct {
	files map[string]bool
	dirs  map[string]bool
}

// NewDestRegistry returns an empty destination registry.
func NewDestRegistry() *DestRegistry {
	return &DestRegistry{files: map[string]bool{}, dirs: map[string]bool{}}
}

// Add records dest (a slash-separated image-relative path) after validating
// it. It errors on duplicate destinations, on a file colliding with a
// previously registered directory (or vice versa), and on unsafe or reserved
// paths.
func (r *DestRegistry) Add(dest string) error {
	clean, err := cleanRelPath(dest)
	if err != nil {
		return err
	}
	if strings.HasPrefix(clean, ReservedPrefix) {
		return fmt.Errorf("destination %q: %q is reserved for Striatum export metadata", dest, ReservedPrefix)
	}
	if r.files[clean] {
		return fmt.Errorf("duplicate destination %q", dest)
	}
	if r.dirs[clean] {
		return fmt.Errorf("destination %q collides with an existing directory", dest)
	}
	// Register all parent directories, rejecting collisions with files.
	parts := strings.Split(clean, "/")
	for i := 1; i < len(parts); i++ {
		dir := strings.Join(parts[:i], "/")
		if r.files[dir] {
			return fmt.Errorf("destination %q collides with an existing file at %q", dest, dir)
		}
		r.dirs[dir] = true
	}
	r.files[clean] = true
	return nil
}
