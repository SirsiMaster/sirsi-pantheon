//go:build darwin || linux

package router

import (
	"os"
	"syscall"
)

func wakeFileIdentity(info os.FileInfo) (wakeImageIdentity, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return wakeImageIdentity{}, os.ErrInvalid
	}
	return wakeImageIdentity{uint64(stat.Dev), uint64(stat.Ino)}, nil
}
