//go:build darwin && !cgo

package setup

import "errors"

func maintenanceSignedImage(string, int) (string, error) {
	return "", errors.New("legacy maintenance: Darwin Security.framework loaded-image binding requires cgo; no fallback")
}
