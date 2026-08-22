//go:build !darwin

package apprecovery

import "errors"

func DefaultRegistryPath() (string, error) {
	return "", errors.New("application recovery is supported on macOS only")
}
func RegisterDefaultTarget(Target) error {
	return errors.New("application recovery is supported on macOS only")
}
func RemoveDefaultTarget(string) error {
	return errors.New("application recovery is supported on macOS only")
}
