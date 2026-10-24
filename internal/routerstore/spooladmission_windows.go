//go:build windows

package routerstore

import (
	"context"
	"errors"
	"os"
)

func lockSpoolAdmission(context.Context, *os.Root, bool) (*os.File, error) {
	return nil, errors.New("spool admission unsupported on Windows")
}
func WithSpoolQuiescence(context.Context, string, func() error) error {
	return errors.New("spool admission unsupported on Windows")
}
