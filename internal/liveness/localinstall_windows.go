package liveness

import (
	"context"
	"errors"
)

func CurrentLocalEngine() (string, error)                         { return "", errors.New("local watch requires macOS") }
func NativeLocalProbes(string, string, LocalCommand) []LocalProbe { return nil }
func InstallLocal(context.Context, string, string, LocalCommand) (string, error) {
	return "", errors.New("local watch requires macOS")
}
