---
name: ctr
description: Check The Router — surface pending router items and wake stranded agent threads. Run when you (or the owner) says "ctr" / "check the router", on session start, or whenever the router may have unread items.
---

You are running CTR ("Check The Router") for this workstation.

CTR is the universal, on-demand wake primitive: one synchronous pass, no daemon.
It surfaces every open router inbox item and wakes agents that have work waiting
but no live watcher. It is safe to run repeatedly.

Steps:

1. Run `ctr` (or `sirsi ctr` if the shim is not on PATH). For machine
   consumption use `sirsi ctr --json`.
2. Read the output:
   - "Woke this pass" — agents a wake was just attempted for.
   - "Stranded" — open items with NO live watcher; that agent's session must run ctr.
   - "Heartbeat-fresh" — a session is checked in, but fresh != consuming; if its
     items still aren't moving, THAT session must run /ctr to pull them.
   - "Needs owner — wake-unavailable" — a closed interactive session that local
     code cannot resurrect; surface these to the owner plainly, do not fake them.
3. If any surfaced items are addressed to YOUR agent id, act on them now: read
   each with `sirsi router show <id>`, do the work, then `sirsi router close <id>`
   (or route a response back). Then stop.
4. Report a one-line summary: how many pending, how many woke, how many need the
   owner.

Notes:
- CTR never blind-spawns interactive sessions; "needs-owner" is honest, not a failure.
- The router is found from the current repo, or from ~/.sirsi/pantheon-repo, so
  `ctr` works from any directory.
