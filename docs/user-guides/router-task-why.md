# Understand what `router task why` does—and does not—check

`sirsi router task why <agent> <task-id>` reads the existing task ledger. It shows
task status, dependency, lease holder/expiry, attempts and recorded failure reason
without claiming/resetting work or printing its lease token.

The diagnostic checks **task-row conditions only**. A `claimable: true` result is
not permission or a promise that your current session can claim. `dispatchable`
also considers who is responsible for the row; it is not execution permission.
Service authorization and actual caller/session claim guards still apply.

New JSON responses include:

```json
{"scope":"task-row","caller_evaluation":"not-evaluated"}
```

These fields describe the scope of the diagnostic, not a new authorization check.
The existing `claimable` and `dispatchable` meanings/values have not changed.

Plain text states the boundary before the verdict. If an older service omits
scope fields—or returns partial/unrecognized scope—the CLI warns that scope is
unknown and caller/session eligibility is **not confirmed**. It does not infer
that omitted checks passed. Legacy service reasons remain reported service text,
not an endorsement that a claim will succeed. JSON mode stays valid JSON and
preserves empty legacy metadata as unknown.

Do not probe this distinction with claim/reset attempts or by adopting another
session. Normal registration of your own actual runtime remains supported; this
diagnostic adds no registration action and changes no authentication or claim
guard. The current-main composition has not been tested, built, installed or
deployed. Separate old-base focused tests and the M5 caller-binding operational
recovery do not constitute qualification of this diagnostic composition.
