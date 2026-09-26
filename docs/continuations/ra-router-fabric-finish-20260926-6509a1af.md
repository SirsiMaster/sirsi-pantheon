<!--
agent: ra
workstream: router A2A fabric finish (owner /goal, 2026-09-26)
repo: sirsi-pantheon
date: 2026-09-26
session: 6509a1af-6205-4a70-b158-8c3a4cb16b23
-->

# Ra — router A2A fabric finish (resume here)

## The GOAL (owner /goal, "Override continue your design path")
Finish the router A2A fabric to a taggable release (fabric only, NOT the public CLI). Done only when ALL FIVE are **shipped = merged to origin/main + deployed to M1 & M5 + verified live**:
1. **One ledger by code** (resolve.go keystone).
2. **Durable ordered outbox** (ADR-069) — held in relay `outbox/`, released in order.
3. **Drain the M5-local stranded items** into the service (idempotent, hash-verified); retire M5 local db.
4. **MCP A2A interface** (ADR-068) — `cmd/sirsi-router-mcp` P1 read-only + P2 mutate behind surface=mcp thread + onboarding doc.
5. **Fabric coherent** — all lanes on one ledger; records on origin/main (A37); memory+continuation current.
Gates: `go test -race -short` + user TMPDIR + neg-controls. SSA review OVERRIDDEN by owner ("Override continue"). One piece to green PR+deploy before the next.

## STATE
- **#1 keystone — ✅ SHIPPED + VERIFIED both Macs.** PR #792 `792d1ccf` merged. Lane CLI rebuilt from main `a3c4e5c1` (VERSION 0.23.9-beta), installed `~/.local/bin/sirsi` on M1 (go build) + M5 (scp `thekryptodragon@m5`, no Go there). Verified: normal shell vs `env -u SIRSI_ROUTER_URL -u SIRSI_ROUTER_DB` vs neg-control `SIRSI_ROUTER_DB=~/.sirsi/router.db` ALL return the same service data (13 open for ra) on BOTH Macs. Marker `~/.sirsi/router-service.env` recovers `spool:///var/sirsipantheon/relay`. M5's canonical db is readable again (chmod-000 reverted) so the keystone is the sole enforcement there — and it holds.
- **#2 outbox — ✅ M1 LIVE; ⛔ M5 blocked on APPLE SIGNING (not sudo — I have sudo).** #794 `a3c4e5c1`. M1 relay swapped to v0.23.9-beta (ADR-069 outbox), running/forwarding. **M5:** Tahoe (macOS 27) rejects an ad-hoc daemon binary (`OS_REASON_CODESIGNING`); needs a real Apple signature. M5 HAS Sirsi certs (Team 9D382WV988) but `codesign` over SSH fails `errSecInternalComponent` (login keychain locked in the SSH session). GATE: owner unlocks the keychain / signs from their GUI session / hands a signed binary; then rm-swap `/usr/local/libexec/sirsi-pantheon/sirsi` + `launchctl kickstart -k`. See [[reference_m5_tahoe_daemon_codesigning_blocks_relay_swap]].
- **#3 drain — ✅ DONE + VERIFIED.** migrate-store is a DESTRUCTIVE MIRROR (would have deleted ~9,300 live items; its rollback saved it) — see [[reference_migrate_store_is_a_mirror_not_additive_merge]]. Drained instead via an ADDITIVE `INSERT … ON CONFLICT (id) DO NOTHING` (generator `scratchpad/gen_drain_sql.py` + `run_drain.sh`, throwaway postgres:16-alpine VPC job, `psql -f`, rehearse-with-ROLLBACK then COMMIT). ra open 13→57→**59** live (all 46 ra items from M5's current db, ids preserved). M5 `~/.sirsi/router.db{,-wal,-shm}` RETIRED → `.RETIRED-drained-20260926-184230`; canonical path absent; M5 self-heals to service (neg-control too). Backups kept (81 non-ra items preserved, out of scope). Drain job deleted; throwaway images need repoAdmin to delete (owner).
- **#4 MCP — spec only (ADR-068 / PR #786 open).** Build `cmd/sirsi-router-mcp` mirroring `cmd/sirsi-gemma` + `mcp.NewBareServer` + `dispatch.Facade`. Not started.
- **#5 coherent — partial.** Keystone closed the split-brain by code. 32 lanes broadcast the ledger location. 4 orphan recipients declared (now with consumers). Records on origin. Continuation+memory current.

## OWNER GATE (walk through, one action) — deploy the #2 outbox relay
The relay binary is root:wheel; only the owner can swap it. On EACH Mac (M1 then M5), after the new lane CLI is in place:
```
sudo cp ~/.local/bin/sirsi /usr/local/libexec/sirsi-pantheon/sirsi   # rm-first if it SIGKILLs: sudo rm first
sudo launchctl kickstart -k system/ai.sirsi.router.relay
```
(M5: the binary is already at `/tmp/sirsi-a3c4e5c1` from the scp — `sudo cp /tmp/sirsi-a3c4e5c1 ...`.) Then verify: send a test item while the cloud/relay path is down, confirm it lands in `<spool>/<lane>/outbox/` and is released in order on recovery (not dropped).

## RESUME SEQUENCE
1. Get the owner to run the two sudo lines on both Macs → verify outbox hold/release live → #2 shipped.
2. #3: compare the 44 M5-local ra items against the service; drain only the absent ones (VPC migrate-store, hash-verified); retire M5 local db.
3. #4: build `cmd/sirsi-router-mcp` (P1→P2) → green PR → merge → deploy → onboarding doc.
4. #5: audit coherence, tag the router-fabric release.
Memory: `project_fabric_split_brain_m5_local_db`, `project_rs42_credentialed_machineid_adoption`, `feedback_i_am_ra`.
