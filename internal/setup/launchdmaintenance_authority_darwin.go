//go:build darwin

package setup

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// WithRootRelayMaintenanceAuthority uses only the existing selected relay's
// protected service route. The token stays in this callback's memory; it is
// never a plan/journal field, environment export, cache or alternate authority.
func WithRootRelayMaintenanceAuthority(ctx context.Context, use func(string, string, string) error) error {
	return (nativeServiceBackend{}).withMaintenanceAuthority(ctx, use)
}

func (b nativeServiceBackend) withMaintenanceAuthority(ctx context.Context, use func(string, string, string) error) error {
	if use == nil {
		return errors.New("legacy maintenance: authorization callback required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	d := b.relayDependencies()
	if err := rootRelayOwner(d.uid(), d.euid()); err != nil {
		return err
	}
	raw, hash, err := d.artifact(rootRelayPlist, true)
	if err != nil {
		return err
	}
	decoded, err := d.read(ctx, raw, "/usr/bin/plutil", "-convert", "json", "-o", "-", "-")
	if err != nil {
		return err
	}
	var config rootRelayConfiguration
	if json.Unmarshal(decoded, &config) != nil || validateRootRelayConfiguration(config) != nil {
		return errors.New("legacy maintenance: selected service configuration invalid")
	}
	var private struct{ EnvironmentVariables map[string]string }
	if json.Unmarshal(decoded, &private) != nil {
		return errors.New("legacy maintenance: selected service authority unavailable")
	}
	endpoint, token := private.EnvironmentVariables["SIRSI_ROUTER_URL"], private.EnvironmentVariables["SIRSI_ROUTER_TOKEN"]
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || token == "" {
		return errors.New("legacy maintenance: selected relay has no direct canonical authority; no spool/DB/credential fallback")
	}
	if _, current, err := d.artifact(rootRelayPlist, true); err != nil || current != hash {
		return errors.New("legacy maintenance: selected authority changed before authorization")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	useErr := use(endpoint, token, private.EnvironmentVariables[routerstore.EnvSPKIPin])
	if useErr != nil {
		useErr = maintenanceAuthorityError{useErr, token}
	}
	if _, current, err := d.artifact(rootRelayPlist, true); err != nil || current != hash {
		return errors.Join(useErr, errors.New("legacy maintenance: selected authority changed during authorization"))
	}
	return useErr
}

type maintenanceAuthorityError struct {
	cause  error
	secret string
}

func (e maintenanceAuthorityError) Error() string {
	return strings.ReplaceAll(e.cause.Error(), e.secret, "[redacted]")
}
func (e maintenanceAuthorityError) Unwrap() error { return e.cause }
