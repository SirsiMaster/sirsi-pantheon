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
- **#2 outbox — MERGED + CLIENT-deployed; RELAY runtime BLOCKED on owner sudo.** PR #794 `a3c4e5c1` merged (CI CLEAN; fixed my own `TestRegistryConsumerCoverage` regression `df9d9759` — the 3 orphan agents needed the canonical consumer block). Outbox logic (`neverReachedService`/`statusHoldForRetry`/`drainOutbox`/`scheduleOutboxRewake` in `internal/routerstore/spool.go`) runs ONLY in the **relay daemon** = `/usr/local/libexec/sirsi-pantheon/sirsi` (root:wheel, launched by `/Library/LaunchDaemons/ai.sirsi.router.relay.plist` as `_sirsipantheon`). Swapping it + `launchctl kickstart -k` needs **sudo** → owner gate. The lane CLI carries only the client side (writes req/, reads res/), which is a no-op without the new relay. **#2 is NOT functionally live until the relay binary is redeployed on both Macs.** Exact commands below.
- **#3 drain — NOT started.** M5 backups `~/.sirsi/router.db.STRANDED-backup-20260926-100722` (+ backup2) = v16, 44 ra items / 125 total. NOTE: with the keystone live, must first check overlap — the service already has 13 open for ra; determine which of the 44 are genuinely absent from the service before draining (idempotent by id). Drain = migrate v16→v23 then `sirsi router migrate-store --from <db> --to <cloudsql DSN>` as a VPC Cloud Run job.
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
