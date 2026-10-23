package setup

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// ServiceInstance describes one launchd-owned service, not an arbitrary PID.
// The caller pins this read-only plan before requesting the finite refresh.
type ServiceInstance struct {
	Label         string `json:"label"`
	Target        string `json:"target"`
	Plist         string `json:"plist"`
	PlistSHA256   string `json:"plist_sha256"`
	Program       string `json:"program"`
	ProgramSHA256 string `json:"program_sha256"`
	PID           int    `json:"pid"`
	Started       string `json:"started"`
}

// ServiceBackend is injected so tests never signal or restart host services.
type ServiceBackend interface {
	Inspect(context.Context, string) (ServiceInstance, error)
	Refresh(context.Context, ServiceInstance) error
}

type ServiceController struct {
	Backend   ServiceBackend
	Authorize func(context.Context, string) error
}

var serviceDigestRE = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validateServiceInstance(p ServiceInstance) error {
	if p.Label == "" || p.Target == "" || p.Plist == "" || p.Program == "" || p.PID <= 1 || p.Started == "" || !serviceDigestRE.MatchString(p.PlistSHA256) || !serviceDigestRE.MatchString(p.ProgramSHA256) {
		return errors.New("service lifecycle: complete exact instance and file digests required")
	}
	return nil
}

// Refresh never enumerates jobs, enables disabled jobs, elevates privileges or
// signals a supplied PID. It refreshes the single approved label only after
// re-reading its instance and artifacts. This is cooperative lifecycle control,
// not a sandbox against an account owner independently operating launchd.
func (c ServiceController) Refresh(ctx context.Context, expected ServiceInstance) (ServiceInstance, error) {
	if err := validateServiceInstance(expected); err != nil {
		return ServiceInstance{}, err
	}
	if c.Backend == nil || c.Authorize == nil {
		return ServiceInstance{}, errors.New("service lifecycle: native backend and owning actor authorization required")
	}
	if err := c.Authorize(ctx, expected.Label); err != nil {
		return ServiceInstance{}, err
	}
	actual, err := c.Backend.Inspect(ctx, expected.Label)
	if err != nil {
		return ServiceInstance{}, err
	}
	if actual != expected {
		return ServiceInstance{}, errors.New("service lifecycle: instance or artifact changed since plan; no restart performed")
	}
	if err := c.Backend.Refresh(ctx, expected); err != nil {
		return ServiceInstance{}, fmt.Errorf("service lifecycle: selected service refresh failed; owning application must perform its recorded recovery: %w", err)
	}
	current, err := c.Backend.Inspect(ctx, expected.Label)
	if err != nil {
		return ServiceInstance{}, fmt.Errorf("service lifecycle: restart readback failed: %w", err)
	}
	if current.Target != expected.Target || current.Label != expected.Label || current.Plist != expected.Plist || current.PlistSHA256 != expected.PlistSHA256 || current.Program != expected.Program || current.ProgramSHA256 != expected.ProgramSHA256 || current.PID <= 1 || current.Started == "" || (current.PID == expected.PID && current.Started == expected.Started) {
		return ServiceInstance{}, errors.New("service lifecycle: selected service did not return with the planned artifacts and a new instance")
	}
	return current, nil
}
