# ADR-119: Authority-bound native sleep controller

## Status
Proposed implementation —2026-10-20. Source implementation is authorized; no new issuer, enrollment, installed guard change or actual native sleep is authorized by this document.

## Context
The accepted9f8e6335 transaction and its Oct13/Oct15 hardware reviews were never integrated into a production controller. The installed legacy pre-tool guard denies raw power commands. NativeUI selection also timed out. Neither failure is a request for the operator to repeat their existing intent.

## Decision
Restore only the reviewed desktopcustody package on current main. Keep one shared transaction engine, one canonical online authority, a native workload custodian, an independent M1 observer and fixed Darwin command adapter. The preliminary Ed25519 permit prototype is removed rather than retained as a parallel fallback. Recovery-purpose capabilities and SSH keys confer no sleep authority.

The existing canonical service stores a finite management-created delegation in a reserved schema-25 state row. `CreateDesktopSleep` and `RevokeDesktopSleep` are concrete-backend operations absent from the Store/RPC interface. Only the service startup reads `SIRSI_MANAGED_SLEEP_DELEGATION`, supplied through existing authorized revision management, never a lane's environment. Exact reapplication is idempotent and cannot replace or revive a consumed/revoked record. Nonces are retained across request IDs; the bounded registry refuses further creation at 1,024 records rather than silently pruning authority history. Generic SetState/CAS cannot write the reserved key.

The delegation binds owner-request reference/digest, sleep-only request and exact wake, platform UUID/boot, executable and reviewed candidate digests, reviewed process/writer inventory, finite window/generation, exact enrolled token/session/machine/thread/runtime identities for coordinator, native custodian and independent M1 observer. The server injects authenticated actor identity into context; body role/identity strings confer nothing. Bootstrap and local-store callers cannot participate. The native participants require actual adopted UUIDs; an existing cloud coordinator remains bound to its exact logical host token/session without inventing a hardware identity. Typed requests additionally require clocks within five seconds. Freshness and revocation are checked against live records even when the general registration gate is configured in log mode.

One typed participant RPC supports status, inspection, fresh role readiness, consume, independent return observation and native return. Consumption and Ma'at reservation admission lock the same canonical transaction row across replicas. Every overlapping active/queued host or component reservation refuses sleep, apart from the exact management-bound controller reservation ID/holder/work. A consumed unresolved host refuses new reservations, including floor shares; expiry or revocation alone cannot clear that fence. Independent same-boot return evidence clears it; the original nonce remains consumed.

The native production client accepts only the release-compiled existing HTTPS origin `https://sirsi-router-6kdf4or4qq-uc.a.run.app` through the existing authenticated RemoteStore client/transport, retaining its pin/proxy policy and refusing redirects, local stores and arbitrary/spool endpoint overrides. No new client, TLS root, IAM principal or key enrollment is introduced. A deployment that only exposes a spool transport cannot dispatch power until its existing privileged native custody service is correctly bound to the enrolled HTTPS transport; a client fallback must not manufacture that binding.

Persist nonce consumption across request IDs and restart, plus a durable host/boot fence. A partial write consumes authority conservatively. Confirmed pre-invocation failure may release only its own host fence; consumed nonces remain consumed. Invoked/unknown outcomes never silently free a host for another dispatch.

Native operations use only Apple's fixed executable and argument vectors, never shell interpolation, user-selected commands, sudo fallback or setting changes. Schedule one exact owned wake, read it back and only then permit the transaction's final custody callback to invoke native sleep once. Reject ambiguous local times, duplicate/substituted events and subsecond schedules. Native return is not physical sleep/wake proof. Unknown wake mutation or native outcome retains reconciliation state and never retries automatically.

SSA reviews exact source, integration and normal signing/release. SHA qualifies hardware/observer custody only after real authority adapters are configured and reviewed. No live power occurs in source tests.

Native identity uses actual IOPlatformUUID, boot-session UUID and running executable hash. Complete process/open-write inventory requires the existing privileged custody service; no sudo/elevation fallback is added. The reviewed inventory digest, OS sleep assertions and fresh three-party custody all participate; a digest match alone is not a declaration that active payloads are idle. The M1 observer takes a real enrolled-SSH boot baseline, refreshes readiness, and records bounded loss/return without treating login/key denial as sleep. Native return additionally requires actual Sleep/full-Wake records, unchanged boot/executable and process/writer continuity. DarkWake or reachability alone cannot qualify.

`reconcile` is an explicit return-only operation, not a dispatch retry. A retained unknown journal can resolve only if native invocation and owned-wake readback were already persisted and the same canonical/native/independent proof succeeds. Unknown wake mutation before invocation cannot become a fabricated successful sleep. An uncertain canonical consumption without native invocation retains its fence for management incident review; it is never automatically freed or replayed.

## Alternatives considered
- Raw power commands or disabling the guard: rejected; bypasses the missing trust/transaction path.
- Reuse recovery grants or create a new issuer key: rejected; would expand authority rather than consume existing explicit sleep authorization.
- Whole legacy-branch merge: rejected; only reviewed package history is relevant.
- File-only request/nonce IDs: rejected; distinct requests and restart must not replay an authorization or bypass unknown host state.

## Consequences
An unconfigured installation denies rather than appearing ready. The operator CLI and production adapters are source-implemented; deployment, actual participant/service privilege bindings, native log-format acceptance and a fresh physical window remain separate qualification. Source/fake-command tests do not prove deployed authority or real full wake. Hard shutdown and FileVault pre-unlock recovery remain out of scope. This proposed ADR and source documentation have not yet been mirrored into the owner Reading Room or Workspace; publication remains pending independent review, not silently complete.

## References
SIRSI_MANIFESTO.md §§6,10,15; ADR-075 (separate proposed recovery authority); docs/plans/DESKTOP_CUSTODY_PRODUCTION_20261020.md; internal/desktopcustody/README.md; retained SHA sleep-contract-210900, sleep-review-171825, sleep-cure-172830 and sleep-publication-192032.

## Actual supervisor budget and planned continuity

The normal supervisor publishes one private, create-only execution fact before
spawning its own exact argv. It records real admission/work/thread, supervisor
PID and kernel start signature, exact argv hash and bounded execution lifetime.
This is evidence, not a grant: the native client verifies its actual parent,
start, argv and private root-owned file; management freezes the fact and digest
inside the canonical delegation. Caller-created files, agent labels and global
PID exemptions confer no authority. Ordinary jobs retain the same admissions,
signals, deadlines and cleanup behavior; no heartbeat is fabricated.

Creation, preflight and consumption check the actual canonical reservation and
confirmed heartbeat/TTL. Both the real lease and conservative execution deadline
must cover the lifecycle plus margin; the job bound is anchored to the service's
reservation creation time and the actual published budget, not just client wall
time. The consumed record retains the original confirmed controller-lease end.
Native v1 is at most180seconds to scheduled wake and210seconds total lifecycle,
and still refuses if the actual remaining budget is shorter. The normal current
supervisor requests240second leases; that fact is checked, never assumed.

The dispatcher remains alive across native sleep and independent return. It
repeats only bounded status reads, never consumption or power. Pre-consume
participant freshness remains90seconds and three-party readiness15seconds. After
wake, a stale participant is still refused until its real, live supervisor
refreshes through the normal PID/start-verified path. Native same-boot/source,
process/writer continuity and independent observer/full-wake proof are then
required before success. An invoked unknown can resolve through return-only
reconciliation with its original error retained; no invocation retry occurs.
If the original lease/job budget or custody identity is lost, continuation is
an incident-review case, not a fabricated success or a new grant.

Service management may also set typed nonsecret
`SIRSI_MANAGED_SLEEP_REVOCATION` (ID and reason) at startup. It cannot remove an
unresolved consumed host fence. Removing configuration or restarting a revision
cannot reset a consumed/revoked record. Only reviewed new code may serve mutation
traffic before policy activation; draining/removing legacy mutation-serving
revisions and preserving the canonical state are release prerequisites. Do not
roll back to a fence-unaware writer while a consumed request is unresolved.

No live delegation, native power, privilege enrollment or new cloud worker is
created by these source changes. Reading Room/Workspace publication remains
pending review; physical and warm-cloud overlap evidence remain unperformed.
