<!-- agent: ra | workstream: router-service (ADR-062) | repo: sirsi-pantheon | date: 2026-09-15 | session: 6509a1af-6205-4a70-b158-8c3a4cb16b23 -->

# Ra — continuation: two router defects fixed (PR #761), M5 relay repaired live, inbox drained (2026-09-15)

Resume from THIS file only if you are `ra` on this workstream. Identity reuses thread
`thr-df2a8cd5b1c61290` (`--thread`, never mint). The record is reaped and lives with host
"MacBookPro"-era authority — `thread register` from this host answers "thread authority — own host";
gate mode is `log`, ledger mutations still land. Do not fight it; note it.

## How to reach the router from the M1 (cost 25 min today)

`~/.zshenv` sources `~/.sirsi/router-service.env` when `SIRSI_ROUTER_URL` is unset. Until today that
file carried an **https URL + a token minted for host "MacBookPro"**; with the kernel hostname at
`M1.local` every call 403'd. Rewritten (backup 0600 at `~/.sirsi/secrets/router-service.env.m5-token.bak-20260915`):

```sh
export SIRSI_ROUTER_URL='spool:///Users/sirsimasterdev/.sirsi/shared/relay'
export SIRSI_RELAY_TRUST_GROUP='_sirsipantheon'
```

plus `SIRSI_AGENT_ID=ra` in the working shell. Tests must run with all four unset
(`env -u SIRSI_ROUTER_URL -u SIRSI_ROUTER_TOKEN -u SIRSI_RELAY_TRUST_GROUP -u SIRSI_AGENT_ID go test …`)
or they hit the live store and fail on "thread authority".

## State at hand-off (verified unless marked)

- **PR #761** `fix/router-stall-gate-spool-fail-fast` (head `faf1672`, worktree
  `~/.sirsi/worktrees/fix-router-stall-gate`, from `origin/main` `a1f4804`): Lint / Secrets / binding-hold
  green; Build + Test were pending at 04:55Z. Merge on green (owner has been waiving the SSA bind for this
  rollout; SSA has no network — bundle if they must read it).
  - **D1 stall gate** (`internal/router/wake.go`): running consumer with no inbox-fingerprint change
    (ids + `acked_at`) for `wakeLoopConsumerStall` = 30 min → `terminateConsumer` (SIGTERM group, SIGKILL
    after `consumerKillGrace` 20 s) once, reason on the thread record, existing no-progress path dispatches
    one replacement. Test + negative control run.
  - **D2 spool** (`internal/routerstore/spool.go`): `refuseUntrustedClientOnTrustedSpool` (setgid spool +
    no trust group → error in ~50 ms, was a 30 s silent wait); `mkdirTrusted` converges a pre-existing 0700
    lane dir THIS uid owns; relay logs once per unreadable lane dir (`Relay.unreadable`);
    `routerServiceEnvXML` renders `SIRSI_RELAY_TRUST_GROUP` into lane plists.
  - **node-status** (`internal/router/nodestatus.go`): `store.Inbox("")` instead of `ListAll` (36 MB).
- **M5 live repair (ops, not a roll):** relay daemon plist lacked `SIRSI_RELAY_TRUST_GROUP` → added with
  `plutil -insert`, applied with **`bootout` + `bootstrap`** (`kickstart -k` keeps the OLD env — verified),
  backup `/var/sirsipantheon/relay-plist.bak-20260915`; `claude-io/` + `M5.local/` chmod 770; spool root
  now 0770; `SIRSI_AGENT_ID=claude-io` round-trip <10 s; 9 lanes untouched. M5 client still `0b2fd3da…`
  (main `1995046`) — **roll at the next M5 change**; until then `node-status`/`ctr` there still pay the
  36 MB ListAll (~30 s). `Mac/` works only via an ACL on that dir (pre-dates the trust group).
- **SSH:** `thekryptodragon@M5.local` works from the M1 (key enrolled); `sirsimasterdev@M5.local` and both
  Tailscale IPs do not. `sudo -n` works on both Macs.
- **Ledger (ra):** rs-40 (this fix) pending → claim-id + complete with PR #761 sha once merged;
  rs-41 (ownerless recovery: signed LAN anchor ADR, SHA 20260915-012036) pending, ADR not started;
  rs-36/37/39 unchanged (rs-39 closes when PR #759 merges; #759 checks green, needs the owner's word).
- **Inbox:** 0 open for `ra` at 04:52Z. 14 SHA items answered with evidence and closed (bodies in this
  session's scratchpad `replies/`, results on the store: `20260915-0450xx-ra-sirsi-hardware-admin-…`).

## Owner decision cards (surface, do not act)

1. **M1 hostname is unpinned and flaps** (`HostName: not set`; configd "setting hostname to" M1.local ↔
   MacBookPro at 13:47, 21:05 on 09-14 and 00:48 on 09-15). The relay's host token is for `M1.local`; a NEW
   session minted while the kernel says `MacBookPro` gets `403 token is for M1.local` (seen 00:52 EDT).
   Existing sessions keep working. Fix is one owner command: `sudo scutil --set HostName M1.local`
   (system setting — owner runs it). Same class fixed on the M5 by SHA ("host identity repaired").
2. **M1 SIP disabled + FileVault Off** after the 09-14 recovery (SHA 035826): Recovery OS `csrutil enable`,
   then `fdesetup enable`; stale Tailscale sysext via vendor uninstall/reboot/reinstall after an access-lane
   receipt.
3. **M5 SSH key enrollment** for ownerless remote recovery (SHA 212138): enroll the approved HA public key in
   `sirsimasterdev`'s `authorized_keys` from an authenticated M5 console.
4. **PR #759** (`--blocked-by` help) and **PR #758** still need the owner's merge word.

## On resume — in order

1. `gh pr checks 761` → merge on green → ledger: `task claim-id ra rs-40-… --thread thr-df2a8cd5b1c61290 --worker <id> --json`,
   then `task complete ra rs-40-… --lease <id> --result-ref @file` naming the merge sha.
2. Roll the M1 client (`341b7ae0…` → merged sha) and, at the next M5 change, the M5 client (recipe §4:
   build in a clean worktree → `.new` → atomic `mv`; kickstart only the 9 running lanes + the system relay).
3. rs-41: draft the ADR (signed LAN anchor; lanes = M1/M5 LAN SSH + three TB rails, each reported
   separately; verbs allowlisted; admissions recorded; no weakening of SSH/SIP/FileVault/Tailscale/Keychain)
   → route to SHA + SSA for review before any code.
4. rs-37 waits on the SHA + SSA ADR-065 verdict (unchanged).
5. Out of scope, noted: `Mac/res` on the M5 held a 36 MB timed-out `ListAll` response (relay sweep reclaims);
   `~/.sirsi/logs/router-relay.log` on both Macs has stale gui-side "spool is a symlink; refusing" lines
   from a non-daemon invocation; `sirsi-menubar` thread got reaped ("pid recycled") on the M1 at 00:2xZ.
