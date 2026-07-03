# 𓇶 Ra Idea Router — Multi-Agent Work Queue

Shared filesystem protocol for agent collaboration on sirsi-pantheon.

The Idea Router is Ra infrastructure homed in Pantheon. Ra owns the agent registry, work queue, dispatch protocol, cross-agent relay, portfolio authority, and super-agent mandates. Horus owns each desktop's local node: daemon health, local agent/window visibility, local repo status, and the operator dashboard for this machine. Router v3 treats the router as a multi-agent work queue, not a two-person notice board. Work is addressed to registered agent IDs, and dispatch is successful only when the target agent starts and writes back to the router protocol.

Thoth preserves router memory across compaction and session boundaries. Ma'at validates router governance: correct agent targeting, repo segmentation, test evidence, honest blockers, and `/goal` completion.

Other repositories must not fork the router. They carry startup pointers and repo-specific law, while Sirsi-wide orchestration remains here under Ra.

## How it works

- `proposals/` — Either agent writes a plan before implementing
- `reviews/` — The other agent reviews the proposal or the code
- `decisions/` — Converged recommendations for user authorization
- `state.json` — Active topics and collaboration rules
- `agents.json` — Registered agent CLIs and wake mechanisms
- `threads.json` — Live thread registrations (CTR): one entry per open agent session/worker

## Thread Registration (CTR)

Registered agents describe *who can be woken*. Threads describe *which sessions are alive right now*. Every conversation/worker that touches the router should:

1. Determine or declare its `agent_id`.
2. Register a `thread_id` once at startup.
3. **Run a heartbeat loop while alive** — a persistent wake-loop that watches the inbox until the thread closes. Each surface uses its own mechanism; the contract is identical (see "Heartbeat Loop" below).
4. Watch its inbox and either work, queue, or block.
5. Close the thread at the end of the session.

Commands:

```sh
sirsi thread register --agent claude-pantheon --surface claude --workstream pantheon
sirsi thread heartbeat --thread thr-XXXX --status active --current-item <doc-id>
sirsi thread list
sirsi thread close --thread thr-XXXX
```

Surfaces are model-neutral: `claude`, `codex`, `gemini`, `gemma`, `qwen`, `mcp`, `api`, `webhook`, `worker`. Missing/invalid registration is treated as a local-node health problem in `sirsi router node-status`.

### Heartbeat Loop (mandatory from register → close)

A registered thread that is not looping is invisible to its own inbox — items addressed to it sit unread until the next manual `ctr`. The heartbeat loop is what makes registration mean "alive and watching," not just "known." It is the same primitive across every surface; only the implementation differs:

| Surface | Heartbeat mechanism |
| :--- | :--- |
| `claude` | **`/loop`** — self-paced, watching the inbox; arm a file Monitor on `items/` and a fallback ScheduleWakeup. This IS the Claude heartbeat. |
| `codex` | Codex app heartbeat automation (`ctr-thread-wake` polling the router inbox); native thread heartbeat where available. |
| `gemini`/`gemma`/`qwen` | Surface-native loop, or fall back to `sirsi router daemon`. |
| `mcp`/`api`/`webhook`/`worker` | `sirsi router daemon` or the resident launch agent. |

Rules:

- **Start the loop at registration, stop it only at `sirsi thread close`.** Registered-but-not-looping is a node-health failure.
- The loop's job is minimal: pull the inbox, act on or queue new items, emit a `sirsi thread heartbeat`, sleep. It is not a work driver — it is a watcher.
- Prefer event-driven waking (file Monitor on `items/`) over fixed polling; keep a long fallback tick so the loop survives a missed event.
- One loop per thread. De-registering (`thread close`) is the only clean way to end it.

**Durable cross-session watcher (A27) — one channel, two front doors.** For worker/headless surfaces the persistent heartbeat loop is a per-agent launchd pull-loop: a `KeepAlive` LaunchAgent (`ai.sirsi.router.wake.<agent>`) that runs `sirsi router wake-loop <agent>`, which each interval pulls the agent's inbox and heartbeats — surviving session restarts because launchd owns its lifecycle. There is exactly ONE such channel; it has two equivalent front doors:

| Command | Scope | Use when |
| :--- | :--- | :--- |
| `sirsi router wake-install <agent>` | agent-scoped (explicit id) | installing for a named agent (setup/topology) |
| `sirsi thread watch --install` | thread-scoped (self-resolving) | a session arming its OWN durable watcher without knowing its agent id |

`sirsi thread watch --install` == `sirsi router wake-install <that agent>` — the thread verb resolves the current thread's agent (via `--agent`, `$SIRSI_AGENT_ID`, the session→agent marker, or a sole live thread) and delegates to the SAME `InstallWakeLaunchAgent` path. It is a thin alias, not a second watcher subsystem. `sirsi thread watch` (no flag) reports whether the channel is installed; `--uninstall` cleanly removes it. Interactive `claude` sessions do NOT use this — they heartbeat via `/loop` (row 1 above).

## Protocol

1. Read `state.json` and latest proposals/reviews before starting work
2. Write a proposal before implementing anything non-trivial
3. Include an ETA/check-back time for each active task so responders do not poll constantly
4. After implementation, the reviewing agent writes a review
5. Safety objections block implementation until resolved
6. Failing tests block release

See `IDEA_ROUTER_DESIGN.md` in the Codex handoff directory for full spec.

## Mandatory Workstream Protocol

Every non-trivial Codex/Claude workstream MUST use:

- `/plan` before implementation.
- `/goal` as the explicit completion flag.
- One repo-scoped agent per repository.
- A written super-agent mandate before any one agent coordinates or edits across repos.
- Router handoff files that keep the other agent queued until the `/goal` is met.
- A registered `agent_id` target once `.agents/idea-router/agents.json` exists.
- An `eta_for_review` or `next_check_at` timestamp for every routed task.

### Reply Notification Cadence — syn/ack (A26/A27)

> Ratified 2026-06-05 by claude-pantheon (`…222752`) + codex-pantheon (`…223029`)
> after repeated missed replies. The failure: `sirsi router pull <agent>` surfaces
> ONLY open items addressed **to** you, so a reply made by *closing the sender's
> item with a `## Result`* is invisible to the sender's normal pull loop.

**The rule — new inbound item is REQUIRED for agent-to-agent notification:**

1. **Reply = a NEW open item** addressed to the other agent. This is the only
   sanctioned notification path. You MAY also close the original with a `## Result`
   for the audit trail, but **close+Result is audit only, never the notification.**
   Agents MUST NOT rely on close-only replies for new work.
2. **ACK handshake**: SYN = sender sends item → recipient inbox. ACK = recipient
   sends a reply/ack item → sender inbox (a one-line `type: ack` referencing the
   original item id is fine for "got it / acting / ETA X"), **then** closes the SYN.
   Two inbound signals, both visible to a plain `router pull`.
3. **Cadence**: 60s heartbeat + inbox poll on both sides. The resident Horus
   supervisor (`ai.sirsi.horus.agent-router`) is the always-on health/inbox-surface
   floor — **not** a delivery layer. Each live session also runs its own watcher
   (file Monitor over `items/`, keyed on its `thread_id` so `pgrep -f <tid>` finds
   it; heartbeats each tick).
4. **Sent-item closure scanning is TRANSITIONAL compatibility only** — a
   backfill observer for legacy Style-A (close-only) replies, NOT a durable second
   notification path (a second path drifts). Watchers may scan their own sent items
   for `status: closed` + `## Result`, but the durable contract is rule #1.

### Repo Segmentation

Work on repositories is segmented by default. A normal agent owns exactly one repository. It may inspect another repo only for read-only context and must not edit outside its assigned repo.

A super agent is allowed only when the `/plan` says:

1. Which repositories are in scope.
2. Whether the super agent may edit files or only coordinate.
3. Which repo-scoped agents own implementation.
4. What evidence is required before the `/goal` is complete.

### Goal Relay

Submissions by one agent should trigger the other:

1. Codex writes proposal/review/decision and adds a pending item for Claude.
2. Claude reads the pending item, works or reviews, then writes its own router artifact.
3. Claude adds a pending item for Codex.
4. The relay continues until the `/goal` completion condition is met.

### ETA-Driven Checks

Agents must provide an approximate completion/check-back time with every routed task. Use absolute ISO-8601 timestamps when possible.

Required fields in router artifacts:

- `eta_for_review`: when the agent expects the task or next checkpoint to be ready.
- `next_check_at`: when Codex/universal responder should check again if no writeback appears.
- `estimated_duration`: rough human-readable duration such as `20 minutes`, `2 hours`, or `tomorrow morning`.

Codex should not run fixed high-frequency polling. It should schedule or perform checks near `next_check_at`, immediately when explicitly asked to `ctr`, and opportunistically when the user is already active. If an agent cannot estimate, it must say why and provide a conservative next checkpoint.

### Codex Agent Contract

Every Codex agent launched inside this repo must know about the router through one of three paths:

1. Repo startup context from `AGENTS.md`.
2. A router launch prompt generated by the daemon.
3. A direct user instruction to run `ctr`.

At startup, a Codex agent must read `state.json`, this README, and pending artifacts addressed to its registered agent id before starting unrelated work. A router-launched Codex agent must receive the repo root, `agent_id`, topic, `/plan`, `/goal`, expected writeback artifact, and timeout/blocked reporting rules in the launch prompt.

Generic `codex` and `claude` inboxes are legacy compatibility only. New work should target registered IDs such as `codex-pantheon`, `claude-pantheon`, or another entry in `agents.json`.

### Interim Universal Responder

Until the multi-agent response fabric is implemented, Codex is the universal responder for router requests. Claude agents and other workstream agents may address review, triage, and routing questions to Codex through the router queue.

This is a coordination mandate, not permission for unbounded edits. Codex may review, approve, reject, route, or define next work for any router item, but implementation remains repo-segmented unless a written super-agent mandate grants cross-repo scope.

Router items that need this temporary responder should target `codex`, `codex-pantheon`, or the registered Codex responder id once `agents.json` exists.

Universal responder requests must include `next_check_at` or `eta_for_review`. Missing ETAs should be treated as an incomplete handoff unless the item is urgent or already complete.

### Parallel Agent Dispatch

The universal responder must not serialize independent work by default. When a router item contains independent repo-scoped tasks, or when the user asks for acceleration, the responder should fan out work to the appropriate registered agents in `agents.json`.

Parallel dispatch rules:

- One repo-scoped agent per repository remains the default unit of work.
- Independent tasks in different repos should be dispatched concurrently.
- Independent tasks in the same repo may be split only when their file ownership is clearly disjoint.
- A super-agent mandate is required before any one agent coordinates or edits across multiple repos.
- The responder should assign concrete ownership, `/goal`, expected artifacts, and ETA/writeback requirements to each launched or routed agent.
- The responder should keep integration, review, and cross-agent conflict resolution with the universal responder unless the `/plan` names another coordinating agent.
- If live agent launch is unavailable, the responder must still create addressed router work items for each target agent instead of holding all work in one queue.

This rule exists to avoid unnecessary serial work. Commercial completion is measured by working, verified outcomes, not by how many turns the coordinator personally spends on implementation.

### Net Goal-Weaving Doctrine

Net 𓁯, The Weaver, is the portfolio goal-weaving standard. Net's job is to keep every repo aligned to its product surface plan, phase plan, architecture choices, and `/goal` completion evidence.

Every routed workstream must preserve these portfolio decisions:

- Simplify vendor surface area: prefer GCP-native, Firebase-native, or open-source tooling before paid third-party services.
- Choose robust primitives, not toy shortcuts: PostgreSQL/Cloud SQL/AlloyDB is preferred for canonical business truth, ledgers, contracts, payments, probate, subscriptions, audit trails, and reporting.
- Use Firestore for real-time UX state, presence, notifications, collaboration, and denormalized dashboard views.
- Use Cloud Storage for files, media, generated PDFs, evidence packets, exports, and user-owned artifacts.
- Use SQLite only for local app state, desktop/CLI caches, router ledgers, and offline indexes.
- Do not introduce a paid vendor without an ADR covering why GCP/open-source is insufficient, what data leaves Sirsi/client control, cost risk, exit path, and replacement plan.
- Do not reopen language choices without a measured blocker and a written ADR.

Surface targets:

- FinalWishes: web and mobile.
- Assiduous: web and mobile.
- Porch & Alley: web and mobile.
- Sirsi Nexus: native web, native mobile app, and native desktop app.
- Pantheon: local desktop and web.
- Homebrew tools: distribution only.

Agents must work independently toward the full `/goal` whenever their repo-scoped work can proceed without waiting. If they need another agent, they should route the dependency with an ETA rather than stopping the whole workstream.

### Thoth Memory Contract

Thoth is part of the router loop. Before context compaction or session handoff, Thoth must preserve router state alongside project memory:

1. Active topics and their `/goal` status.
2. Pending items by agent id.
3. The next required agent/action.
4. Dispatch ledger status when present.
5. Blockers, failed dispatches, or writeback gaps.

`sirsi thoth compact` embeds a router snapshot automatically when `.agents/idea-router/state.json` exists. Agents using the Thoth skill must still read router files directly when they need current work context.

## Router Addressing Law

Every router item must be addressed to exactly one repo-scoped agent unless a written super-agent mandate exists.

Use this addressing formula:

```text
<agent-family>-<repo-or-workstream>
```

Examples:

- FinalWishes repo review for Claude: `claude-finalwishes`
- FinalWishes repo review for Codex: `codex-finalwishes`
- Pantheon router/CLI work for Claude: `claude-pantheon`
- Sirsi Nexus work for Codex: `codex-nexus`
- Assiduous work for Claude: `claude-assiduous`

Do not address FinalWishes work to `claude-pantheon` just because the router lives in Pantheon. Pantheon is the router home; the target repo still determines the agent id.

### Choosing The Target Agent

1. Identify the repo that owns the implementation or review.
2. Pick the agent family requested or implied by the work: `codex`, `claude`, `gemini`, `gemma`, `qwen`, or another registered family.
3. Look up the exact id in `/Users/thekryptodragon/Development/sirsi-pantheon/.agents/idea-router/agents.json`.
4. Put the item under `pending.<agent_id>` only.
5. Use `pending_for_codex` or `pending_for_claude` only for legacy compatibility when no repo-scoped registered id exists. If you must use a legacy field, it must contain plain string document ids only and must not create a duplicate route to the wrong repo agent.

### State JSON Shape

`state.json` pending queues are machine-readable and must remain arrays of strings:

```json
{
  "pending": {
    "codex-finalwishes": ["20260520-example-doc-id"],
    "claude-pantheon": []
  },
  "pending_for_codex": [],
  "pending_for_claude": []
}
```

Never put metadata objects inside `pending`, `pending_for_codex`, or `pending_for_claude`. Metadata belongs in the proposal/review/decision frontmatter and body.

Invalid:

```json
{
  "pending": {
    "codex-finalwishes": [
      {
        "id": "20260520-example-doc-id",
        "eta_for_review": "2026-05-20T22:00:00-04:00"
      }
    ]
  }
}
```

That object-valued form breaks the Go router parser and stalls automation.

### Required Artifact Frontmatter

Each routed artifact should include:

```yaml
id: 20260520-agent-repo-topic
author: claude-finalwishes
addressed_to: codex-finalwishes
topic: finalwishes-tier1-ga
repo: /Users/thekryptodragon/Development/FinalWishes
agent_scope: repo-segmented
eta_for_review: 2026-05-20T22:00:00-04:00
next_check_at: 2026-05-20T22:00:00-04:00
estimated_duration: 1 hour
```

### Super-Agent Exception

A broad coordinator may route or edit across repos only when a router artifact explicitly names it as a super agent and lists:

- repositories in scope
- whether it may edit or only coordinate
- repo-scoped implementation owners
- verification evidence required before `/goal` completion

Without that mandate, route work to the repo owner agent and stop there.


## Full Automation

### Thread Registration

Agents are registered in `agents.json`; live conversations and workers must also register as threads. A registered agent without a live thread is only an address. A live thread tells Horus what is actually awake on this desktop.

Required thread facts: `thread_id`, `agent_id`, repo, workstream, surface, start time, last heartbeat, watched inboxes, wake mechanism, current item, and last error. Horus `router node-status` must distinguish registered agents from live/stale threads.

The commercial path is `sirsi router work` for operator-driven pull, plus the autorouter daemon for always-on push/pull. Manual status checks are diagnostic only.

### Commands

| Command | Purpose |
|---------|---------|
| `sirsi router status` | Show registered-agent inboxes and active topics |
| `sirsi router work` | Check the router once, then launch runnable registered-agent work |
| `sirsi router work --poll` | Keep polling and launching runnable work until interrupted |
| `sirsi router work --dry-run` | Preview runnable launches without starting agents |
| `sirsi router daemon --dry-run` | Preview dispatches without launching agents |
| `SIRSI_ROUTER_NOTIFY=1 sirsi router daemon` | Run live in the foreground |
| `sirsi router install-agent --load` | Install and start the resident macOS launch agent |
| `sirsi router service-status` | Check whether the launch agent is installed and loaded |
| `sirsi router uninstall-agent` | Stop and remove the launch agent |
| `sirsi router smoke` | Verify both CLIs can launch and write to `.agents/idea-router/` |
| `sirsi router smoke --agent-pair` | Full relay test: seed item → Claude → Codex → verify writeback |
| `sirsi router smoke --dry-run` | Check CLIs exist without launching |

### What Process Runs

`sirsi router work` is the explicit "check the router, then work" command. It does not require `SIRSI_ROUTER_NOTIFY=1` because invoking it is the operator approval to launch registered agents. By default it checks once; `--poll` makes it a foreground polling worker.

The daemon (`sirsi router daemon`) watches `.agents/idea-router/state.json`, `proposals/`, `reviews/`, and `decisions/` with `fsnotify`, with a one-second fallback poll. It dispatches pending inbox items immediately, keeps a persistent `dispatch-ledger.json` so restarts do not relaunch unchanged work, and never acknowledges inbox items for an agent.

When installed as a launch agent (`sirsi router install-agent --load`), launchd runs the daemon automatically at login and restarts it if it crashes. The plist sets `SIRSI_ROUTER_NOTIFY=1` and `KeepAlive=true`.

### Agent Launch Flags

**Claude** (launched by `notifyClaude`):
```
claude --print --permission-mode auto <prompt>
```
- `--print`: non-interactive, outputs result and exits
- `--permission-mode auto`: bypasses interactive permission prompts

**Codex** (launched by `notifyCodex`):
```
codex exec -C <repo-root> --sandbox workspace-write <prompt>
```
- `-C <repo-root>`: sets the working directory to the repo root
- `--sandbox workspace-write`: allows writes within the workspace (covers `.agents/idea-router/`)
- Note: `--add-dir` is NOT used for paths inside the workspace — it creates conflicting sandbox rules

### Where Logs Live

- Foreground daemon: stdout/stderr
- Launch agent: `.agents/idea-router/logs/autorouter.out.log` and `.agents/idea-router/logs/autorouter.err.log`
- Dispatch history: `.agents/idea-router/dispatch-ledger.json`

### How to Prove It Is Working

```bash
# 1. Check service status
sirsi router service-status

# 2. Verify both agents can write (fast, no relay)
sirsi router smoke

# 3. Full relay test (launches both agents, seeds items, verifies writeback)
sirsi router smoke --agent-pair

# 4. Check inbox state
sirsi router status
```

If no automation runner is active, the pending item is still the source of truth. Agents must check `state.json` and the latest router files at session start.
