//go:build darwin || linux

package completionproof

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openCompletionProof(path string) (*os.File, error) {
	// Refuse a substituted symlink and avoid waiting for a FIFO writer before
	// captureCompletionProof can inspect the retained descriptor's file type.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("adopt completion proof descriptor")
	}
	return file, nil
}
