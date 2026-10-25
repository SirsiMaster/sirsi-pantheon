//go:build !darwin

package routerstore

import (
	"context"
	"io"
	"strings"
)

func (*Relay) awaitMaintenanceStartup(context.Context) (io.Closer, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
