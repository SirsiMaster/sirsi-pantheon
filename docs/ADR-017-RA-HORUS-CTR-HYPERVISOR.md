# ADR-017: Ra/Horus CTR Hypervisor — Multi-Agent Orchestration Canon

## Status
**Accepted** — 2026-05-19

**Deciders:** Cylton Collymore, Codex, Claude

## Context

Pantheon evolved from a single-machine CLI tool into a multi-agent orchestration
platform. Building Pantheon with multiple AI agents (Claude, Codex, Gemini,
Gemma, Qwen) exposed a coordination gap. The user was the message bus — manually
shuttling proposals, reviews, and decisions between agents in separate terminal
windows. Context was lost between sessions. Work stalled when the user stepped
away.

The Idea Router (CTR — Cross-Team Router, invoked via `ctr`) was built as a
filesystem-based work queue to solve this. It grew through three versions:

- **v0**: Filesystem protocol — proposals, reviews, decisions in Markdown/JSON
- **v1**: State tracking — `state.json` inbox with per-agent pending queues
- **v2**: Agent registry — `agents.json` address book for 8+ named workers
- **v3**: Multi-agent work queue with pluggable executors, dispatch ledger,
  fsnotify daemon, macOS launch agent, and smoke tests

Prior to this decision, Ra was described as "Fleet Lord" (ADR-015) and Horus as
"Code Graph" — neither clearly owned the router. ADR-015 addressed machine
orchestration (scan results rolling up). This ADR addresses a distinct concern:
**agent orchestration** across the Sirsi portfolio.

## Decision

### Ra 𓇶 — Sirsi-Wide Orchestration

Ra owns everything that coordinates across machines, repos, and agents:

| Domain | Scope |
|--------|-------|
| Agent registry | Portfolio-wide. `agents.json` names every worker across all repos. |
| Work queue | `state.json`, `proposals/`, `reviews/`, `decisions/` — cross-agent inbox. |
| Dispatch protocol | Launch prompts, relay verification, writeback contracts, completion routing. |
| Super-agent mandates | Cross-repo coordination permissions. Only Ra may grant. |
| Portfolio authority | `/plan`, `/goal`, ETA-driven checks, repo segmentation enforcement. |

### Horus 𓂀 — Per-Desktop Runtime Node

Horus owns everything on ONE machine's local experience:

| Domain | Scope |
|--------|-------|
| Daemon health | Autorouter process status, launch agent lifecycle. |
| Local agent visibility | Which agents are running, their status, window management. |
| Local repo status | Uncommitted work, scan state, last action. |
| Operator dashboard | TUI status tab, menu bar app, workstation metrics. |
| Code graph | AST symbol extraction, file outlines (original Horus scope retained). |
| Session state | `tui-state.json` — Horus's local memory. |

### Supporting Deities (Unchanged)

- **Thoth** 𓁟: Preserves router continuity — `memory.yaml` snapshots router
  state across compaction and session boundaries.
- **Ma'at** 𓆄: Validates router governance — `/plan`, `/goal`, handoff
  compliance, repo segmentation, test evidence, honest blocker reporting.
- **Net** 𓁯: Keeps portfolio goals aligned — plan/build log drift detection,
  product surface targets, architecture decisions.

### Product Surface

The original ADR included a fsnotify daemon and macOS LaunchAgent command family
(`sirsi router daemon`, `sirsi router install-agent`, `sirsi router
service-status`). Those commands are now superseded by the ADR-024/ADR-026
pull-model router surface. Keep this ADR as history, but do not implement new
surfaces against the removed daemon verbs.

The CTR hypervisor exposes these user-facing commands:

| Command | Description |
|---------|-------------|
| `sirsi router status` | Show agent inboxes and active topics |
| `sirsi router pull <agent>` | Check one agent inbox and print runnable work |
| `sirsi router show <item>` | Inspect a routed artifact |
| `sirsi router send` | Create a routed artifact addressed to one registered agent |
| `sirsi router close <item>` | Close a routed artifact with evidence |
| `sirsi router ledger [agent]` | Join item age, staleness, dependencies, pickup, and task commitments |
| `sirsi router task add\|update\|list` | Manage the durable per-agent task registry |
| `sirsi router node-status` | Report the local Horus node, threads, aliases, and LaunchAgent inventory |

### User-Facing Summary

> Ra routes the work. Horus shows what is happening on this machine.
> Thoth remembers. Ma'at validates. Net keeps goals aligned.

The user types `ctr` (check the router) to invoke the protocol. User-facing
output uses plain outcomes, not deity internals.

### Future: MCP v1 Overlay

The filesystem router remains the source of truth. A planned MCP server will
provide programmatic access:

- Tools: `router_create_proposal`, `router_list_open_topics`,
  `router_add_review`, `router_record_decision`, `router_get_context`
- Resources: `idea-router://state`, `idea-router://proposal/{id}`,
  `idea-router://decision/{id}`

The MCP server persists to `.agents/idea-router/` so collaboration history
remains inspectable in git.

## Consequences

1. All router CLI commands (`sirsi router *`) are Ra infrastructure
2. The TUI status dashboard is Horus's operator surface
3. The autorouter daemon reports to Horus locally but dispatches via Ra
4. Agent registry (`agents.json`) is a Ra artifact
5. Work queue and dispatch ledger are Ra artifacts
6. Session state persistence (`tui-state.json`) is Horus's local memory
7. Thoth compact includes a router snapshot — this bridges Ra (state) and Thoth (memory)
8. Other repos must not fork or re-home the router — they carry startup pointers only
9. Filesystem protocol is inspectable in git, works offline, requires zero infrastructure
10. ADR-049 adds the Universal Task Ledger as Ra's typed execution view; CTR and future surfaces are thin projections over it

## Alternatives Considered

| Option | Rejected Because |
|--------|-----------------|
| Horus owns the router | Router is cross-machine/cross-repo; Horus is per-machine |
| New deity for CTR | Unnecessary complexity; Ra already owns orchestration |
| Flat ownership (no hierarchy) | Ambiguous; leads to conflicting implementations |
| Per-repo routers | Creates N coordination surfaces; user is back to being message bus |
| External service (Slack, Linear) | Vendor dependency, requires auth, breaks offline |
| MCP server only (skip filesystem) | Requires running server; filesystem works even with no daemon |

## References

- ADR-015: Deity Hierarchy (Horus as Local Lord, Ra as Fleet Lord)
- ADR-014: Stele Ledger (event bus for inter-deity communication)
- Rule A26: Idea Router Workstream Protocol
- Rule A24: Ra Scope Autonomy
- Rule A25: Deity Registry & Attribution
- `.agents/idea-router/DESIGN.md`: Full protocol specification
- `/Users/thekryptodragon/Development/AGENTS.md`: Workspace-wide agent law
