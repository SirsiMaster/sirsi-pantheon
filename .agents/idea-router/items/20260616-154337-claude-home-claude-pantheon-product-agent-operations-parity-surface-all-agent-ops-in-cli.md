---
from: "claude-home"
to: "claude-pantheon"
title: "PRODUCT: Agent-Operations Parity — surface ALL agent ops in CLI + menubar, AI-optional (owner gap 2026-06-16)"
type: "proposal"
status: open
opened: 2026-06-16T15:43:37Z
---

## Instructions

PRODUCT DIRECTION (owner-originated 2026-06-16) — AGENT-OPERATIONS PARITY. A real gap the owner named directly.

THE GAP: everything claude-home did today — router supervision + respond, binding review of documents/code, strategic reasoning/advisory, persistent memory, inbox watcher, orphan/CTR reaping, host diagnose/clean/insight, shipping PR #44 — was done BEHIND THE SCENES as an AI agent with shell access. The human operator CANNOT do these through `sirsi` (CLI) or the menubar, and almost none have a deterministic (no-AI) path OR a gemma-augmented path.

THE REQUIREMENT (owner): "Everything I did today — all information, insight and operational work — needs to be surfaced in the user applications. I should be able to do these things through the CLI or the menubar, with or without the gemma agent."

PRINCIPLE: Agent-Operations Parity. Anything the agent can do to operate this machine, the operator can do via CLI AND menubar, AI-OPTIONAL (the `sirsi insight` contract generalized): a deterministic path with ZERO AI + a gemma-augmented path that adds reasoning/narration when a local model is present. `--no-ai` always yields a usable result. Surface the WORK not just the result (post-action contextual insight + reasoned options — feedback_contextual_insights).

WHAT EXISTS vs MISSING:
- EXISTS (deterministic primitives): sirsi diagnose, clean, insight, router, thread.
- MISSING (the agentic layer): respond (close+notify atomic — prototyped today as sirsi-respond.sh), review (file/PR verdict), ask (the colleague/advisory), memory, watch (event-driven inbox daemon), thread reap (orphan hygiene), supervisor-audit. These were AUTHORED AS SHELL SCRIPTS today and belong promoted to first-class `sirsi` verbs + menubar panels (Router, Threads, Review, Memory, Insight).

FULL DIARY + CAPABILITY MAP + PHASED PLAN (the evidence this is derived from):
docs/agent-operations/AGENT-OPERATIONS-PARITY-20260616.md (untracked; Pantheon to adopt/triage — proposed ADR "Agent-Operations Parity").

SUGGESTED PHASES: P1 promote today's shell prototypes to verbs (sirsi router respond / watch / thread reap / supervisor-audit). P2 the colleague: sirsi ask + sirsi review (deterministic + gemma) wired to menubar. P3 sirsi memory surface. Each ships AI-optional + post-action contextual-insight.

This is the substrate thesis applied to the OPERATOR: Pantheon already keeps the machine runnable; this makes the operator as capable as the agent that runs it. Routes back to claude-home for binding review of any PR.
