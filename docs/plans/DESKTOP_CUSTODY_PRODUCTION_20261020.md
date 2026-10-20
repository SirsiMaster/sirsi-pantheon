# Production desktop custody implementation

/goal: Ship one supported, authenticated, recoverable sleep-only controller rather than bypassing the installed guard. Actual power qualification remains a separate fresh hardware window.
/plan:
1. Selectively restore accepted transaction package9f8e6335 onto current main22944b24; preserve its source provenance and unknown-outcome journal semantics.
2. Implement admission/custody adapters over existing authenticated authority and runtime bindings; existing service-authenticated actor identity, exact host/boot/request/action/window/source/review binding, durable cross-request nonce consumption. Missing enrollment/configuration denies; no issuer minting, grant-file fabrication or production test doubles.
3. Implement fixed Darwin command adapter, exact owned scheduled-wake readback/cancel and native identity/event inspection. No shell interpolation, user command argument, reboot/shutdown, setting change, hook change or live power from source tests.
4. Expose explicit preflight/status/dispatch/return operations through existing CLI/shared engine. Keep recovery/native return distinct from physical full-wake qualification. Human guide, ADR and changelog fragment in same slice.
5. Run focused negative/race tests under actual normal Ma'at admission, then independent SSA source review and normal release gates. Only after accepted real authority bindings and release qualification negotiate a fresh live hardware window and independent observer.

Scope: sirsi-pantheon only, isolatedcheckout /private/tmp/sha-desktop-custody-20261020, branchcodex/desktop-custody-production-20261020. SHA implementation/native qualification; SSA independent review/integration/release/cross-cloud proof; Ra custody/authority source coordination. Existing shared source, installed hooks, security controls and retained artifacts untouched.

Commercialization Gate:
- User: Mac/fleet operator preserving remote access and active owned workloads.
- Pain: repeated manual sleep/custody handoffs, unsafe raw commands, lost progress and unverifiable return.
- Workflow: inspect readiness, authenticate finite sleep intent and quiet custody, journal once, schedule/read owned wake, dispatch once, independently prove full return/continuity.
- Willingness-to-pay path: governed fleet operations and reduced support/downtime; internal foundation qualification, not a new standalone customer product claim.
- Trust boundary: existing enrolled authority only; OS host/boot identity, protected canonical workload state, one-use durable authorization, exact native adapter. No automatic issuer enrollment or permission relaxation.
- Operational owners: SHA native evidence; Pantheon engine; SSA release; Ra existing authority.
- Done evidence: exact-object source reviews, denial/replay/concurrency/unknown-recovery tests, actual pipeline proof, validated completion proof, followed separately by fresh native Sleep/fullWake/continuity and scheduled cloud execution within interval.
- Classification: platform-foundation. No literal shutdown, native sleep or fleet independence claimed from source tests.

Initial ETA: first source slice and focused qualification packet within2hours; next check at new authority-binding discovery or first tested source commit. Zero additional cloud workers/meters commissioned; independent review remains SSA. No code or review budget expansion.

## Source implementation checkpoint

The canonical online path replaces the preliminary signature prototype. It adds
one typed participant RPC; concrete management-only creation/revocation; protected
schema-25 state with durable one-shot records; authenticated exact participant
binding; shared-transaction Ma’at reservation/consumption fence; native identity,
process/writer inventory, assertion and full-wake parsers; independent M1
observation; production controller CLI; and return-only unknown reconciliation.
Actual deployment/privilege/participant configuration and physical qualification
remain separate. Existing keys, guard, OS settings and schema version are unchanged.

Validation includes authenticated role spoofing, generic-state bypass, replay,
revision reapplication, stale/expired/revoked/boot/inventory mismatch, concurrent
reserve-versus-consume, future workload, native DarkWake/missing-record/mismatched
observer, explicit unknown return and no-invocation denial. The first reconcile
regression exposed legacy journal terminal ordering; preserve its failing receipt
and the corrected pass. Full source gates and independent SSA review are pending.

Three-home publication is pending source review; no Reading Room/Workspace sync
is asserted. No additional cloud worker or new metered job was commissioned.
