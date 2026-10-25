//go:build !darwin

package setup

import (
	"context"
	"errors"
)

func WithRootRelayMaintenanceAuthority(context.Context, func(string, string, string) error) error {
	return errors.New("legacy maintenance: native root relay authority required")
}
