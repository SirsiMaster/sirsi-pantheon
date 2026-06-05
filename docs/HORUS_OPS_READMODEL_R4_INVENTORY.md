# R4 Capability Inventory — Watcher Specs + Horus Ops Read-Model

| Field | Value |
| :--- | :--- |
| **Requirement** | **R4** of the always-on supervisor (capability inventory). Companion to R1/R2/R5 (ADR-024) and R3 (ADR-025). |
| **Status** | Living inventory — June 2, 2026. Tracks code ground truth; update on every watcher-spec or `NodeStatus` change. |
| **In-code source of truth** | `internal/router/watcherspec.go` (watcher specs) · `internal/router/nodestatus.go` (`CollectNodeStatus` → `NodeStatus`) |
| **Owner** | claude-home (Horus ops-view content lane). Surface rendering = claude-pantheon (ADR-026 boundary). |
| **Governs** | [ADR-026](ADR-026-HORUS-OPS-DASHBOARD.md), [ADR-024](ADR-024-ONE-WATCHER-PER-SURFACE.md), PANTHEON_RULES.md A27 |

> **Why this exists:** R4 of the supervisor vision is a *capability inventory* —
> a single, auditable answer to two operator questions: **(1) how does each
> surface stay alive (which watcher does it arm)?** and **(2) what can the
> operator actually see, and what is still trapped in Go?** `watcherspec.go`
> encodes (1) in code; this doc is its human-readable form plus the read-model
> exposure ledger that ADR-026 acts on.

---

## Part 1 — Per-Surface Watcher Capability Inventory

The router prescribes exactly **one** liveness/wake mechanism per surface
(ADR-024 Decision 2). `sirsi thread register` is a pure handshake that **returns**
the canonical spec via `router.WatcherFor(surface, agentID, threadID)`; the
surface arms exactly that — it never invents its own. One mechanism per thread,
armed at `register`, stopped only at `close` (A27).

| Surface(s) | Watcher `Type` | Mechanism | Heartbeat | Watches inbox? | Resident? | Arm responsibility |
| :--- | :--- | :--- | :---: | :---: | :---: | :--- |
| `claude` | `loop-monitor` | `/loop` + Monitor on `items/` | 60s | yes | no | Agent arms `/loop`; idempotent re-arm keyed on **`pgrep -f thr-<thread_id>`** (never the shared `DIR=` body, never TaskList) |
| `codex` | `app-heartbeat` | codex app heartbeat (`ctr-thread-wake` polling `items/`) | 60s | yes | no | Native app automation; no manual loop |
| `gemini` `gemma` `qwen` | `surface-loop` | surface-native pull loop over `items/` | 60s | yes | no | Surface loop pulls its own inbox |
| `menubar` `tui` `vscode` `jetbrains` `cursor` `macapp` | `native-runloop` | native runloop heartbeat ping (resident) | ≥60s | **no** | **yes** | Heartbeat from native runloop on a bounded interval; **no inbox poller** unless the surface acts on items; close on graceful shutdown, hard kill → OS-truth reap (ADR-022) |
| `mcp` `api` `webhook` `worker` | `pull-loop` | bounded headless pull loop over `items/` | 60s | yes | no | Loop calls `sirsi router pull <agent>` and heartbeats |
| *(unrecognized)* | `pull-loop` | generic pull loop over `items/` | 60s | yes | no | Fallback: pull inbox + heartbeat; no daemon verb is exposed |

**Invariants:**
- **Idempotence** — re-arm only when zero matching watcher processes exist for
  *this* `thread_id`. The signature MUST include the thread_id; the shared loop
  body collides with other agents' loops on a shared host (claude-deck regression
  `838ad66`), and TaskList falsely reports empty (F2 → duplicate watchers).
- **Resident surfaces are nodes, not renderers** (A27 addendum 2026-06-01) — a
  menubar/TUI/IDE/macapp registers bound to its **own process PID** and
  heartbeats on a bounded interval (≥60s) from its native runloop, **never** on a
  render/stats tick (the 2026-06-01 `mds_stores` lockup). Heartbeat-only is
  sufficient when the surface does not act on inbox items.
- **One watcher per thread** — registration does not spawn a watcher (ADR-024
  D2); it returns the spec. No belt-and-suspenders (no caffeinator + fs-watcher +
  `/loop` churning together).

---

## Part 2 — Horus Ops Read-Model Source Inventory

`router.CollectNodeStatus(repoRoot, launchctlCheck, authProbe…)` aggregates every
operator-visible signal into one `NodeStatus` in a single pass. This is the
"picture inside the frame" (ADR-026). Inventory of what it already pulls:

| Read-model section | `NodeStatus` fields | Source | Honesty / safety property |
| :--- | :--- | :--- | :--- |
| Agents + wake readiness | `RegisteredAgents`, `AgentCount`, `WakeHealth[]` | `LoadRegistry` + `cfg.Validate()` + PATH lookup | Each agent's prescribed wake mechanism validated; `Ready=false` + detail on misconfig |
| Router queue | `PendingByAgent`, `TotalPending`, `ActiveTopics`, `CompletedCount`, `LastClaudeRead`, `LastCodexRead` | `router.ReadState()` (normalized) | Pending-by-agent is the unread-inbox signal per agent |
| Dispatch failures | `RecentFailures[]` (≤5, newest first) | `LoadWorkQueue` → `StatusFailed`/`StatusBlocked` | Last attempt error surfaced; no silent failure |
| **Live + stale threads** | `LiveThreads[]`, `StaleThreads[]`, `LiveThreadCount`, each `ThreadSummary.os_state` | `ReapDeadThreads` (host-scoped) then `LoadThreadRegistry` + `PIDStateOf` | **OS-truth liveness (ADR-022)** — a gone/defunct PID can never render live; reap runs *before* read |
| LaunchAgent + binary drift | `LaunchAgents[]` plus legacy `DaemonInstalled`, `DaemonLoaded`, `ConfiguredBinary`, `BinaryExists`, `BinaryIsGoRun` | Known LaunchAgents (`ai.sirsi.pantheon`, `com.sirsi.idea-router`, sweep, registry-police, legacy router daemon) + `LaunchAgentProgram` | **Stale-deploy drift (ADR-023)** — installed helpers vs live pull-model contract |
| Agent CLI auth | `AgentHealth[]` (`CLIFound`, `AuthOK`, `NeedsLogin`, `BlockedItems`) | `exec.LookPath` + injectable `AuthProbe` | Distinguishes unauthenticated from stripped-env (`USER`/`HOME` missing) false-negatives |

### Exposure ledger — the R4 gap ADR-026 closes
The read-model is **complete**; only its *exposure* is missing.

| Exposure seam | Today | ADR-026 |
| :--- | :--- | :--- |
| CLI verb | ❌ **none** — only `router status` (queue) + `thread list --json` (raw). Rule A27 references `router node-status`, which **does not exist**. | `sirsi router node-status [--json]` over `CollectNodeStatus` |
| Dashboard HTTP | ❌ **none** — contract matrix row *Router ack → MISSING (no `/api/router/*`)*. `contract.go` freezes `StatsResponse` only. | typed `GET /api/node-status` (+ `?view=summary` → `OpsSummary`) serving `router.NodeStatus` |
| menubar render | ❌ hosts dashboard server (`main.go:131-143`) but renders no ops-view | summary rows + glyphs, resident heartbeat-only (claude-pantheon) |
| TUI render | ❌ scaffold, no ops pane | 4th pane over full `NodeStatus` (claude-pantheon) |
| Name collision | ⚠️ `/api/horus/*` already = code-graph (`SymbolGraph`/`WorkstationReport`) | ops-view is `/api/node-status`, **not** `/api/horus` (ADR-026 §2) |

### Naming note — two distinct "Horus"
- **Code-graph Horus** (`internal/horus`): Go AST symbols, file outlines, context
  queries → `/api/horus/scan|query|report`. Unchanged.
- **Ops Horus** (this read-model, ADR-015 "the dashboard is Horus"): the operator
  command-center over the router → `/api/node-status` + `sirsi router
  node-status`. ADR-026 keeps these names disjoint.

---

Refs: PANTHEON_RULES.md A27 (heartbeat loop) / A26 (relay) / A22 (Triad);
ADR-024 (`watcherspec.go`), ADR-022 (OS-truth), ADR-023 (binary drift),
ADR-026 (the ops-dashboard that consumes Part 2); `DASHBOARD_CONTRACT_MATRIX.md`
(the action-side companion to this read-side inventory).
