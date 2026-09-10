# ADR-064: Isis AutoRenice Governs Runaways, Not Governed Compute

## Status

Accepted 2026-09-10 — owner decision: keep both defaults (pressure gate on, governed-compute allowlist). Authored by claude-io from the io-connect long-receipt investigation.

## Context

The Pantheon menubar starts the Isis watchdog with `AutoRenice = true`. Any process at ≥ 80 % CPU for three consecutive 5 s samples is hit once with `renice 10` + `taskpolicy -b` (Background QoS). On a Mac that is the wrong test: a 10-core M1 Pro running one MLX workload sits at 540 % CPU by design.

Measured on 2026-09-10 (stele `auto_renice` rows, both Macs): the guard demoted SNE inference (`Python`, `metal`), the codex lanes (278 times on the M5), compilers (`clang`, `swift-frontend`), Go test binaries (`routerstore.test`, `tbraw.test`), the io-connect receipts and the sleeve sim. A demoted process's sockets gain `SOF1_TRAFFIC_MGT_SO_BACKGROUND`; TCP drops to the BK class; a 15 MB all_gather over Thunderbolt went 5.5 ms → 8 ms → 65–85 ms. It looked like a leak that starts after ten seconds. Evidence and repro: sirsi-io-connect `docs/evidence/long-run-collapse-isis-watchdog-20260910.md`.

The only exemption was the hardcoded protected list, whose sole non-system entry is the substring `sirsi`.

## Decision

1. **Pressure gate (default on).** `WatchConfig.AutoReniceOnlyUnderPressure` holds the renice unless the host is at kernel memory-pressure Warn or Critical (in-process dispatch level, else the Hapi cross-process cache, else Unknown = hold). Raw CPU % alone never demotes. The Sekhmet alert is still emitted.
2. **Governed-compute allowlist.** `WatchConfig.AutoReniceExempt` (case-insensitive name fragments), `nil` = `DefaultAutoReniceExempt`: `python mlx sne codex clang swift ld metal .test go node ioconnect tcpbench sirsimpi`. An empty non-nil slice exempts nothing. The hardcoded protected list is unchanged and still wins.
3. **Never silent.** A held renice is inscribed as `auto_renice_held` with its reason (`exempt:<frag>` or `no-pressure:<level>`); `sirsi report` prints the last 24 h of demotions and holds; `sirsi guard undo <pid>` reverses one (`renice 0`, `taskpolicy -B`).

Owner knobs (pick in the PR): keep both defaults; or allowlist only (gate off); or gate only (allowlist empty); or AutoRenice opt-in per host via the menubar config.

## Alternatives Considered

- Disable AutoRenice outright — loses the one thing it is for (a true runaway under memory pressure; ADR-035).
- Raise the CPU threshold above one core (e.g. 400 %) — still the wrong signal: a runaway at 120 % under pressure is worse than a workload at 800 % on an idle host.
- Rename every hot binary to contain `sirsi` — works (the io-connect receipts do it today) but is a workaround, not a policy.

## Consequences

- Workloads on the Sirsi Macs run at full priority unless the host is actually short of memory.
- The demotion count becomes visible; the blind spot that cost a day of receipt debugging is closed.
- Tests: `internal/guard/watchdog_policy_test.go` (exempt names, pressure gate, held-but-alerted, undo).

## References

- ADR-035 Runaway-Proof Execution (the Runaway Executor doctrine this refines)
- sirsi-io-connect `docs/evidence/long-run-collapse-isis-watchdog-20260910.md`
- xnu `bsd/sys/socketvar.h` (`SOF1_TRAFFIC_MGT_SO_BACKGROUND`)
