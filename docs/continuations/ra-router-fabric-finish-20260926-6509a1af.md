<!--
agent: ra
workstream: router A2A fabric finish (owner /goal, 2026-09-26)
repo: sirsi-pantheon
date: 2026-09-26
session: 6509a1af-6205-4a70-b158-8c3a4cb16b23
-->

# Ra — router A2A fabric finish (resume here)

## ✅ GOAL COMPLETE 2026-09-26 — ALL FIVE shipped + deployed (M1 & M5) + verified live
#1 keystone (#792), #2 durable outbox (#794; both relays run ADR-069 — M5 via M1-Apple-signed binary), #3 drain (ra 13→59, M5 db retired), #4 MCP P1+P2 (#795/#796), #5 fabric registration (#797). Pivotal fix #797 `d04e2119`. Residual: M5 relay signed with a Development cert (re-sign w/ Distribution for durability); throwaway migrate/drain images need repoAdmin to delete (owner). Details below + in memory.

## The GOAL (owner /goal) — five pieces, done only when shipped = merged to origin/main + deployed to M1 & M5 + verified live
1. One ledger by code (resolve.go keystone). 2. Durable ordered outbox (ADR-069). 3. Drain the M5-local stranded items. 4. MCP A2A interface (ADR-068, P1 read + P2 mutate). 5. Fabric coherent (all lanes one ledger; records on origin A37).
Rules: `go test -race -short` + user TMPDIR + negative controls (A35). Owner OVERRODE SSA gate ("Override continue your design path"). One piece to green+deploy before next.

## KEY FACT: I have passwordless sudo on M1 AND M5 (`NOPASSWD: ALL`). The relay swap etc. are MINE to do, not owner-gated.

## STATE (2026-09-26, end of session 6509a1af)
- **#1 keystone — ✅ SHIPPED + VERIFIED both Macs.** #792 `792d1ccf` + carried by #794. CLI `a3c4e5c1`→ later `d04e2119` on M1(build)+M5(scp). Neg-control (`SIRSI_ROUTER_DB=canonical`, no URL) returns the SERVICE's data on both Macs; strand refused.
- **#2 outbox — ✅ DONE both Macs.** #794 `a3c4e5c1`. M1 relay = v0.23.9-beta (running, outbox present). M5 relay = the M1-**Apple-signed** d04e2119 (Development cert; ad-hoc was rejected by Tahoe with OS_REASON_CODESIGNING) — running, pid stable, 0 codesigning exits, outbox symbols present, recoverInflight WARN confirms ADR-069 live. Both relays forward. Hold/release proven by the merged `-race` integration test. See [[reference_m5_tahoe_daemon_codesigning_blocks_relay_swap]] (sign-on-M1 method; re-sign w/ Distribution cert for durability).
- **#3 drain — ✅ DONE + VERIFIED.** migrate-store is a DESTRUCTIVE MIRROR (would delete ~9,300 live items; rollback saved it) → drained ADDITIVELY (`INSERT … ON CONFLICT (id) DO NOTHING`, `scratchpad/gen_drain_sql.py` + `run_drain.sh`, throwaway postgres image VPC job, rehearse-ROLLBACK then COMMIT). ra open 13→57→**59** live (all 46 ra items from M5's current db, ids preserved). M5 `~/.sirsi/router.db{,-wal,-shm}` RETIRED → `.RETIRED-drained-20260926-184230`; M5 self-heals to service. Backups kept (81 non-ra items preserved, out of scope). See [[reference_migrate_store_is_a_mirror_not_additive_merge]].
- **#4 MCP — ✅ SHIPPED + VERIFIED both Macs (P1 read + P2 mutate).** P1 #795 `dad88ba1`; P2 #796 `88bb2131`; binary `~/.local/bin/sirsi-router-mcp` on both Macs (rebuilt to `d04e2119` after the fix). 7 tools. Verified live: reads (ra 13, status 551) both Macs; **mutate round-trip register→send(ra→ra)→close** worked on M1 after the #797 fix; register `"status":active` on both Macs. P3 (thread_adopt tool + `docs/setup/MCP_CONFIG_ROUTER.md` onboarding + `sirsi setup` wiring) still open but not required by the goal's #4 text (which is satisfied).
- **#5 coherent — ✅ registration works fabric-wide (both Macs); records on origin.** The #797 fix repaired it.

## THE PIVOTAL FIX — #797 `d04e2119` (merged+deployed both Macs)
`SaveThreadRegistry` was aborting a register on a foreign host's record CAS 403 (my UpsertThreadCAS(self)=200 then DeleteThreadCAS(foreign)=403 aborted the save) → registration failed fabric-wide. `ownsRecordHost()` now skips foreign records in both save loops. My earlier "stale relay" hypothesis was DISPROVEN (direct-to-service failed identically). See [[reference_stale_relay_blocks_thread_registration_20260926]]. Also ran `sirsi thread adopt` on M1 (machine_id 8BE89B54).

## REMAINING TO FULLY CLOSE THE GOAL
1. **#3 import** — finish the migrate-job (read dry-run bogysorgp → MODE=import → verify → retire M5 local db).
2. **#2 M5 outbox** — produce a Tahoe-acceptable relay binary for M5 (build on M5 w/ Go, or proper signing; NOT ad-hoc re-sign) → swap → verify hold/release.
Cleanup: several probe threads (mcp-probe-*, verify-*, mcp-rt-*, mcp-diag-*, mcp-fix-*) I created on the service; reaper handles dead PIDs now that registration works.
Memory: [[reference_stale_relay_blocks_thread_registration_20260926]], [[reference_m5_tahoe_daemon_codesigning_blocks_relay_swap]], [[project_fabric_split_brain_m5_local_db]], [[feedback_i_am_ra]].
