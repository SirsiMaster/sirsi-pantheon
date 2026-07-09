# Continuation Prompt — INDEX (read this, then open YOUR thread's file)

> ⚠️ This file is a **router**, not a continuation. A single shared continuation collides:
> every thread overwrites it and a resumer can load the _wrong_ one. Real continuations live
> per-thread in **`docs/continuations/<agent>-<workstream>-<date>-<session8>.md`**.
>
> **To resume:** find the row whose `agent` + `workstream` match who you are, open the file at
> the exact path, and follow it. Do NOT act on a row that isn't yours.

## Active continuations

| Agent             | Workstream                                                                                                                                                                                                                                      | Date       | Exact path                                                                                                                             |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `claude-home`     | router-conduit (WRAP-UP 2026-07-09 — thread archived; work moved to the ROUTER CONDUIT SUPERVISOR. Open: codex #168 acceptance-bar verify [blocked on Codex app] · session-reaper duty at claude-pantheon · 2 owner-gated items · Ma'at fast-path unverified)                     | 2026-07-09 | `/Users/thekryptodragon/Development/sirsi-pantheon/docs/continuations/claude-home-router-conduit-20260709-40d9d244.md` |
| `claude-pantheon` | incident-canon-phase2 (✅ 2026-07-04/05 #162-#168 ALL MERGED: incident canon + ADR-035 + Sekhmet + Router-v2 Phase 2 AND Phase 3 — one dispatch facade; worker OFF/quarantined; next: Phase 4 migration+cutover) | 2026-07-05 | `/Users/thekryptodragon/Development/sirsi-pantheon/docs/continuations/claude-pantheon-incident-canon-phase2-20260704-d8b52186.md` |
| `claude-pantheon` | flaky-router-ack-gitdir (✅ PR #99 opened + Ma'at gate PASSED; test-only GIT_* env isolation fix for TestRouterAckLegacyPending; next: get CI green + merge #99)                                                                                  | 2026-06-29 | `/Users/thekryptodragon/Development/sirsi-pantheon/docs/continuations/claude-pantheon-flaky-router-ack-gitdir-20260629-e008ddba.md`    |
| `claude-nexus`    | SirsiNexusApp live-product security-hardening + prod-deploy (✅ incident CLOSED+DEPLOYED all 3 surfaces; #94/#95 merged + seedHardcodedAccounts purged from prod 403→404; only #93 Dockerfile open; watcher PID 77115; zero open security debt) | 2026-06-22 | `/Users/thekryptodragon/Development/SirsiNexusApp/docs/continuations/claude-nexus-security-hardening-prod-deploy-20260622-40d9d244.md` |

## Naming convention (so threads never collide)

```
docs/continuations/<agent>-<workstream-slug>-<YYYYMMDD>-<session8>.md
                    │         │                 │          └ first 8 of the session UUID (uniqueness)
                    │         │                 └ date the continuation was written
                    │         └ short workstream slug (what this thread is doing)
                    └ agent id (claude-pantheon, codex-pantheon, …)
```

When you write a new continuation: create the per-thread file above, then add/replace your row
in this table (one row per agent+workstream; keep the latest). The `<session8>` makes two same-day
runs of the same workstream distinct. Never write the full continuation into this index file.
