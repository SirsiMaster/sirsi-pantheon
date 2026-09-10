# ADR-062 rs-20 — M5 cut-over to the router service: evidence (2026-09-10, UTC)

Executed by Ra (session 84ab1eaa, M1) from `scripts/router-service/cutover-m5.sh`, owner-authorized (rs-19: 1a unattended, 2a all lanes; merge of #711 on owner authority 6492a65a).

## Snapshot and import (G4)
| Fact | Value |
|---|---|
| M5 freeze | 2026-09-10T01:59Z: `PRAGMA wal_checkpoint(TRUNCATE); journal_mode=DELETE; chmod a-w ~/.sirsi/router.db`; binary swapped to main 6492a65a |
| Snapshot file | `~/.sirsi/cutover/snap-20260910T015918Z.db` on the M1, sha256 `bd9ffbe30673e8d1d67d8208992e542868ac8ab1dd0c7679816eafaeee5daf38`, schema 16→18, 753 open / 5717 closed |
| Import #1 | Cloud Run execution `sirsi-router-migrate-v94gq`: ROLLED BACK by the transactional gate — destination `86dd19062607f39f5d9492f196876cd717d63a353258260ca99489f8506a9ff4` ≠ source; 6,422 pre-existing keys (rehearsal rows of 2026-09-08) named in the report |
| Destination reset | `sirsi-router-psql-sj98z`: TRUNCATE of all 14 router tables; items 0, schema_version 18 |
| Import #2 | `sirsi-router-migrate-gpld5`: source `e69c7de33974261f31366e41d0f1c81b7551c56c5e0929bd6ecf5328cab91ead` == destination `e69c7de33974261f31366e41d0f1c81b7551c56c5e0929bd6ecf5328cab91ead` == source-after; wrote items 6470, tasks 888, wake_events 5864, send_quota 1481, threads 2, state 2, breakers 2, counters 1; 7 trigger-minted extras removed; container `sha256sum /data/src.db` == local snapshot sha256 |
| Image | `sirsi-router-migrate:20260910T021020Z` (and `:20260910T020154Z` from import #1) — deletion owed to the owner (repoAdmin) |

## Service (G6)
Revision `sirsi-router-00007-dqm` (boot-time store-open retry), image `sha256:a4bd2c1881dcb099e8f875faee95fbe35c46d971dce4ef4e7e3ca0b15b1992e4`, SPKI pin `78rPvnhm1Lb3jziI2hDDogyku5XoaVABHemUnWwOd7M=`. Tokens minted for hosts `Mac` (execution `sirsi-router-token-9vxx4`) and `MacBookPro` (`sirsi-router-token-zkzmw`); all pre-cut-over tokens gone with the TRUNCATE.

## Both hosts on one ledger (G7)
Same-minute observations, each host running `sirsi router status` through its own token:
```
M1 MacBookPro 2026-09-10T02:49:20Z   Items: 658 open, 5814 closed
M5 Mac        2026-09-10T02:49:21Z   Items: 658 open, 5814 closed
```
(Counts moved from the frozen 753/5717 because the SSA lane was closing its backlog at the time — the ledger is live.)
Writers proven: Claude lane on the M1 (Ra: rs-18/19/20 closed via the service, no ssh); M5 write (`task reclaim-expired`) at 02:2xZ; **Codex lane on the M5**: `sirsi-software-admin` wake loop (pid 51686) dispatched `codex exec … -c sandbox_workspace_write.network_access=true`, which claimed and closed router items through the service (inbox 108 → 107 at 02:47Z, then 751 → 658 open fleet-wide by 02:49Z) and wrote response item `20260910-024658`.
Boundary: codex sandboxes have no DNS, so codex lanes reach the service only with the owner's network exception (2026-09-10: SSA lane only) until the unix-socket relay (rs-22) lands. No codex runtime is installed on the M1. G7 is therefore closed for Claude lanes on both hosts and for the codex lane the owner authorized; the remaining codex lanes are gated on rs-22, not on the cut-over.

## Rollback rehearsal (G8), node-local on the M5, timed
```
G8 rehearsal #2 on Mac 2026-09-10T02:50:03Z: data dump sha 7aa0444cdf1c4125
rolled back in .030 s   (env file out, chmod u+w, journal_mode=wal, previous binary)   local: Items: 753 open, 5717 closed
re-forwarded in .566 s  (env back, checkpoint + journal_mode=DELETE + chmod a-w, main binary)
window .596 s; data dump sha after 7aa0444cdf1c4125 (IDENTICAL — no data loss); service: 658 open, 5814 closed
```
(Rehearsal #1 at 02:49:43Z showed the same counts; its raw-file sha differed only because the journal-mode toggle rewrites two header bytes, hence the dump-level hash in #2.)

## Horus (G10 partial)
`ai.sirsi.horus.agent-router` repointed from a vanished `/private/tmp/...` build to `~/.local/bin/sirsi` (owner decision), restarted 02:22Z with `SIRSI_ROUTER_URL` in its environment; read-only-database errors in its err log all predate the restart. Board/menubar verification remains rs-22.

## Guards added after the run (PR #713)
Step 4 refuses when `~/.sirsi/cutover/activated` exists or the destination holds any host token (`RAISE EXCEPTION` inside the psql job); step 7 writes the activation marker. Wake plists are written privately (0600, temp + rename) and tightened on the idempotent path.
