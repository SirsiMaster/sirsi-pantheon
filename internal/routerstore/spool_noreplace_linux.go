//go:build linux

package routerstore

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameSpoolNoReplace(source *os.File, sourceName string, destination *os.File, destinationName string) error {
	return unix.Renameat2(int(source.Fd()), sourceName, int(destination.Fd()), destinationName, unix.RENAME_NOREPLACE)
}
