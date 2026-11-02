//go:build !darwin && !linux

package routerstore

import (
	"errors"
	"os"
)

func renameSpoolNoReplace(*os.File, string, *os.File, string) error {
	return errors.New("relay: atomic no-clobber custody transfer is unsupported on this platform")
}
