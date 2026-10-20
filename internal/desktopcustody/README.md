# Typed sleep custody source contract

This internal package contains the typed transaction and fixed native adapter.
It does not yet install an operator command, enroll an issuer, or grant permission
to sleep a host. Missing
trusted verifier, custodian, power adapter, or journal denies dispatch.

Dispatch verifies a finite sleep-only admission, obtains quiet receipts, persists
the prepared request, schedules and reads back an owned wake, and asks the trusted
custodian to revalidate current authority immediately before native invocation.
The custodian must invoke its callback synchronously and must not retain it. The
service permits one native invocation; late or duplicate callbacks are rejected.
Production adapters remain responsible for authenticated review, durable nonce
consumption across different request IDs/journals, host reservation, workload and
inhibitor checks, revocation, independent observer enrollment, and native evidence.
The request-ID journal alone does not provide those guarantees.

Every journal event retains the original nonce, admission window and custody
generation. Return verification reconstructs that exact admission and rejects
mixed bindings. Expiry prevents new dispatch; it does not replace the historical
capability during independently authenticated recovery.

The journal separates native-invoked and native-returned receipts from physical
sleep/wake proof. A native-returned receipt records an adapter error verbatim when
present; the interface currently supplies an error, not a numeric process exit
code. An invocation error or post-invocation custody error leaves the outcome
unknown and retains the scheduled recovery wake. Denial before invocation cancels
the owned wake. Prepared, invoked or unknown transactions are never automatically
retried. Failed receipt persistence cannot authorize another native invocation.

Only independent full-wake evidence allows verified-return and owned-wake cleanup.
A successful native return, reachability or DarkWake cannot establish that result.
The native adapter is source-tested with fake command execution. No production
issuer/custodian enrollment or live native qualification is supplied here.

Verify without touching host power:

    go test -race ./internal/desktopcustody -count=1
    go vet ./internal/desktopcustody

All power seams in these tests are fakes. The hardware review contract is retained
at ~/.sirsi/hardware-admin/20261013/sleep-contract-210900/result.md; the source
review is router item 20261015-172011 from sirsi-hardware-admin.

## Production controller implementation (2026-10-20, in progress)

The accepted9f8 transaction is selectively restored on current main. The first
implementation slice adds persistent nonce/host fences, exact-purpose signature
verification and a fixed Darwin native adapter. There is no issuer generator,
root enrollment, credential fallback or shipped test custodian. No deployed
sleep-purpose authority is currently demonstrated; SleepIssuerLookup must be
bound to the existing trusted authority's explicit sleep policy and revocation.
A recovery public key must not be promoted into that role.

The signed permit's payload is the exact json.Marshal(SleepPermitClaims) byte
encoding, signed over `pantheon.sleep-custody.v1` plus one NUL byte plus payload.
Unknown, expired, revoked, wrong-purpose/host/boot/request/wake/executable/review
bindings deny. Signature verification does not prove live workload quietness;
the independently trusted custodian must revalidate that immediately at dispatch.

State claims are local, private, no-follow and durable. Consumed nonce records
are retained. Host fences persist across process exit and unresolved outcomes.
Confirmed pre-invocation failure or verified full return can release only the
owned host fence; no background claim collector or automatic retry is added.
Native scheduling mutation uncertainty is recorded separately from known denial,
with the original planned wake retained for reconciliation.

The native adapter changes no power settings and has no executable/shell/sudo
fallback. It uses one owned wake tag and exact schedule/cancel operands. Every
native adapter test injects fake command execution; no actual power qualification
or new execution permission follows from tests.

Remaining first-class integration: operator controller/CLI, real enrolled issuer,
live workload custodian and independent return observer bindings, recovery of
ambiguous outcomes, current-main review/normal release and separately authorized
native qualification. See ADR-119 and the implementation plan; do not describe
this intermediate packet as a complete production feature.
