//go:build !unix

package selfcontained

import (
	"fmt"
	"os"
	"path/filepath"
)

// openContainedRegular is the portable fallback: open through os.OpenRoot,
// which enforces containment on the directory handle itself. Nonblocking
// semantics are best-effort here.
func openContainedRegular(root, clean, display string) (*os.File, error) {
	dirRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open root %q: %w", root, err)
	}
	defer func() { _ = dirRoot.Close() }()
	f, err := dirRoot.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", display, err)
	}
	return f, nil
}
