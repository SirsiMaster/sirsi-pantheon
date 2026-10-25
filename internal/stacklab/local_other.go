//go:build !unix

package stacklab

import "errors"

func InspectLocal(string) (LocalState, error) {
	return LocalState{}, errors.New("local Stack Lab persistence unavailable on this platform")
}
func SetLocal(string, bool, string) (LocalState, error) {
	return LocalState{}, errors.New("local Stack Lab persistence unavailable on this platform")
}
