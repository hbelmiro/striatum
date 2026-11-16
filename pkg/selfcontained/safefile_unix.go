//go:build unix

package selfcontained

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// openContainedRegular opens root/rel nonblocking (so a FIFO swapped in
// after validation cannot block the open) and verifies on the opened
// descriptor that the file it refers to stays inside root (so a component
// swapped for an escaping symlink cannot redirect the read). The regular
// file check on the descriptor happens in ReadFileSafe.
func openContainedRegular(root, clean, display string) (*os.File, error) {
	path := filepath.Join(root, filepath.FromSlash(clean))
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", display, err)
	}
	f := os.NewFile(uintptr(fd), path)
	// Resolve the descriptor's real path and require it to stay under the
	// resolved root; fail closed when the resolution is unavailable.
	realPath, err := fdRealPath(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("resolve real path of %q: %w", display, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("resolve root %q: %w", root, err)
	}
	if !containedUnder(realPath, resolvedRoot) {
		_ = f.Close()
		return nil, fmt.Errorf("open %q: resolved path %q escapes root %q", display, realPath, resolvedRoot)
	}
	return f, nil
}

// containedUnder reports whether path is root or lies beneath it. On
// case-insensitive filesystems (common on macOS) the descriptor's canonical
// casing can differ from the supplied root casing; a case-variant path can
// only have opened successfully on such a filesystem, so a case-insensitive
// match is accepted as containment.
func containedUnder(path, root string) bool {
	if path == root || strings.HasPrefix(path+string(filepath.Separator), root+string(filepath.Separator)) {
		return true
	}
	return strings.EqualFold(path, root) ||
		strings.HasPrefix(strings.ToLower(path)+string(filepath.Separator), strings.ToLower(root)+string(filepath.Separator))
}
