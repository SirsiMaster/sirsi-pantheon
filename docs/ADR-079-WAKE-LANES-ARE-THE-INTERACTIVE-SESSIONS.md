# ADR-079 — Wake lanes are the interactive sessions; headless workers are ephemeral

- **Status:** PROPOSED (design only. Nothing in this ADR is built or deployed.) Rev 2 — addresses codex-pantheon CHANGES_REQUESTED on head `4dd44b70`: narrowed the watcher-retirement/attendance claims to Ra's observed case (§2), scoped D1 to an attendance/dispatch fence rather than delivery proof (§3 D1), named `session-message` as suppression-only metadata with no delivery path yet (§3 D2), and surfaced the `ConsumerCapable`/`armed` gap in D4 with a required close-session negative fixture (§3 D4, §6).
- **Date:** 2026-10-26 (rev 2026-10-27)
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
2. **Live probe:** `AttendedSessionLive`. **This probe did not pass for Ra's interactive Claude session (above).**
   - A `claude` surface maps to watcher type `loop-monitor`. `threadArmedForNewest` then requires `WatcherAliveForThread`, which means a running in-session watch loop for that thread id (internal/router/loopevidence.go, strand.go:35). `WatcherAliveForThread` is a union of three probes — thread-keyed, predecessor-keyed, and newest-agent-keyed watcher evidence (loopevidence.go) — any of which clears it; Ra's lane had none of the three live.
   - ADR-065 proposes retiring the per-lane watcher LaunchAgents, but that retirement is explicitly **gated separately per lane, not shipped fleet-wide**: "Retirement of any per-lane LaunchAgent is out of scope for Phase 1 and gated separately" (ADR-065, SSA build authorization 2026-10-07). So whether this probe passes is a per-lane fact, not a property of being a Claude session.
   - The result observed for Ra: this specific lane read **not attended** despite a fresh heartbeat, because no watcher process (thread-, predecessor-, or agent-keyed) was running for it. Other Claude lanes whose per-lane watcher has not yet been retired may still pass this probe today; this ADR does not claim otherwise. Codex sessions prove liveness by app heartbeat and are unaffected either way.

A third defect makes each duplicate worse:

3. **The consumer prompt defines a resident, not an ephemeral job.** Ra's `consumer.prompt` says: "Work your inbox as a LOOP… pull again. AN EMPTY INBOX ALONE IS NOT COMPLETION — always check the task registry and the ledger before you stop."
   - A spawned worker therefore keeps running until the 30-minute stall kill, instead of finishing its item and exiting.
   - `worker_answers: true` on Ra also lets that worker answer and close items on the lead's behalf.

## 3. Decision

The interactive session **is** the lane. A headless worker is a job the lane runs; it never becomes a second standing reader of the same inbox.

**D1. A live interactive session counts as attended on heartbeat.**
- In `AttendedSessionLive`, an interactive `claude`/`codex` thread is attended when it is active, not suspended, and heartbeat-fresh (`DefaultThreadStaleAfter`).
- It must not be a wake-loop worker thread, an API/observer surface, or any other non-session thread: only a thread whose registered surface is an actual interactive agent session (type `claude`/`codex`, `SessionMode: interactive`) is credited. A worker, API caller, or observer thread never counts as the attended consumer, however fresh its heartbeat.
- **Scope of this fence:** D1 decides whether a *second, dispatch-worthy* thread exists — it is an attendance/dispatch-suppression fence, not proof of message delivery. ADR-065 §3b already draws this line: *"'Connected' is not a heartbeat — it is a proven state... a heartbeat is necessary, never sufficient"* (A27/A36). A fresh heartbeat under D1 proves the thread is live, session-typed, and not a worker; it proves nothing about whether any given router item has reached or been read by that session. That delivery/read-ack gap is exactly what ADR-065 §3b already names unresolved, and D1 leaves it exactly as unresolved as it is today — it narrows one specific duplicate-dispatch race, not the delivery problem.
- This is the proof ADR-065 already assigns to interactive surfaces (§ table: "`claude`, `interactive` → existing session … `session-message`"). The fix removes the leftover dependency on a watch loop that, for lanes whose per-lane watcher has already been retired under ADR-065's per-lane gate, cannot be relied on (see §2, point 2).
- **Fail direction is unchanged, and is only this predicate's existing behavior:** an unreadable registry still reads as not attended. This is a narrow fail-closed default for one check, not a claim that D1 makes dispatch safely mutually exclusive in general — see §3's D4 note on `ConsumerCapable`/`armed`, which D1 alone does not resolve.

**D2. Lanes that have a lead session declare it.**
- Set `attended: true` and `wake.mechanism: session-message` on every lane whose owner works it interactively, and remove `worker_answers` from those lanes.
- Ra is the first. This is data only: the `wake-loop` LaunchAgent keeps running. It is **not parked, unloaded or restarted**. It logs `dispatch held — lane is declared attended` and keeps reporting depth.
- D1 alone would fix the race while a session is open. D2 also covers an idle-but-open lead (owner 2026-10-16: "secondary consumers should NEVER override the attended lane").
- **The `session-message` label is metadata, not a delivery mechanism.** Setting `wake.mechanism: session-message` on a registry row does not, by itself, push anything into a live interactive session — no code path that delivers a woken item *into* an open Claude/Codex session exists today. What D1+D2 actually implement is dispatch *suppression* (the wake loop declines to spawn a headless worker); the interactive session still has to notice and pull its own inbox, exactly as it does now. The missing concrete piece, if delivery-into-session is ever wanted, is a real `session-message` transport (the same open gap ADR-065 §3b already flags for delivery/read-ack in general). Acceptance evidence for this ADR is therefore scoped to suppression, not delivery: "zero dispatches while attended" (§6), never "the session was notified."

**D3. Headless workers are one-pass jobs.**
- Replace the "work as a LOOP / empty inbox is not completion" prompt on every lane with a one-pass contract: claim the items you were dispatched for, finish them with evidence, then **exit**.
- The wake loop's existing completion gate already stops it re-dispatching while a worker is running. With one-pass workers, the 30-minute stall kill becomes a backstop, not the normal way a worker ends.
- A worker answers router items only where the lead lane has granted `worker_answers` (unchanged rule from 2026-10-18).

**D4. No interactive session means needs-owner, not a spawn.**
- For a lane with `attended: true` and no live session, the wake is reported as `needs-owner` (CTR "wake-unavailable").
- This is ADR-065's existing boundary ("if no live session → **needs-owner**, stated not faked"). Nothing new is invented for it.
- Lanes the owner never works interactively stay headless backstops under D3. They are listed in §5 for the owner to confirm.
- **Open gap, not yet resolved by D1+D2: the wake-loop's own `ConsumerCapable` report is independent of attendance.** `RunWakeLoop` computes `capable := consumer != nil && !consumer.Resident` (wake.go:1243) from its *own* resolved headless consumer (e.g. the `claude --print` worker it can spawn) and publishes that on its *own* thread, regardless of whether an attended interactive session is currently live. `WakePass`'s `armed` selection (wake.go: the loop building the `armed` map, ~441) then credits any fresh `IsInboxConsumer` thread as armed. Nothing in D1–D3 changes either of these: the wake-loop thread stays `ConsumerCapable: true` and `armed` after the interactive session closes, because it is reporting its own continued ability to spawn a worker, not the session's presence. Left unresolved, `doctor`/`ctr`/`WakePass` can therefore report a lane as *armed* at the exact moment D4 says it should report *needs-owner* — D4's boundary is correct in intent but not yet backed by this path.
- **This is named here as an open question for the rollout, not decided by this ADR:** either (a) gate `ConsumerCapable` on attendance too (the wake-loop reports `capable: false` while D2 holds dispatch for an attended lane with no session), or (b) have the `doctor`/`ctr`/`WakePass` surfaces that implement D4 check attendance state directly instead of relying on `armed`/`ConsumerCapable` alone. Whichever is chosen, it must ship with a **close-session negative fixture** distinct from the `thread attended` check already in §6: close the attended session and assert that `WakePass`/`doctor` report `wake-unavailable`/`needs-owner` for that lane, not `armed`. Step 2 in §5 is not complete until this fixture exists and passes.

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
| 2 | D2 for **Ra only**: `attended: true`, `session-message`, drop `worker_answers`. Resolve the `ConsumerCapable`/`armed` gap named in D4 (either gate `ConsumerCapable` on attendance, or make `doctor`/`ctr`/`WakePass` check attendance directly). | `sirsi thread attended ra` → true while this session is live. `wake-ra.log` shows `dispatch held` and **zero** dispatches over 2 h with depth > 0. Close-session negative fixture (§6) passes: `WakePass`/`doctor` report `wake-unavailable`/`needs-owner`, not `armed`, once the session closes. |
| 3 | D3 one-pass prompt, Ra first. | Any dispatched worker exits 0 after closing its items, with no 143s. |
| 4 | Per-lane D2 for each lane whose lead confirms it is interactive. The candidate list comes from the thread registry (interactive `claude`/`codex` threads seen in the last 7 days), not from memory. Each lead confirms its own lane. | Same checks, per lane. |
| 5 | Lanes kept headless (no interactive lead): the owner confirms the list. | — |

Measure: dispatches per lane per hour, from `~/.sirsi/logs/wake-*.log`, before and after. The expected result for Ra is about 2 per hour falling to 0 while the attended session is open.

## 6. Pre-registered test (run at step 2)

- **Hypothesis:** with D1 and D2 applied, Ra's wake loop dispatches 0 headless workers while the attended session's heartbeat is fresh, at any inbox depth.
- **Pass:** 0 `dispatched consumer` lines in `wake-ra.log` during a 2-hour window with depth ≥ 1, and `thread attended ra` → true for the whole window.
- **Fail:** any dispatch in that window.
- **Control:** close the attended session. Within `DefaultThreadStaleAfter`, `thread attended ra` → false, and CTR reports Ra as `needs-owner` rather than spawning a worker.
- **Close-session negative fixture (additional, required before step 2 is considered done):** this control above only proves the `thread attended` predicate flips. It does not, by itself, prove `WakePass`/`doctor` stop reporting the lane as `armed` — that path reads `ConsumerCapable`/`armed` (wake.go ~441, ~1243), which D1–D3 do not touch (D4 note, §3). The fixture: with the attended session closed, assert `WakePass`'s armed selection and `doctor`'s surfaced state for Ra are `wake-unavailable`/`needs-owner`, not `armed`. Fail: either surface still reports armed with no live session.
