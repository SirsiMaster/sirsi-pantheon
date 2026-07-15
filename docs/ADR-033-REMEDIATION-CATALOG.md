# ADR-033 — Remediation Catalog: every finding maps to a real macOS lever (never a monitor)

**Status:** Accepted · 2026-06-30 · claude-pantheon (owner-directed: "solve this once and for all")
**Governs:** the monitor→identify→**fix** loop — the app's core value.

---

## Problem

Monitoring and identifying were built; **fixing was fake for a whole class of findings.**
Tapping "Relieve the live cause" on a memory finding ran `sirsi guard` — which is the
**monitor** (aliased to `sirsi monitor`) — so it opened a read-only dashboard, not an action.
A monitor that identifies a problem but cannot fix it is half a product.

## Decision — the Three-Outcome Law

Every finding that can alarm MUST resolve to exactly one of three honest outcomes.
**A passive monitor/dashboard is never a "fix".**

1. **ACTION** — a real macOS lever runs, then **measures before/after and proves the result**, then **re-verifies** the finding. Safe levers run on confirm; privileged levers use the macOS **native admin-auth dialog** (the password goes to the OS, never to sirsi); destructive levers preview first (Rule A1).
2. **GUIDANCE** — when no safe automatic lever exists (a user app hogging RAM, a kernel panic), **name the cause** and give the exact manual step. Honest, actionable, never a dead-end.
3. **INFO** — historical/no-current-action (7-day trends): plain, no alarm color, no fix button (ADR precedent: `demoteTrendsToInfo`, `gateSwapOnPressure`).

## The macOS lever catalog (verified available on macOS 15/Darwin 25)

| Resource | Lever | Command | Privilege | Reversible | Frees / fixes |
|---|---|---|---|---|---|
| **Memory** | Flush inactive caches | `/usr/sbin/purge` | admin | yes (re-caches) | relieves pressure, returns cached pages to free pool |
| Memory | Name the hog | `ps -axo rss,comm` / `top -o mem` / `footprint` | user | — | identifies top consumer (the "identify" half) |
| Memory | Suspend a runaway | `kill -STOP`/`-CONT` (SIGSTOP/CONT) | user (own) | yes | halts a hog, frees its active RAM (Hapi for governed) |
| Memory | Quit an app | `osascript -e 'quit app "X"'` | user | yes | graceful free of a user app's RAM |
| **CPU** | Lower priority | `renice` + `taskpolicy -b` | user | yes | hands cycles back to the foreground app |
| **Disk** | Trash regenerable | `sirsi clean` (trash-first) | user | yes | reclaims caches/build artifacts |
| Disk | Thin APFS local snapshots | `tmutil thinlocalsnapshots /` | admin | no | reclaims Time-Machine local-snapshot space (often GBs) |
| **Spotlight** | Exclude a churning path | `sirsi spotlight-exclude` | user | yes | stops an indexing storm at its source |
| Spotlight | Rebuild a corrupt index | `mdutil -E /` | admin | yes | fixes a storming/broken index |
| **DNS/Net** | Flush DNS | `dscacheutil -flushcache` + `killall -HUP mDNSResponder` | admin | yes | clears stale DNS |
| Font | Clear font caches | `atsutil databases -remove` | admin | yes | clears font-cache bloat |
| **Service** | Stop a runaway agent | `launchctl bootout gui/$UID/<label>` | user (own) | yes | stops a misbehaving LaunchAgent |
| Self | Heal binary drift | `sirsi self-update` | user | yes | replaces a stale/self-crashing binary |

Ruled OUT as fixes (honest): `renice` for a **memory** problem (frees CPU, not RAM);
`memory_pressure`/`vm_stat` (diagnostics, not levers); `vm.compressor_mode` (unsafe tuning).

## Finding → lever map (the contract)

| Finding | Outcome | Lever |
|---|---|---|
| RAM Pressure (high) | ACTION | `sirsi relieve --memory` (name hog + `purge`, admin-auth) |
| Swap Usage (critical **and** live pressure) | ACTION | `sirsi relieve --memory` |
| Top Memory Consumers | GUIDANCE | name + "quit / suspend it" |
| App Hangs (live CPU) | ACTION | `sirsi relieve` (renice) |
| Thread Leaks | ACTION | `sirsi relieve` |
| Disk Space (files) | ACTION | `sirsi clean --include-caution` |
| Disk Space (snapshots) | ACTION | `tmutil thinlocalsnapshots` (admin-auth) |
| Spotlight Storm | ACTION | `sirsi spotlight-exclude` / `mdutil -E` |
| App Crashes (7d) | ACTION | `sirsi clean --include-caution` |
| binary-drift | ACTION | `sirsi self-update` |
| Kernel Panics (7d) | GUIDANCE | name + advise (no safe auto-fix) |
| Jetsam Events (7d) | INFO (trend) | + memory relief when live |
| DNS stale | ACTION | flush DNS (admin-auth) |
| Loaded-not-running agents | ACTION | `launchctl bootout` |

## Architecture

A single **`RemediationCatalog`** (`internal/guard/remediation.go`) is the source of truth:
`Check → []Lever{Command, Privilege(User|Admin), Class(Action|Guidance|Info), Reversible, Measure(before/after), Describe}`.
`remediationCommand`/`remediationKind` become lookups into it. Every surface (menubar
`FindingView`, CLI, TUI) runs the uniform flow: **preview → confirm/auth → run → measure →
prove → re-verify**. Admin levers always route through the macOS auth dialog.

## Rollout

- **P1 (shipped, PR #124):** memory — `sirsi relieve --memory` (name hog + `purge` + proof).
- **P2:** disk snapshots (`tmutil`), Spotlight rebuild (`mdutil -E`).
- **P3:** DNS flush, `launchctl bootout` for runaway agents, font caches.
- **P4:** extract the `RemediationCatalog` type; migrate `remediationCommand` to it; add the guidance renderer for the no-lever findings.

## Neith's Triad (A22)

- **Data flow:** `finding → RemediationCatalog lookup → {Action | Guidance | Info} → (preview → auth/confirm → lever → measure) → re-verify → surface`.
- **Order:** P1→P4 above; P1 is the minimum viable (memory, the reported break).
- **Key decisions:** monitor-as-fix → **banned** (Three-Outcome Law); renice-for-memory → **rejected** (frees CPU not RAM); privileged levers → **macOS admin-auth dialog** (never sirsi-handled credentials, A11).
