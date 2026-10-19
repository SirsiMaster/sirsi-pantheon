//go:build !darwin && !linux

package router

import "os"

func wakeFileIdentity(os.FileInfo) (wakeImageIdentity, error) {
	return wakeImageIdentity{}, os.ErrInvalid
}
