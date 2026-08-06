# Router store contracts

`routerstore` is Pantheon's CGO-free SQLite authority for messages, tasks,
leases, requirements, and dispatch evidence. Schema migrations are append-only;
opening the store applies each numbered migration under the existing serialized
writer contract and refuses a database newer than the binary understands.

Permanent execution adds two contracts:

- `Requirement` records one canonical obligation with stable source coordinates,
  lane ownership, optional task linkage, state, and typed evidence.
- `Store.Runnable(agent)` evaluates the R1 predicate without consulting process
  state: non-terminal messages OR actionable tasks OR unmet/in-progress
  requirements.

Heartbeat and PID state must never be added to `Runnable`. They describe whether
a session exists, not whether canonical work is executing. Callers needing lane
presentation combine this predicate with lease/task mutation evidence and must
classify a live but inactive lane as `IDLE_WITH_WORK`, not `WORKING`.

When adding requirement evidence kinds, update validation and tests together.
Terminal `verified` requirements must retain implementation, test, deployment,
and production evidence; `waived` requirements must retain a waiver.
