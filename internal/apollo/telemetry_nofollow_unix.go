//go:build !windows

package apollo

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openTelemetryFile obtains the leaf through O_NOFOLLOW. Telemetry is a
// local evidence input, so accepting a symlink between a pathname check and
// open would allow an unrelated file to be rendered as SNE state.
func openTelemetryFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("adopt no-follow Apollo telemetry descriptor")
	}
	return f, nil
}
