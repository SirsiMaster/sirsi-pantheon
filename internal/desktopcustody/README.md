# Typed sleep custody source contract

This internal package is source scaffolding. It does not install a power command,
provide a CLI flag, enroll an issuer, or grant permission to sleep a host. Missing
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
No production issuer, custodian, adapter or native qualification is supplied here.

Verify without touching host power:

    go test -race ./internal/desktopcustody -count=1
    go vet ./internal/desktopcustody

All power seams in these tests are fakes. The hardware review contract is retained
at ~/.sirsi/hardware-admin/20261013/sleep-contract-210900/result.md; the source
review is router item 20261015-172011 from sirsi-hardware-admin.
