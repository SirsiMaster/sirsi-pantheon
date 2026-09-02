# ADR-062 rs-13 — two-Mac concurrency evidence (2026-09-02)

**Claim (goal G3):** two hosts working the same ledger through the router
service never claim the same item concurrently, including with the service
delayed to 2× the measured p99 and with a 30 s database outage injected.
**Also produced:** latency p50/p95/p99 per verb per host (G3), both hosts read
identical counts in the same second (G7), rollback is one variable (G8).

Everything below was produced by `sirsi router bench` (hidden verb, this
commit) driving `routerstore.Resolve()` — the exact path every other verb
uses. Nothing was mocked. Reproduce: `docs/user-guides/router-service.md`
§ "Evidence run".

## Setup

| | |
|---|---|
| Service host | M5 "Mac", `sirsi router serve --store postgres://…/routerstore_bench --listen 127.0.0.1:18090` |
| Backend | scratch PostgreSQL 14.22 on the M5, schema v18 from `pg/schema.sql` as `router_migrator` |
| Node 2 | m1-backup "MacBookPro" (arm64, macOS 26.6.2, no toolchain) reached the service through `ssh -R 18090:127.0.0.1:18090` |
| Identity | per-host tokens minted with `sirsi router token mint`; each node minted its own session (runtime hash of its binary) |
| Binaries | same source, built three times during the run (each rebuild = new runtime hash = new session, by design) |

## Leg 1 — no injection

1,000 items seeded; both hosts claimed and completed concurrently.

```
records: 3042   distinct items claimed: 1000   items claimed by >1 host: 0
host verb                         n    p50 ms    p95 ms    p99 ms   errs
Mac ClaimNext                   805       1.0       1.2       1.4      0
Mac Complete                    805       0.6       0.7       0.8      0
Mac Inbox                       805       2.4       4.5       4.8      0
MacBookPro ClaimNext            195      12.3      16.3      22.9      0
MacBookPro Complete             195      11.2      17.0      19.4      0
MacBookPro Inbox                195      72.0     108.2     191.8      0
```

Database after: `items: completed=1000`; `lease_sessions` 1,000 rows over 2
distinct sessions (one per host).

## Leg 2 — 50 ms injected delay (≈2× M1 p99) + 30 s Postgres outage, first binary

`SIRSI_ROUTER_SERVE_TEST_DELAY=50ms`; `pg_ctl stop -m fast` at 22:34:26.8,
start at 22:34:59.1. Lease TTL 120 s.

```
records: 4029   distinct items claimed: 1010   items claimed by >1 host: 1
```

The one item, traced across both logs and the database:

```
22:34:26.827 MacBookPro ClaimNext  ok
22:34:26.888 MacBookPro Complete   FAIL  HTTP 401: GetSession: FATAL: terminating connection
22:36:26.057 Mac        ClaimNext  ok        ← exactly one lease TTL later
22:36:26.109 Mac        Complete   ok
db: status=completed  wake: item:create, item:open:…:1   lease_session = Mac's
```

**Verdict: not a concurrent duplicate.** The M1 claimed at the instant the
database went down, its completion failed, the lease expired at TTL, the item
reopened (`item:open` wake event) and the M5 took it. That is the designed
recovery. It exposed two real defects, both fixed in this commit and each now a
test:

1. **The service answered a dead database with 401.** Token/session lookups
   that failed on the store were treated as auth verdicts, so every call during
   the outage read "missing or invalid bearer token" and clients re-minted
   sessions. Now a store failure is **503 + `ErrServiceUnavailable`** (retry,
   credentials unchanged). `TestOutageIs503NotUnauthorized` proves the client
   keeps its session across an outage.
2. **The bench abandoned a lease after one failed completion.** A worker must
   retry a completion for as long as its lease is valid. The bench now retries
   with backoff up to the TTL, and the report distinguishes a *concurrent
   duplicate* (two hosts both complete an item) from a *lease-expiry reclaim*
   (first holder never completed; second claim after TTL).

Backpressure seen: 355 (M1) + 55 (M5) `dispatch budget exceeded` — the
store's per-agent concurrency cap doing its job under the delay.

## Leg 3 — same injection, fixed binary

```
records: 3568   distinct items claimed: 1000   concurrent duplicate claims: 0   lease-expiry reclaims (designed): 0
host verb                         n    p50 ms    p95 ms    p99 ms   errs backpres.  unavail.
Mac ClaimNext                   573      54.9      56.8      59.0      0         0       301
Mac Complete                    573      53.3      55.0      57.9      0         0         0
Mac Inbox                       572      58.0      62.8      64.6      0         0         1
MacBookPro ClaimNext            427      64.3      74.6     190.3      0         0       225
MacBookPro Complete             427      63.0      69.1      83.9      0         0         0
MacBookPro Inbox                427      95.9     133.4     192.9      0         0         0
```

All 1,000 items claimed exactly once and completed; every outage-window call
classified `unavailable` (503), zero unexplained errors, zero completion
failures (retries succeeded once the database returned).

**Sessions across the whole run** (mint time / host / runtime hash):

```
22:29:57 Mac/bench-seeder     82e10125      ← binary 1
22:30:28 MacBookPro/bench-m1  82e10125
22:31:04 Mac/bench-m5         82e10125
22:32:56 Mac/bench-seeder     240eef7e      ← binary 2 (rebuild)
22:34:12 Mac/bench-m5         240eef7e
22:43:54 Mac/bench-seeder     aa25d6d5      ← binary 3 (rebuild)
22:44:55 MacBookPro/bench-m1  aa25d6d5
22:45:02 Mac/bench-m5         aa25d6d5
outages: 22:34:26–22:34:59, 22:45:12–22:45:45
```

Every mint coincides with a rebuild (new runtime hash), none with an outage:
runtime binding behaves as ADR-062 §3 specifies, and after the 503 fix an
outage causes no re-mints.

## G7 — both hosts, same second

```
22:58:23  MacBookPro  Items: 3011 open, 0 closed   (via ssh tunnel, per-host token)
22:58:23  Mac         Items: 3011 open, 0 closed
```

(`router status` counts every non-closed row as open; the database holds
3,010 `completed` + 1 `open` — the seeder's over-quota escalation item,
addressed to the escalation agent, never a bench target.)

## G8 — rollback is one variable

With `SIRSI_ROUTER_URL` set, `router status` answered from the service in
0.10 s. With it unset, the same binary resolved the local file path in 0.02 s
(the throwaway `$HOME` used here has no local ledger, so it stopped at the
dirty-build guard rather than create one). The switch itself is
`routerstore.Resolve()`'s first branch and is unit-tested
(`TestResolveAndLocalPathRefuseWhenServiceURLSet`, `TestResolveOpensSIRSIRouterDBAndCreatesParent`).
A timed rehearsal against a real node home is Phase D step 20.

## Lease TTL rule (ADR-062 §2)

Highest p99 measured for a lease-bearing verb: 190.3 ms (M1 `ClaimNext` under
50 ms injected delay). 10× p99 = 1.9 s. The fleet default of 30 s and the
run's 120 s both satisfy it with a wide margin; the 120 s figure is what let a
completion retry through a 30 s outage without the lease expiring.

## What this does not show

- Cloud Run / Cloud SQL latency (Phase D; measured again at rs-16).
- Codex lanes (the bench is a `sirsi` binary; Codex lanes call the same binary).
- A third machine (Phase E, rs-21).
