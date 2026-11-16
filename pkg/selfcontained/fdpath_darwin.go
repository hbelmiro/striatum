//go:build darwin

package selfcontained

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// fdRealPath returns the filesystem path an open descriptor refers to,
// using fcntl(F_GETPATH): on macOS, /dev/fd entries do not support
// readlink.
func fdRealPath(f *os.File) (string, error) {
	var buf [1024]byte
	// F_GETPATH = 50 on darwin.
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), 50, uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return "", fmt.Errorf("F_GETPATH: %w", errno)
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i]), nil
		}
	}
	return string(buf[:]), nil
}
