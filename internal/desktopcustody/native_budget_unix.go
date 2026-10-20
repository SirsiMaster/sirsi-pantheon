//go:build unix

package desktopcustody

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func readNativeBudgetFile(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return nil, err
	}
	if st.Uid != 0 || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0o077 != 0 || st.Nlink != 1 || st.Size > 16<<10 {
		return nil, errors.New("sleep custody: execution fact is not private, immutable native supervisor data")
	}
	data := make([]byte, st.Size)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, err
	}
	return data, nil
}
