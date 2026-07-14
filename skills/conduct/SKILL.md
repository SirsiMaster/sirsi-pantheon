---
name: conduct
description: Run the Sirsi Pantheon orchestration loop unattended — the configured brain (sirsi-brain) decides, this app session executes. Use when the owner wants Pantheon to keep running (triage inboxes, route builds, bind low-risk PRs, escalate owner-gated) without prompting each step. Pair with /loop for continuous unattended operation.
---

# Conduct — brain leads, app executes

You are the EXECUTOR for the orchestration brain. Each cycle:

1. Run `sirsi-conductor cycle` to get the brain's parsed ACTION PLAN. (The brain is
   whatever `sirsi-brain which` reports — gemma when a clean model + RAM are available,
   else `provider=claude` = YOU produce the plan directly from `sirsi-conductor state`.)
2. If `provider=claude` (or the gemma plan is empty/garbled), produce the action plan
   YOURSELF from `sirsi-conductor state` using the SAME grammar:
   ROUTE <id> <agent> | CLOSE <id> | <reason> | ACK <id> | <reply> | BIND <pr#> |
   BUILD <id> | ESCALATE <id> | <why> | NOOP
3. EXECUTE each action with the real tools (Max-plan auth that works):
   - ROUTE/ACK/CLOSE → `sirsi router send` / `sirsi router close`
   - BIND → source-deep review the PR, then merge if green + low-risk (YOUR judgment — never delegate the bind)
   - BUILD → build it yourself in a worktree, or route to its owning thread
   - ESCALATE → surface to the owner concisely; do NOT act on credentials/console/risk
   - NOOP → nothing this cycle
4. Living-Horus guardrail every few cycles: `sirsi diagnose`; if RAM is tight, relieve
   (e.g. idle iOS sim) and back off spawning workers — never trigger Jetsam.

Honesty: BIND and BUILD are judgment/agentic — only YOU (the app) do them; the brain
only decides WHAT to do, not the binding verdict. Owner-gated items ESCALATE, never act.

To run unattended: `/loop /conduct` (the keepalive + loop keep cycling without prompting).
Switch the brain painlessly anytime: `sirsi-brain use gemma|claude|gemini|openai`.
