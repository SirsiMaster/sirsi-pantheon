# Unreleased — transport default fixture alignment

Align the Go plan-only fixtures with the retained direct-default implementation:
when all six proxy environment variables are absent or empty, plans for each
enrolled host must equal an explicit direct plan. Explicit SOCKS without a proxy
still refuses. Existing unknown-host, remote-proxy, conflicting-identity,
injected-work, timeout, command and strict transport checks remain intact.

This changes no production transport and performs no remote execution. The
previous combined normal-test failure remains retained; this successor still
requires its own baseline.

Refs: ADR-062, ADR-063.
