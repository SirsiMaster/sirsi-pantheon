# ADR-079 — Wake lanes are the interactive sessions; headless workers are ephemeral

- **Status:** PROPOSED (design only. Nothing in this ADR is built or deployed.)
- **Date:** 2026-10-26
- **Owner directive (decision card, 2026-10-26):** "Wake lanes should always be the interactive sessions. Other sessions are ephemeral work sessions that close."
- **Author:** Ra. **Reviewer:** codex-pantheon (no self-review).
- **Router task:** `ra/wake-lane-targets-interactive-session`
- **Relates to:** ADR-054 (wake matrix), ADR-065 (router-owned informer; lanes carry no arming logic), #636 (runaway spawn), the attended fence (owner 2026-10-16), worker answering roles (owner 2026-10-18).

## 1. What happened

On 2026-10-26, `ai.sirsi.router.wake.ra` started four headless `claude --print` workers in about two hours. They started at 19:15, 19:51, 20:06 and 21:02 local, at inbox depths of 1 to 6. The whole time, the attended Ra session (`thr-e5ed79be62671cda`) was live and working the same inbox. Some items were reviewed twice. One worker sat for 30 minutes without taking any durable action and was stall-killed (`exit 143`). Worker thread `thr-f27673b7a05c76f4` looked like a second Ra, which misled the attended session itself.

Live evidence, captured while the attended session was in use:

```
$ sirsi thread attended ra --json
{ "agent_id": "ra", "attended": false }        # exit 1
```

## 2. Root cause: three defects that each let the duplicate through

The router already has a mechanism meant to prevent this: `attendedSessionOwnsInbox` (internal/router/wake.go). It holds dispatch when either check passes:

1. **Declared fence:** `agents.json` sets `attended: true`. **Ra's entry does not.** Only codex-apollo, codex-pantheon, mercury, SSA and SHA carry it.
2. **Live probe:** `AttendedSessionLive`. **This probe cannot pass for an interactive Claude session.**
   - A `claude` surface maps to watcher type `loop-monitor`. `threadArmedForNewest` then requires `WatcherAliveForThread`, which means a running in-session watch loop for that thread id (internal/router/loopevidence.go, strand.go:35).
   - ADR-065 deliberately removed that arming logic from lanes, so no interactive Claude session runs such a loop any more.
   - The result: every interactive Claude session on the fleet reads as **not attended**, however fresh its heartbeat. The hold is dead code for Claude lanes. Codex sessions are unaffected because they prove liveness by app heartbeat.

A third defect makes each duplicate worse:

3. **The consumer prompt defines a resident, not an ephemeral job.** Ra's `consumer.prompt` says: "Work your inbox as a LOOP… pull again. AN EMPTY INBOX ALONE IS NOT COMPLETION — always check the task registry and the ledger before you stop."
   - A spawned worker therefore keeps running until the 30-minute stall kill, instead of finishing its item and exiting.
   - `worker_answers: true` on Ra also lets that worker answer and close items on the lead's behalf.

## 3. Decision

The interactive session **is** the lane. A headless worker is a job the lane runs; it never becomes a second standing reader of the same inbox.

**D1. A live interactive session counts as attended on heartbeat.**
- In `AttendedSessionLive`, an interactive `claude`/`codex` thread is attended when it is active, not suspended, and heartbeat-fresh (`DefaultThreadStaleAfter`).
- It must not be a wake-loop worker thread: the thread was not registered by the lane's own `wake-loop`, and it carries no consumer lease.
- This is the proof ADR-065 already assigns to interactive surfaces (§ table: "`claude`, `interactive` → existing session … `session-message`"). The fix removes the leftover dependency on a watch loop that ADR-065 abolished.
- **Fail direction is unchanged:** an unreadable registry still reads as not attended.

**D2. Lanes that have a lead session declare it.**
- Set `attended: true` and `wake.mechanism: session-message` on every lane whose owner works it interactively, and remove `worker_answers` from those lanes.
- Ra is the first. This is data only: the `wake-loop` LaunchAgent keeps running. It is **not parked, unloaded or restarted**. It logs `dispatch held — lane is declared attended` and keeps reporting depth.
- D1 alone would fix the race while a session is open. D2 also covers an idle-but-open lead (owner 2026-10-16: "secondary consumers should NEVER override the attended lane").

**D3. Headless workers are one-pass jobs.**
- Replace the "work as a LOOP / empty inbox is not completion" prompt on every lane with a one-pass contract: claim the items you were dispatched for, finish them with evidence, then **exit**.
- The wake loop's existing completion gate already stops it re-dispatching while a worker is running. With one-pass workers, the 30-minute stall kill becomes a backstop, not the normal way a worker ends.
- A worker answers router items only where the lead lane has granted `worker_answers` (unchanged rule from 2026-10-18).

**D4. No interactive session means needs-owner, not a spawn.**
- For a lane with `attended: true` and no live session, the wake is reported as `needs-owner` (CTR "wake-unavailable").
- This is ADR-065's existing boundary ("if no live session → **needs-owner**, stated not faked"). Nothing new is invented for it.
- Lanes the owner never works interactively stay headless backstops under D3. They are listed in §5 for the owner to confirm.

## 4. What this does NOT do

- It does not park, unload, restart or delete any wake lane, LaunchAgent, session or thread, and it prunes nothing.
- It does not deploy #639 or change the spawn ceiling.
- It does not touch Horus (killed by the owner on 2026-10-09).
- It does not change the codex lanes' heartbeat proof.
- It does not change the router service, the database or Cloud Run. D1 is client-side `internal/router` logic, and D2 is registry data.

## 5. Rollout (after review; freeze rules apply: these are fixes)

| Step | Change | Proof before moving on |
|---|---|---|
| 1 | D1 code and tests. `AttendedSessionLive` is true for a fresh interactive Claude thread with no watch loop, and false for a wake-loop worker thread. Negative control: revert → test fails. | `go test ./internal/router/` |
| 2 | D2 for **Ra only**: `attended: true`, `session-message`, drop `worker_answers`. | `sirsi thread attended ra` → true while this session is live. `wake-ra.log` shows `dispatch held` and **zero** dispatches over 2 h with depth > 0. |
| 3 | D3 one-pass prompt, Ra first. | Any dispatched worker exits 0 after closing its items, with no 143s. |
| 4 | Per-lane D2 for each lane whose lead confirms it is interactive. The candidate list comes from the thread registry (interactive `claude`/`codex` threads seen in the last 7 days), not from memory. Each lead confirms its own lane. | Same checks, per lane. |
| 5 | Lanes kept headless (no interactive lead): the owner confirms the list. | — |

Measure: dispatches per lane per hour, from `~/.sirsi/logs/wake-*.log`, before and after. The expected result for Ra is about 2 per hour falling to 0 while the attended session is open.

## 6. Pre-registered test (run at step 2)

- **Hypothesis:** with D1 and D2 applied, Ra's wake loop dispatches 0 headless workers while the attended session's heartbeat is fresh, at any inbox depth.
- **Pass:** 0 `dispatched consumer` lines in `wake-ra.log` during a 2-hour window with depth ≥ 1, and `thread attended ra` → true for the whole window.
- **Fail:** any dispatch in that window.
- **Control:** close the attended session. Within `DefaultThreadStaleAfter`, `thread attended ra` → false, and CTR reports Ra as `needs-owner` rather than spawning a worker.
