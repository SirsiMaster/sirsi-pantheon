# ADR-119: Authority-bound native sleep controller

## Status
Proposed implementation —2026-10-20. Source implementation is authorized; no new issuer, enrollment, installed guard change or actual native sleep is authorized by this document.

## Context
The accepted9f8e6335 transaction and its Oct13/Oct15 hardware reviews were never integrated into a production controller. The installed legacy pre-tool guard denies raw power commands. NativeUI selection also timed out. Neither failure is a request for the operator to repeat their existing intent.

## Decision
Restore only the reviewed desktopcustody package on current main. Keep one shared transaction engine, an enrolled sleep-purpose signature verifier, live trusted workload custodian, native identity/observer binding and fixed Darwin command adapter. Every unresolved binding denies; the source contains no issuer generation/enrollment or production fake adapter. Recovery-purpose capabilities and SSH keys confer no sleep authority.

The signed permit authenticates exact request/wake, native platform/boot/executable, review digest, nonce, purpose and bounded validity. The existing enrolled authority must provide current sleep-specific policy/revocation through SleepIssuerLookup; accepting arbitrary caller public keys or a transport bearer as owner authority is prohibited. No deployed sleep issuer is yet demonstrated; this is a real qualification prerequisite, not a hidden fallback.

Persist nonce consumption across request IDs and restart, plus a durable host/boot fence. A partial write consumes authority conservatively. Confirmed pre-invocation failure may release only its own host fence; consumed nonces remain consumed. Invoked/unknown outcomes never silently free a host for another dispatch.

Native operations use only Apple's fixed executable and argument vectors, never shell interpolation, user-selected commands, sudo fallback or setting changes. Schedule one exact owned wake, read it back and only then permit the transaction's final custody callback to invoke native sleep once. Reject ambiguous local times, duplicate/substituted events and subsecond schedules. Native return is not physical sleep/wake proof. Unknown wake mutation or native outcome retains reconciliation state and never retries automatically.

SSA reviews exact source, integration and normal signing/release. SHA qualifies hardware/observer custody only after real authority adapters are configured and reviewed. No live power occurs in source tests.

## Alternatives considered
- Raw power commands or disabling the guard: rejected; bypasses the missing trust/transaction path.
- Reuse recovery grants or create a new issuer key: rejected; would expand authority rather than consume existing explicit sleep authorization.
- Whole legacy-branch merge: rejected; only reviewed package history is relevant.
- File-only request/nonce IDs: rejected; distinct requests and restart must not replay an authorization or bypass unknown host state.

## Consequences
An unconfigured installation denies rather than appearing ready. Source/fake-command tests qualify mechanisms, not deployed authority or real full wake. An operator-facing dispatch/return controller and actual runtime bindings remain required before product completion. Hard shutdown and FileVault pre-unlock recovery remain out of scope.

## References
SIRSI_MANIFESTO.md §§6,10,15; ADR-075 (separate proposed recovery authority); docs/plans/DESKTOP_CUSTODY_PRODUCTION_20261020.md; internal/desktopcustody/README.md; retained SHA sleep-contract-210900, sleep-review-171825, sleep-cure-172830 and sleep-publication-192032.
