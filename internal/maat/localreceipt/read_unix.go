//go:build unix

package localreceipt

import (
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

func readFact(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var native unix.Stat_t
	if err = unix.Fstat(fd, &native); err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0o600 || native.Uid != uint32(os.Getuid()) || native.Nlink != 1 || st.Size() > 16<<10 {
		return nil, errors.New("local admission: private singly linked regular fact required")
	}
	return io.ReadAll(io.LimitReader(f, 16<<10))
}
