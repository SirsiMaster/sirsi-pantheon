//go:build !darwin

package setup

import (
	"context"
	"errors"
)

type unsupportedServiceBackend struct{}

func NativeServiceBackend() ServiceBackend { return unsupportedServiceBackend{} }
func (unsupportedServiceBackend) Inspect(context.Context, string) (ServiceInstance, error) {
	return ServiceInstance{}, errors.New("service lifecycle requires native macOS")
}
func (unsupportedServiceBackend) Refresh(context.Context, ServiceInstance) error {
	return errors.New("service lifecycle requires native macOS")
}
