# RA-P15 — Claim eligibility (`router task why`)

Owner: Ra. Source: `internal/routerstore/taskeligibility.go`, `cmd/sirsi/routerledgercmd.go` (`routerTaskWhyCmd`). Revision: main `0ed15ac9`.

A read-only answer to "why would a claim of this task be refused?" It exposes the fields the plain task list omits — holder, lease expiry, attempts against the ceiling, failure reason, and the `blocked_by` dependency's own state — and names every cause that currently blocks a claim, most decisive first. Built so a worker or an auditor reads the refusal instead of probing it with live claim attempts (each of which counts as a failed attempt against the retry ceiling).

## Logical view
```mermaid
flowchart TD
  CLI[sirsi router task why agent task-id] --> READ[TaskEligibility: read-only row + dependency row]
  READ --> DEP{blocked_by set?}
  DEP -- empty --> DEPOK[dependency satisfied]
  DEP -- task id found --> DEPSTATE[blocked_by_state = task:status; done only if status=done]
  DEP -- no matching task row --> EXT[blocked_by_state = external-reason, free-text block]
  READ --> LEASE{lease_token set?}
  LEASE -- empty --> NOLEASE[not held]
  LEASE -- set, past expiry --> EXPIRED[EXPIRED - next claim pass reclaims it, counts as failed attempt]
  LEASE -- set, live --> HELD[live lease by claimed_by/thread_id until expiry]
  READ --> ATTEMPTS{attempts >= MaxRetriesPerItem?}
  DEPOK --> CLAIMABLE
  DEPSTATE --> CLAIMABLE{status in pending/in-progress AND dependency done AND no live lease AND attempts below ceiling}
  NOLEASE --> CLAIMABLE
  ATTEMPTS --> CLAIMABLE
  CLAIMABLE -- yes --> RESP{responsible_party is '' or 'self' or this agent?}
  RESP -- yes --> DISPATCHABLE[Dispatchable: a wake loop will start a worker for it]
  RESP -- no --> CLAIMNOTDISP[Claimable but not dispatchable - owner/other-party assigned]
  CLAIMABLE -- no --> REFUSED[Not claimable - Reasons lists every cause]
```
## Data view
```mermaid
flowchart LR
  TASKROW[(tasks row: status, responsible_party, blocked_by, claimed_by, thread_id, lease_token, lease_expires, attempts, failure_reason)] --> ELIG[TaskEligibility]
  DEPROW[(blocked_by's own tasks row, if it is a task id)] --> ELIG
  ELIG --> REASONS[Reasons: ordered, human-readable, never the lease token]
  REASONS --> CLI2[CLI text or --json]
```

## Failure and recovery
- Never mutates anything and never prints the lease token — a strictly read-only probe, safe to run speculatively instead of a real claim attempt.
- `blocked_by` pointing at a non-existent task id is **not** a validation defect (rs-39, A35-corrected): it is the designed external-reason form, cleared by hand with `task update --blocked-by ""` (no lease needed) once the real-world blocker clears.
- `Claimable` and `Dispatchable` are deliberately two different booleans: a task can be claimable (the state machine would allow it) yet not dispatchable (assigned to the owner or another party) — a wake loop only auto-starts a worker for the latter.
- An expired-but-still-recorded lease is reported distinctly from a live one; the next claim pass reclaims it and counts it as a failed attempt toward the retry ceiling, not a free retry.
