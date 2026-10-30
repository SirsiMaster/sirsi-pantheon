# ADR-062 G3: two-host claim contention on the Postgres store, M1 + M5 (2026-10-29)

**Verdict: PASS.** I repeated the [2026-10-26 G3 run](ADR-062-G3-TWO-HOST-CONTENTION-20261026.md) with the test service's
store moved from scratch SQLite to **Postgres 16**, the backend production uses. 1,000 contended rounds, with one
claimer on each of two Macs, at 2× the measured p99 delay. **Every round had exactly one winner.** The negative
control (each host on its own database) saw two winners in 50 of 50 rounds.

This closes the limit the 10-26 run recorded ("a two-host run against a Postgres test service has not been done"),
which Mercury raised as a non-blocking note on #1222.

## What ran

| | |
|---|---|
| Store | A disposable **PostgreSQL 16.15** cluster on the M1. `initdb` ran in the run directory with trust auth. It listened on `127.0.0.1:55433` only, with no Unix socket, and was stopped and deleted at the end. Roles and schema came from `v0.24.143:internal/routerstore/pg/{roles,schema}.sql`. Each database is owned by `router_migrator`, with the schema applied as that role. The service connects as `router_service` (DML only), as in production. One database per ledger: `base`, `g3`, `left`, `right`. |
| Service | `sirsi router serve` v0.24.143 on the M1, bound to its Tailscale address only, with `--store postgres://router_service@127.0.0.1:55433/<db>`. `SIRSI_ROUTER_RULE_OF_RA=off`; per-run random bearer token, never written to disk. |
| Seeding | Through an undelayed test service on `:18764`, using the CLI's remote `task add`. The store's own invariants apply, with no SQL inserts behind its back. That service was stopped before each measured phase. |
| Delay | `SIRSI_ROUTER_SERVE_TEST_DELAY=204ms` on every call. That is 2 × the p99 of 100 uncontended claims from the M5 (p50 82 ms, p99 102 ms; all 100 won). |
| Claimers, timing, isolation | Unchanged from 10-26. The shipped CLI `sirsi router task claim-id` runs on both hosts (both v0.24.143). Rounds fire on a shared wall-clock schedule with a period of 0.66 s and 0–30 ms jitter. The M5 is shifted by the measured clock offset (+78.5 ms), and the M1 fires 105 ms later (half the median RTT). Each host has its own `HOME`, the client ids are `g3-m1` / `g3-m5`, and the URL is an explicit test URL. Production (Cloud Run `sirsi-router`, Cloud SQL) was not contacted. |

## Results (`raw/tally.txt`)

| phase | rounds | winners per round | other errors | latency (client, incl. 204 ms delay) |
|---|---|---|---|---|
| B: shared Postgres ledger | 1000 | **{1: 1000}**. M1 won 274, M5 won 726 | 0 | p50 272 ms, p99 303 ms |
| C: separate databases (negative control) | 50 | {2: 50} | 0 | |

- **Store readback** (`raw/g3-readback.txt`). After phase B, `router.tasks` in the shared database holds exactly `in-progress|1000`. Every seeded task was leased once, and none was left claimable or double-counted.
- Every losing claim was the expected `no claimable task` refusal.

## Reproduce

```
docs/evidence/ADR-062-G3-TWO-HOST-CONTENTION-PG-20261029/run.sh
```

- Needs Homebrew `postgresql@16` on the M1, passwordless ssh to the M5 over Tailscale, and `sirsi` on both hosts.
- The driver and tally are the 10-26 files, used by reference.
- `tally.py raw/` recomputes the verdict from the committed per-round TSVs, and `raw/SHA256SUMS` pins them.
- The committed `run.sh` is byte-identical to the executed copy (`cmp`).
- The serve logs went through the post-run path redaction from #1265. Here the store is a DSN with the userinfo already redacted (`postgres://…@127.0.0.1:55433/<db>`), so no host path appears.

## Limits

- **Local Postgres, not Cloud SQL.** This is the same driver, schema and role split, but on loopback, not across the Cloud SQL connector. Connector latency changes the delay window, not the claim's atomicity.
- **Outage leg.** G3's 30 s service outage leg is unchanged. It is still covered by `outage_test.go`.
- **The link.** RTT to the M5 was about 210 ms (travel link), as on 10-26.
