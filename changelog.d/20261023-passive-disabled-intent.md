### Fixed

- Passive managed LaunchAgent recovery preserves this host's OS-disabled labels,
  whether loaded or unloaded. Missing, failed, or malformed disabled-state probes
  refuse recovery before enable/bootstrap. Enabled unloaded jobs retain recovery.
- Broker liveness observes the same disabled intent before model probing or
  start/restart. Existing quarantine precedence and confirmed, target-scoped
  disabled-service repair remain intact. No historical machine-specific preference
  or owner-reversal gate is imposed.
- Added injected fixtures for passive refusal, enabled recovery, confirmed repair,
  malformed native output, and broker refusal without model/service calls.
- Corrected the disabled-label diagnostic: intentional stops remain unchanged;
  deliberate restoration uses the existing confirmed Ma'at repair and verification.

Scope: these are per-pass intent snapshots, not atomic protection against an
operator changing overrides concurrently. Test execution is reserved for main's
admitted verification; this source change makes no runtime qualification claim.

Retained residual outside this patch: `scripts/router/sirsi-fabric-watchdog.sh` retains unguarded
kickstart attempts; its template points at an installed copy under `~/.local/bin`.
ADR-054 describes that watchdog as LIVE, while no installer wiring was found in
the current Go setup/CLI, install script, Makefile, or GoReleaser references.
Main's read-only host inspection established that the installed script is byte
identical to the retained source; no matching user LaunchAgent plist reference
was found. File presence does not establish active execution, and loaded-label
verification remains separate. This patch does not qualify that retained
restorer or claim all-restorer closure.
