# ADR-062 G3: two-host claim contention, M1 + M5 (2026-10-26)

**Verdict: PASS.** 1,000 contended rounds, with one claimer on each of two Macs, against a test router service
with an injected delay of 2× the measured p99. **Every round had exactly one winner.** The negative control
(each host on its own ledger) saw two winners in 50 of 50 rounds, so the counter can fail.

This closes the gap the readiness audit left open for G3
([ADR-062-ROUTER-READINESS-AUDIT-20261005.md](ADR-062-ROUTER-READINESS-AUDIT-20261005.md), row G3: "The two-host,
2× p99 delayed variant … is not run"). The single-host 1,000-round test with its negative control was already
in `TestClaimExactlyOnceAcrossManyRounds`.

## What ran

| | |
|---|---|
| Service | `sirsi router serve` v0.24.137 on the M1, bound to its Tailscale address only, with a **scratch SQLite** store (`--store <scratch>/g3.db`). `SIRSI_ROUTER_RULE_OF_RA=off`; per-run random bearer token, never written to disk. |
| Delay | `SIRSI_ROUTER_SERVE_TEST_DELAY=312ms` on every call. That is 2 × the p99 of 100 uncontended claims from the M5 (p50 77 ms, p99 156 ms), measured in phase A on the same link. |
| Claimers | The shipped CLI, `sirsi router task claim-id ra rNNNN`, one subprocess per round on each host. M1 (`sirsimasterdev`) and M5 (`thekryptodragon`), both v0.24.137. |
| Timing | Both hosts fire round *i* at a shared wall-clock time `t0 + i·0.93 s` plus 0–30 ms random jitter. The M5 schedule is shifted by the measured clock offset (+78 ms). The M1 fires 111 ms later (half the median RTT), so that both requests land inside the server's 312 ms delay window together. |
| Isolation | Separate `HOME` per host, client agent ids `g3-m1` / `g3-m5` (the session cache is keyed by agent), and an explicit test URL. The production router (Cloud Run `sirsi-router`) was not contacted. Scratch homes on both hosts were deleted after the run. |

## Results (`raw/tally.txt`)

| phase | rounds | winners per round | other errors | latency (client, incl. 312 ms delay) |
|---|---|---|---|---|
| B: shared ledger | 1000 | **{1: 1000}**. M1 won 138, M5 won 862 | 0 | p50 384 ms, p99 487 ms |
| C: separate ledgers (negative control) | 50 | {2: 50} | 0 | |

- Both hosts won rounds. The skew toward the M5 comes from the arrival alignment, not from a fairness property of the claim; G3 asks for one winner per round, not an even split.
- Every losing claim was the expected `no claimable task` refusal.

## Reproduce

```
docs/evidence/ADR-062-G3-TWO-HOST-CONTENTION-20261026/run.sh
```

- Needs passwordless ssh from the M1 to the M5 over Tailscale, and `sirsi` on both hosts.
- `tally.py raw/` recomputes the verdict from the committed per-round TSVs. Each row is round, exit code, ms and class. `raw/SHA256SUMS` pins those files.

## Limits (what this does not show)

- **Store driver.** The test service ran on **SQLite**, not on the Postgres store that production uses (`FOR UPDATE SKIP LOCKED`). This run proves the two-host path end to end through the HTTP service, auth and the CLI, with genuine concurrent arrival. Postgres claim atomicity is covered separately by the dual-driver contract suite (rs-07); a two-host run against a Postgres test service has not been done.
- **Outage leg.** G3's 30 s service outage leg is unchanged by this run. It is still covered by `outage_test.go`.
- **The link.** The M1 was on a travel hotspot (RTT ≈ 210 ms to the M5). A faster LAN would shrink the delay window, but not the one-winner property this run measures.
