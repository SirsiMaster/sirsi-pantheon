# ADR-062 step 20a.5 — Codex lanes claim and close through the spool with no network and no token (2026-09-10T05:25Z–05:45Z, M5)

Registry at this head: `codex-deck`, `codex-finalwishes`, `codex-inference`, `codex-pantheon`, `sirsi-software-admin` run
`env -u SIRSI_ROUTER_TOKEN SIRSI_ROUTER_URL=spool:///Users/thekryptodragon/.sirsi/relay SIRSI_AGENT_ID=<lane> codex exec --sandbox workspace-write …`
(each codex lane already declared `~/.sirsi` via `--add-dir`; the SSA lane declares the spool as its writable root); `network_access` appears nowhere.
Relay: `ai.sirsi.router.relay` under launchd on the M5 (0600 plist; the host's only token holder). Wake loops installed with `sirsi router wake-install` (plists 0600).

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

## What this proves / does not prove
Proves: five Codex lanes on the M5 claim tasks and close items on the router service with no network access and no token in their environment; the SSA lane's network exception is gone from the registry. Author-reported; the relay log and the ledger are the durable receipts (`sirsi router dump` is re-runnable by any reviewer).
Does not prove: a Codex lane on the M1 (none installed — 20a.6 owner gate), G8, full Bind #4.
