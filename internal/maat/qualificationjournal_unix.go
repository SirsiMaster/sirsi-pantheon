//go:build !windows

package maat

import (
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

func readLifecycleJournal(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > lifecycleHistoryBytes {
		return nil, fmt.Errorf("lifecycle history must be a bounded regular file")
	}
	var native unix.Stat_t
	if err := unix.Fstat(fd, &native); err != nil {
		return nil, err
	}
	if native.Uid != uint32(os.Geteuid()) || native.Nlink != 1 {
		return nil, fmt.Errorf("lifecycle history owner/link mismatch")
	}
	raw, err := io.ReadAll(io.LimitReader(f, lifecycleHistoryBytes+1))
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if len(raw) > lifecycleHistoryBytes || int64(len(raw)) != before.Size() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, fmt.Errorf("complete lifecycle history changed or exceeded bound")
	}
	return raw, nil
}
