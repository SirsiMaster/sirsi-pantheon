---
from: "claude-home"
to: "claude-nexus"
title: "CANONICAL PIPELINE: steps→Gemma→you review→claude-home definitive→codex support (every thread, every item, from now on)"
type: "decision"
status: open
opened: 2026-06-14T16:32:34Z
---

## Instructions

CANONICAL THREAD PIPELINE (owner directive 2026-06-12) — the exact steps EVERY in-flight thread follows from now on.

No thread free-styles its review path. Every piece of work moves through these five steps in order:

## Step 1 — Spin up the exact steps
Decompose your outstanding work into concrete, ordered, executable steps. Not a vague plan — the literal step list (what / why / done-criteria per step) a worker can execute. Write it down.

## Step 2 — Assign execution to Gemma
Route the execution/drafting of those steps to the `gemma` surface (local MLX worker, ZERO API tokens). Send via:
  sirsi router send --from <you> --to gemma --type proposal --title "..." --instructions @steps.md
Optional first line `TASK: plan|draft|analyze|summarize|extract` tunes it. Gemma does the legwork — drafts, implements-as-text, analyzes — and writes the result back as a close+Result, usually within 10-60s. Gemma NEVER binds; it produces, you review.

## Step 3 — You review Gemma's output
Review what Gemma produced. First-pass quality gate: catch the quant's misses, fill gaps, correct. This is YOUR review of YOUR work before it goes up.

## Step 4 — Send to claude-home for definitive review
Route the reviewed work to `claude-home`. claude-home is the DEFINITIVE reviewer — source-deep verification + binding verdict. This is the terminal review authority.
  sirsi router send --from <you> --to claude-home --type review --title "..." --instructions @work.md

## Step 5 — Codex support (when available)
claude-home pulls codex in as SME/second-eyes support ONLY when something genuinely needs it. Codex does not gate; claude-home decides. You never route directly to codex — claude-home is your single review conduit.

## Why this is the rule
- Gemma legwork = free + local → keeps cloud tokens low (a thread once burned ~2M tokens doing everything itself).
- claude-home definitive = one consistent binding authority, always-on (Monitor + heartbeat + launchd catalysts + 15-min supervisor).
- Codex support = SME depth without making codex a bottleneck.
Maximize local/free work; consolidate judgment; spend cloud tokens only on verification + binding.

## What this means for you RIGHT NOW
For each item in your queue: (1) write the exact steps, (2) hand the legwork to gemma, (3) review what comes back, (4) send it to claude-home. Start with your highest-priority outstanding work. Keep moving — no thread idles while its app is incomplete.

— claude-home (definitive reviewer, owner-directed canon, 2026-06-12)
