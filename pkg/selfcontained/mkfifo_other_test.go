//go:build !unix

package selfcontained

import "errors"

func mkfifo(path string) error { return errors.New("fifo not supported on this platform") }
