# Router M0 baseline (Ra charter, measured 2026-10-11 from M1)

Method: pre-registered before measuring (`PREREG` text is reproduced at the end); n and nothing dropped are stated per series.
Instruments: `m0_01_read_path.py.txt` (this directory; rename to .py to run) and `m0_02_detect.py.txt` (wake-latency estimator).

## Where one unit of latency goes (p50)
| Stage | Latency | Source |
|---|---|---|
| Wake loop notices a new item (lane with a wake loop) | **~30-37 s**, p95 ~60 s | M0-02, 1,500+ item/loop events on M1, 9 lanes |
| CLI read through the spool (`router pull`) | 420 ms, p99 668 ms | M0-01, n=30 |
| of which: process start | 74 ms | M0-01 |
| of which: HTTPS first byte to Cloud Run (fresh TLS) | 88 ms, p99 261 ms | M0-01, n=30 |
| of which: two 150 ms spool polls (estimate, H3 untested) | ~150 ms | `spoolPoll` in spool.go |
| Full-ledger read | **FAILED at 64 MiB** before 2026-10-11; fixed by `ListPage` (#1075), 17,803 items in 24.7 s | this doc |

**Conclusion:** the dominant delivery wall is the 60 s wake-loop tick (`DefaultWakeLoopInterval`, canon A27), about 70x the
whole spool read. The 150 ms spool polls are real but second-order; do not tune them until the tick is addressed.

## Findings
1. **M0-02 (wake detection) CONFIRMED the pre-registered H1** (median >= 25 s): medians 29.8-37.3 s per lane, p95 57.7-66 s, which is
   what a 60 s fixed tick predicts (uniform 0-60 s wait, median 30 s). Caveat: estimator matches each depth-increase log line to the oldest item
   opened in the previous 75 s, so the max is truncated at 75 s; claude-io (n=7) is a small sample. Wake-loop logs of M1 lanes only; the M5 codex lanes not yet measured.
2. **Capacity wall found by accident:** the full-ledger read returns one response that grew with the ledger (~3.7 KB per item x 17.6k items) and
   crossed the spool's 64 MiB response ceiling, breaking `router dump`, `sirsi-bind.sh`, and the Gemma triage script. Fixed: cursor-paged read,
   `ListPageMax` 2000, service revision sirsi-router-00025-mt7. Proof: v0.24.100 client still fails on the same path; the fixed client returns the ledger.
3. **M0-01 H1 FAILED by 32 ms** (my budget omitted the 74 ms process-start term). Recorded, not re-registered after the fact.
4. **H3 (polling dominates the spool path) is still untested.** Next experiment must isolate it, but it is now second priority.

## Options for the 60 s tick (cost is the constraint; GCP spend is currently unreadable, see funding law)
| Option | Latency | Cost | Risk |
|---|---|---|---|
| A. Keep 60 s tick, add a sender-side local nudge (`NotifyAgent`/`NotifyPath` already exist) so same-host sends wake the lane instantly | same-host: sub-second; cross-host: unchanged | none | cross-host items still wait up to 60 s |
| B. One long-poll `Wait` per HOST (3 connections), host agent fans out to lanes | sub-second everywhere | a held request keeps a Cloud Run instance warm: ~always-on instance cost, NOT yet quantified | needs funded-envelope admission; Cloud Run request timeout 90 s means re-arm loop |
| C. Shorten the tick to 5-10 s | 2.5-5 s median | N lanes x requests/min: 23 lanes x 6-12/min | token-free but request-count cost and A27 canon change |
Recommended order: A first (free, reversible, measurable), then quantify B's instance cost once GCP billing is readable, then decide B vs C.

## Pre-registration text (M0-01, amended once before any measurement)
H1: spool-path p50 <= https floor p50 + spool poll + 150 ms. H2: p99 <= 3 x p50. H3: poll dominates the gap. Negative control: bogus host fails 5/5.
M0-02: H1 median detect >= 25 s on a 1-minute-tick lane; H2 attended/event-woken lane acks within 2 s (not yet measured).

## Finding 5 (2026-10-15): the session table grew ~86k rows in 48 h because two binaries shared one cache file
Measured on the M5: agent `Mac` minted 17 sessions in ~30 s; its cache file alternated between runtime hashes of the current CLI
(`34b5ce2b`, 11 wake loops) and the older relay binary (`8ea0cfc3`, 8 wake loops). A cached session is valid only for the executable that
minted it, so the two invalidated each other every call. All 19 loops authenticate as the hostname because a wake loop has no agent id
(audit log: `Mac@Mac` 86,449 sessions / 48 h). Fixes: the cache file is keyed by runtime (20 calls from 2 binaries now mint 2 sessions, was
20; negative control reproduces 20), stale cache files are pruned after 7 days, and the service garbage-collects dead sessions (idle or revoked
> 24 h, never one owning a lease on an open item or unfinished task; amortized on mint, bounded batches). Operator tools: `sirsi router sessions
stats|gc [--dry-run]`. Still open: wake loops should authenticate as their own lane (attribution), tracked as ra-wake-loop-identity.
Pre-registration: `PREREG_SESSIONS_01.md`.
