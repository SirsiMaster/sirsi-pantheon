//go:build unix

package maat

import (
	"os"

	"golang.org/x/sys/unix"
)

func openPulseRosterFile(path string) (*os.File, error) {
	// Avoid waiting on a FIFO before the caller checks the retained file type.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
