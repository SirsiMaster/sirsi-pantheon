# Agent-Operations Parity — Diary + Product Requirement (2026-06-16)

**Author:** claude-home (AI agent) · **For:** claude-pantheon / Pantheon product
**Status:** PROPOSED (untracked artifact — Pantheon to adopt/triage)

---

## Why this document exists (the gap, in the owner's words)

> "Everything you did today you did behind the scenes as the AI agent. I should also be
> able to do these things either through the CLI or the menubar with or without the gemma
> agent. This is a real gap. Pantheon needs to know this and ensure that everything you did
> today — all information, insight and operational work — needs to be surfaced in the user
> applications."

Pantheon's thesis is the **self-healing compute substrate**: it keeps the *machine* able to
run. Today exposed the next frontier: the **operator** still can't *do* what the agent does.
Every coordination, review, insight, memory, and hygiene action below was run by an AI agent
with shell access — none of it is reachable by a human through `sirsi` (CLI) or the menubar,
and almost none of it has a deterministic (no-AI) path or a gemma-augmented (local-AI) path.

**The requirement:** every class of work in Part 1 must be surfaceable through the CLI **and**
the menubar, in two modes — **deterministic** (works with zero AI) and **gemma-augmented**
(local model adds reasoning/narration when present). This is `sirsi insight`'s AI-optional
pattern ([[reference_local_models_through_pantheon]]) generalized to *all* agent operations.

---

## Part 1 — Diary: everything the agent did today (detailed)

### A. Router supervision & the relay (coordination)
1. **Built `sirsi-respond.sh`** — an atomic "respond to a routed request" primitive: closes the
   item with a Result (audit) **and** routes a fresh inbound back to the requester
   (notification). Encodes the rule *"a request always requires a response"* — the router's
   close-with-Result is audit-only and never wakes the sender.
2. **Hardened the eternal supervisor** (`router-conduit-supervisor`) with an audit step: scan
   recently-closed request items and re-route any whose answer never reached the sender.
3. **Worked the inbox as definitive reviewer** — pulled, triaged, and responded to items;
   routed corrections to claude-deck (Forum VC), each as a fresh inbound.
4. **Armed an event-driven inbox watcher** (Monitor on `.agents/idea-router/items/`) — wakes
   only on a new open `to: claude-home` item, zero idle cost. Proven live this session.

### B. Binding review (document/code verdicts)
5. **Source-deep binding review of a legal instrument** — read a Forum SAFE + Side Letter in
   full, verified internal consistency, fee mechanics, entity correctness, absence of MFN,
   TEDCO-match non-trigger; issued a **GO** verdict with 3 counsel-awareness flags; routed it
   back via `sirsi-respond.sh`.
6. **Binding review request for code (PR #44)** — routed with explicit source-deep verify points.

### C. Strategic reasoning & advisory (the "colleague" layer)
7. **Caught and corrected a valuation category error** — accelerator capital is a credential,
   not a priced-round valuation mark (≠ fund check). Re-derived the founder doctrine.
8. **Fee/cap reconciliation** — proved the fee's equity bite = `fee ÷ cap`; the cap is the
   whole game, the fee defangs itself once the cap is won.
9. **"De-couple" analysis** — conservation of money: the SAFE *purchase amount* is the only
   number that sets dilution; "de-couple to 2.0%" is a fee waiver in substance, not free.
10. **Fund-check strategy** — go-tomorrow plan, $20M gate, the TEDCO auto-match lever ($500K →
    $1.0M for ~5%), the two-cap answer, the deployment-checklist ask.
11. **Probability estimate** — calibrated acceptance odds, updated on new evidence (3 meetings /
    3 weeks / accelerating cadence).
12. **The $5M floor three-frame defense** — shield (cascade/down-round), wall (constraint not
    preference → credible walk-away), credential ("if I won't do it to them, I won't do it to
    you"). Built into the operator's meeting card (docx, style-matched, validated).

### D. Memory (persistent operator knowledge)
13. Created/updated auto-memories: `feedback_request_requires_response`; the
    accelerator≠valuation-mark capstone + the $5M-floor three-frames in
    `feedback_founder_terms_or_pass`; **reversed the suspension** in
    `feedback_autoarm_stack_on_resume` (now: implicitly arm the watcher every resume); fixed
    the stale "don't re-arm" hub lines in `MEMORY.md`.

### E. Host operations & hygiene (the deterministic core — already partly in Pantheon)
14. **`sirsi diagnose`** — read host health (swap, Jetsam trend, crashes); interpreted it as
    *trailing-7d* pressure vs. a live crisis (90% RAM free now).
15. **Reaped 2 orphan heartbeat loops** — dead-parent `while true; sirsi thread heartbeat`
    shims (~2 days old) that were falsifying CTR liveness. Verified cmdline + dead ppid before
    kill ([[feedback_pid_alive_is_not_kill_evidence]]).
16. **`sirsi clean`** (preview) — discovered it's broader than `diagnose` advertised, surfacing
    the footgun that became PR #44.
17. **CTR reconciliation** — observed the registry reap 45 dead claude-home threads.

### F. Engineering (Pantheon's own code)
18. **Shipped PR #44** end-to-end: explored the codebase, based an **isolated worktree off
    `origin/main`** (the main checkout was on a peer's branch — untouched), fixed two footguns
    (active-dep rules → caution-tier; diagnose's crash remediation → `--include-caution`), added
    a regression test, ran `gofmt`/`go vet`/`golangci-lint` (0) + tests (green), wrote a
    house-style CHANGELOG entry, committed with SirsiMaster identity + traceability,
    pushed, opened PR #44, applied `binding-hold`, routed an independent-review request.

### G. Operator assistance
19. Read a screenshot and explained the menubar/sidebar iconography.
20. Spawned, then handled inline, the clean/diagnose fix as a tracked task.

---

## Part 2 — Capability map: what each action should become in the user apps

| Agent action (today) | In Pantheon now? | CLI surface (deterministic) | + Gemma (local AI) | Menubar |
|---|---|---|---|---|
| Work the router inbox / respond | partial (`sirsi router`) | `sirsi router work`, **`sirsi router respond <id> <file>`** (promote `sirsi-respond.sh` to a first-class verb) | gemma drafts the response, flags for review | Router panel: inbox count, one-click respond, "answer pending" badges |
| Auto-notify on reply (SYN/ACK) | NO | built into `router respond` | — | banner on new inbound |
| Binding review of a file/diff | NO | **`sirsi review <file|PR>`** → structured verdict | gemma reads + reasons; deterministic = checklist/lint | "Review…" item → verdict window |
| Strategic/advisory reasoning | NO | **`sirsi ask "…"`** (the colleague) | gemma answers locally; without gemma = deterministic templates/links | chat/insight panel |
| Persistent memory | NO | **`sirsi memory add/list/recall`** | gemma summarizes/links | Memory panel |
| Inbox watcher / liveness | partial (hooks nudge) | **`sirsi watch`** (event-driven daemon, auto-armed) | — | live status dot |
| Thread/CTR hygiene (reap orphans) | partial (auto-reap) | **`sirsi thread reap`** + surfaced list | — | Threads panel: alive/stale, one-click reap |
| Diagnose | **YES** | `sirsi diagnose` | gemma narration (`insight`) | partial — make it first-class |
| Clean | **YES** | `sirsi clean` (+ PR #44 tiering) | — | Clean Waste flow |
| Insight aggregation | **YES** | `sirsi insight` | `rules+gemma` | Insight panel |

**Pattern:** the deterministic primitives exist (diagnose/clean/insight/router/thread). The
**agentic layer** — respond, review, ask, remember, watch, reap, and the *contextual insight +
reasoned options after every action* ([[feedback_contextual_insights]]) — does not. That layer
is the gap.

---

## Part 3 — The requirement for Pantheon

1. **Agent-Operations Parity is a product principle:** anything the AI agent can do to operate
   this machine, the human operator can do through `sirsi` (CLI) **and** the menubar.
2. **Every operation is AI-optional** (the `sirsi insight` contract, generalized): a
   deterministic path that works with **zero** AI, and a **gemma-augmented** path that adds
   reasoning/narration when a local model is present — `--no-ai` always yields a usable result.
3. **Surface the work, not just the result:** after every action, show the insight and the
   reasoned next options (colleague, not menu) — in both CLI and menubar.
4. **Promote the shell primitives to first-class verbs:** `sirsi-respond.sh`, the watcher, the
   supervisor audit, the orphan-reap — these were authored as scripts today; they belong in
   `sirsi` proper so the operator (and the menubar) can invoke them.
5. **The menubar is the operator's cockpit:** Router, Threads, Review, Memory, Insight panels —
   each backed by the same CLI verbs, each working with or without gemma.

This is the substrate thesis applied to the operator: Pantheon already keeps the machine
*runnable*; this makes the operator *as capable as the agent that runs it*.

---

## Part 4 — Suggested next steps for Pantheon (triage)

- **ADR**: "Agent-Operations Parity — operator can do everything the agent does, AI-optional."
- **Phase 1 (promote existing scripts):** `sirsi router respond`, `sirsi watch`, `sirsi thread reap`, supervisor-audit as a built-in — all already prototyped as shell today.
- **Phase 2 (the colleague):** `sirsi ask` + `sirsi review` (deterministic + gemma), wired into menubar panels.
- **Phase 3 (memory):** `sirsi memory` surface.
- Each phase ships with the AI-optional contract and the post-action contextual-insight surface.

*Full evidence of the agent operations this requirement is derived from is Part 1 above.*
