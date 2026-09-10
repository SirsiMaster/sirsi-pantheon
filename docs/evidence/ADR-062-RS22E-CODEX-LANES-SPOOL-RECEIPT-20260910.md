# ADR-062 step 20a.5 — four Codex lanes claim and close through the spool without a token in the consumer (2026-09-10T05:25Z–05:45Z, M5); codex-inference reads/updates after the 14:43Z fix

Registry at this head: `codex-deck`, `codex-finalwishes`, `codex-inference`, `codex-pantheon`, `sirsi-software-admin` run
`env -u SIRSI_ROUTER_TOKEN SIRSI_ROUTER_URL=spool:///Users/thekryptodragon/.sirsi/relay SIRSI_AGENT_ID=<lane> codex exec --sandbox workspace-write …`
(each codex lane already declared `~/.sirsi` via `--add-dir`; the SSA lane declares the spool as its writable root); `network_access` appears nowhere.
Relay: `ai.sirsi.router.relay` under launchd on the M5 (0600 plist). **Token boundary, exactly:** the relay's plist and each authorized wake-loop plist (0600) hold the host token — the loops need it to read their inboxes; every codex consumer process is started with `env -u SIRSI_ROUTER_TOKEN` and is expected to hold none, subject to shell startup not re-adding it (see the codex-inference finding and PR #723). The token was observed absent inside exactly one consumer sandbox: codex-inference at 2026-09-10T14:43Z (`tokenset=` empty); that observation is not generalized to the other lanes here. Wake loops installed with `sirsi router wake-install` (plists 0600).

## Relay log — mutating methods by codex lane (05:25Z–05:45Z), `grep 'relay: forwarded' ~/.sirsi/logs/router-relay.log`
```
  27 agent=codex-deck        method=CloseItem
  26 agent=codex-pantheon    method=ClaimTask
  23 agent=codex-finalwishes method=ClaimTask
  22 agent=codex-finalwishes method=CompleteTaskLease
  22 agent=codex-finalwishes method=CloseItem
  22 agent=codex-deck        method=SendGuarded
  17 agent=codex-pantheon    method=CloseItem
  16 agent=codex-pantheon    method=CompleteTaskLease
  11 agent=codex-deck        method=UpdateTask
   7 agent=sirsi-software-admin method=UpdateTask
```
Token occurrences in the relay log: 0. Spool lane directories present: `codex-deck`, `codex-finalwishes`, `codex-pantheon`, `sirsi-software-admin` (+ `Mac` from the first minutes before `SIRSI_AGENT_ID` was set).

## Ledger — items closed since 05:25Z by recipient (`sirsi router dump`, read from the M1 over HTTPS)
```
codex-deck 27   codex-finalwishes 22   codex-pantheon 17   claude-deck 21   claude-finalwishes 12   sirsi-software-admin 2
first: codex-finalwishes 20260827-123718-…-response-finalw… closed 2026-09-10T05:34:48Z
       codex-pantheon    20260827-123718-…-response-guard-sui… closed 2026-09-10T05:38:07Z
       codex-deck        20260902-181446-…-response-fallback-adversarial-r… closed 2026-09-10T05:34:05Z
```
Fleet total: 172 open / 6391 closed at 05:17Z → 107 open / 6497 closed at 05:45Z.

## What this proves / does not prove (conclusion of the 05:25Z–05:45Z window — superseded below for codex-inference)
Proves for **four** codex lanes on the M5 — codex-deck, codex-finalwishes, codex-pantheon, sirsi-software-admin — that they claimed tasks and closed items on the router service through the spool, with consumers started without a token and no `network_access` in their argv, in the window 05:25Z–05:45Z. ~~Proves: five Codex lanes …~~ — **retired 14:5xZ**: codex-inference made no router call in that window (see below); it is not part of this proof. Author-reported; the relay log and the ledger are the durable receipts (`sirsi router dump` is re-runnable by any reviewer).
Does not prove: codex-inference claim/close (its receipt is reads/updates after the 14:43Z fix); a Codex lane on the M1 (none installed — 20a.6 owner gate); G8; full Bind #4; fleet-wide token isolation beyond the one sandbox observation named above.

## Per-lane correlated receipts (collected 2026-09-10T14:41Z and 14:46Z from the M5 logs; credentials never printed)
Effective consumer for every codex lane (from each lane's first `dispatched consumer` wake-log line): `env -u SIRSI_ROUTER_TOKEN SIRSI_ROUTER_URL=spool:///Users/thekryptodragon/.sirsi/relay [SIRSI_AGENT_ID=<lane>] codex exec -C <repo> --sandbox workspace-write --add-dir /Users/thekryptodragon/.sirsi --ephemeral <prompt>` (the SSA lane: `… codex exec --sandbox workspace-write -c sandbox_workspace_write.writable_roots=["…/.sirsi/relay"] --skip-git-repo-check -C ~`). Token-presence boolean: the wake **loop** plist carries the token (needed by the loop itself); the **consumer** argv begins `env -u SIRSI_ROUTER_TOKEN`, and a live probe inside a consumer shell printed `tokenset=` (empty) with `url=spool://…` (see codex-inference below). `network_access` appears in no consumer argv. Relay log token occurrences: 0.

| lane | dispatches | relay methods (count) | spool dir |
|---|---|---|---|
| codex-deck | 8 | Get 51, ListThreads 39, ListTasks 36, Render 32, **CloseItem 31**, **SendGuarded 25**, UpsertThreadCAS 18, **UpdateTask 17** | drwx------ |
| codex-finalwishes | 4 | Render 27, Get 24, **ClaimTask 24**, **CompleteTaskLease 23**, **CloseItem 23**, **AddTask 23**, Inbox 16, ListTasks 13 | drwx------ |
| codex-pantheon | 2 | Get 45, Render 37, **ClaimTask 36**, **CloseItem 30**, **AddTask 30**, **CompleteTaskLease 28**, **SendGuarded 18**, GetTask 18 | drwx------ |
| sirsi-software-admin | 21 | **ClaimTask 18**, **UpdateTask 17**, ListTasks 17, GetTask 17, Get 16, Render 14, **SendGuarded 9**, ListAll 9 | drwx------ |
| codex-inference | 8 (+2 after fix) | before 14:43Z: **none**; after: Render 87, ListThreads 11, ListTasks 6, Inbox 5, ListAll 3, **UpdateTask 2**, GetTask 2, UpsertThreadCAS 1 | drwx------ |

**codex-inference defect and fix.** Its 8 dispatches made zero relay calls and the loop logged `consumer made NO PROGRESS` with backoff. Cause: codex runs every command through a login shell, and `~/.zshenv` sourced `router-service.env` unconditionally, handing the https URL and the host token back to a lane that had chosen the spool and cleared the token; its sandbox has no DNS, so every router call failed. Fix: the source line runs only when `SIRSI_ROUTER_URL` is unset or empty — applied on both Macs by hand (reported manual action, this author); the durable, fixture-backed repair for already-cut-over hosts (marked managed line replaced, legacy/repeat fixtures) is PR #723 at head 051fcfde or later. Receipt inside the lane's own sandbox after the fix: `sirsi router pull codex-inference` → `72 open items for codex-inference: …`, `url=spool:///Users/thekryptodragon/.sirsi/relay tokenset=` (empty). Its loop was kickstarted at 14:43Z and the relay methods above followed. The same `--add-dir ~/.sirsi` shape is what made the other codex lanes' spool writes possible; why their early runs were not hit by the same override is not established (they may have started their first commands before re-sourcing), so the host-level fix is the durable guarantee for all of them.

Claim, exactly: codex-deck, codex-finalwishes, codex-pantheon and sirsi-software-admin claimed/closed/sent through the spool (05:25Z–05:45Z) with consumers started without a token and no `network_access`; codex-inference reads and updates tasks through the spool after the 14:43Z fix, with the token observed absent inside its sandbox at that time; its claims/closes are not yet demonstrated. No lane on the M1 (owner gate 20a.6). Per-lane counts other than the SSA's own are author-reported log summaries, not per-request lineage reconstructed by a reviewer.
