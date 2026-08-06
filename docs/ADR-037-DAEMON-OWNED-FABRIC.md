# ADR-037: Daemon-Owned Fabric — the Ship-Complete Control Plane

> **Implementation status (ADR-057 amendment):** this document states the target architecture and completion law. Universal transition emission, fresh-install provisioning, adapter conformance, installed-runtime verification, and cross-surface production acceptance are not current facts until the ADR-057 evidence gate records them.

## Status
**Accepted** — 2026-07-10 · claude-pantheon. Custodian: 𓁢 the Router. Extends ADR-035 (runaway-proof execution) and ADR-036 (Router v2 durable dispatch); governs PANTHEON_RULES A26/A27 and the Orchestration Brain PRD (`docs/prd/ORCHESTRATION_BRAIN.md`, Tier-0 invariant). Answers the owner's question (2026-07-10): "what is the final architecture, and how do we ship it so the user never has to converse with the app to fix it?"

## Context

The fabric that dispatches work between agents was, until the Router v2 cutover, held together by things a shipped product cannot depend on:
- **Wake was a conversational ritual.** Each surface was *told in prose* to "arm /loop watching items/" (watcherspec.go emitted the instruction; a SessionStart hook re-asserted it). Liveness depended on an LLM session obeying an instruction, not on a running daemon.
- **Authority was bypassable.** The `items/*.md` files were a writable inbox; any process — or a stale ritual — could inject work the dispatch layer never saw. ADR-035 axiom 1: *a dispatch layer whose authority can be bypassed by any file write cannot bound anything.*
- **Fixes happened in conversations.** Stranded watchers, migration races, and surface bugs were repaired by talking to an agent. A downloaded copy has no such conversation.

A public Pantheon must run the fabric out of the box, at Level 0 (deterministic, no AI configured), and self-heal — with zero conversations. That is the bar this ADR sets and the completion-proof it demands.

## Decision

**One deterministic authority with thin adapters. Nothing load-bearing is a conversation or an LLM.**

### 1. Control plane — a Tier-0 daemon, not a model, not a session
- The **SQLite store** (`~/.sirsi/router.db`, outside every git tree) is the **sole** dispatch authority: no store row, no dispatch (§2b axiom 8). Idempotency, quotas, breakers, and leases are *database invariants*, not application promises.
- The daemon owns the **wake FIFO** (<250ms, PRD /goal #1), heartbeat, liveness, and reaping — all deterministic, no LLM. "The eternal loop is a daemon; models are invoked, not resident loops" (Orchestration Brain PRD).
- It ships as a launchd LaunchAgent (RunAtLoad + KeepAlive), installed by `sirsi setup` at first-run — **never armed by a conversation.**

The resident Go runtime is also the sole authority for deciding whether an
agent lane may park. It evaluates the canonical three-source predicate defined
by ADR-057: an actionable router item, an actionable ledger task, or an unmet
traced canon requirement makes the lane runnable. Process existence, a thread
heartbeat, a prompt instruction, and a model's statement that it is finished
are not inputs to that predicate. A lane may park only after the store proves
all three sources audited empty.

Workers execute through fenced store transitions:

`pull -> lease -> bind thread/task -> execute -> record evidence -> close -> pull again`.

The daemon leases wake delivery and executable work independently, expires
orphaned leases, retries delivery within a bounded policy, and accepts a wake
acknowledgment only when the worker performs a real source-store action. Merely
starting a process or refreshing a session heartbeat never acknowledges work.

### 2. Adapters are thin — MCP, CLI, and hooks are I/O onto the one facade
Not "fully MCP" and not "fully hooked." Both are *adapters*, holding no state or logic:
- **MCP** (`router_*`) — the adapter for LLM agents that speak the tool protocol.
- **CLI verbs** (`sirsi router wait/send/pull/show/close/status`) — the adapter for any shell/headless consumer. `router wait` blocks on the store FIFO; every read verb reads the dual-read union so a store-only item is never invisible.
- **Hooks** (SessionStart/UserPromptSubmit) — the adapter for lifecycle events (assert liveness, route).

All three call `internal/dispatch` (the one facade). Surfaces (menubar / TUI / Nexus) **read** daemon state; their buttons call the same verbs. No surface holds logic.

LLM vendors and user interfaces are adapter details. Claude, Codex, Gemini,
OpenAI-compatible APIs, local model runners, and future providers participate
through capability-declared wake and consumer adapters. Provider names must not
appear in the control-plane state machine. An adapter is conformant only when it
can prove delivery, claim acknowledgment, lease renewal, reconstitution, and
truthful completion through the shared store contract.

### 3. The cutover this ADR completes
Wake moves off the `items/` directory-watch onto the store (`sirsi router wait`); `Send` stops writing the `items/<id>.md` audit view; `Show/Pull/Status/Close` read/close from the store when no file exists. Gated behind `SIRSI_ROUTER_STORE_WAKE` (default off) so a binary ships identical-to-before until the flag is flipped **after** the wake verb is in the running binary and live watchers are re-armed (ADR-036: an owner-visible step). The files, once demoted, are an optional, gitignored, non-authoritative audit view.

### 4. Ship-complete: how it's correct without future conversations
- **First-run install provisions everything** — `sirsi setup` creates the store, installs the daemon, sets Brain Level 0, and handles all FDA/TCC at install (not mid-use). A fresh download dispatches/routes/heartbeats/wakes with zero AI configured.
- **Self-healing replaces conversation** — `sirsi doctor` / `sirsi brain doctor` diagnoses a dead daemon, missing model, bad auth, or a stranded item in plain English and offers the one-click remediation lever (monitor→identify→**FIX**, ADR-033). The user clicks a lever or the daemon self-heals; they never converse to fix.
- **The completion-proof (the law that keeps us honest):** *any capability exercised in a design conversation must become a shipped, deterministic, test-enforced lever before it counts as done.* If something only works because a conversation did it, it is a product bug — it must become a `sirsi` verb + a menubar button + a doctor check. Enforced by CI + the Ma'at gate + a first-run E2E test (fresh install → dispatch works at Level 0). This is the Commercialization Gate applied to the fabric: green tests are necessary, not sufficient; product/technical/operational closure is.
- **Continuous-execution proof:** fresh-install and failure-injection tests must
  create work in each authoritative source, exercise every supported adapter,
  terminate or stall the worker, and observe lease expiry, re-wake, a real claim,
  continued work mutation, and evidence-backed completion without an owner nudge.
  A source-only test or a live-process screenshot is not operational proof.

## Alternatives Considered
1. **Fully-MCP fabric (every agent + the loop over MCP).** Rejected: MCP is a participation protocol for LLM tool-callers, not a control plane; it can't be the authority for headless consumers or the deterministic daemon, and a shipped app can't require a model to run its loop.
2. **Fully-hooked fabric (lifecycle hooks carry dispatch).** Rejected: hooks are event edges, not a store; they can't hold leases/quotas/liveness, and they run only when a session fires them — no session, no fabric.
3. **Keep the conversational arm ritual, harden the workers.** Rejected: the runaway-executor incident proved bypassable/instructed authority relocates failure instead of removing it (ADR-035 axiom 1). Only a daemon-owned store can bound the fabric.
4. **Keep dual-writing files forever as the audit trail.** Retained as an *option* (gitignored, non-authoritative), not the authority — the store is the record; the file view is a convenience, toggled off by the cutover.

## Consequences
The fabric becomes a product: a deterministic daemon + store, thin adapters, self-describing/self-healing surfaces, provisioned at install. The remaining work is execution, not architecture — flip the cutover on the running fabric (verified live), and hold every future fabric capability to the completion-proof above.
