//go:build darwin && !cgo

package setup

import "errors"

func maintenanceSignedImage(string, int) (string, error) {
	return "", errors.New("legacy maintenance: Darwin Security.framework loaded-image binding requires cgo; no fallback")
}

func maintenanceKernelCDHash(int) ([]byte, error) {
	return nil, errors.New("legacy maintenance: kernel loaded-image identity requires cgo; no fallback")
}
