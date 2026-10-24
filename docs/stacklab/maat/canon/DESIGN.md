# Design — Ma'at System One

The canonical flow is `observe -> normalize/hash -> prepare retained evidence -> create/commit immutable evidence -> exact-scope preflight -> assess -> append decision -> Casebook -> CLI/Horus API`. `internal/maat` owns assessment semantics and local failure-memory integrity; Casebook derives a read-only view. Every governed leaf has a hash-bound prepared/committed write record; unresolved preparation is recovery debt and fails closed. Recovery instructions remain evidence-linked presentation data, never executable input. Unknown status, unavailable evidence or journal input, and unsupported writes return errors rather than synthetic success.

Failure-memory preflight dispatches the closed pure `maat.active-incident/v1`
guard only for active exact matches. It rejects recurrence from verified retained
evidence, not from a new host-health probe. Unsupported matching guards are
unverifiable; terminal/out-of-scope incidents never become executed checks.
Evaluated guard identity binds embedded implementation and dispatch source bytes
(`memoryguard.go`, NUL, `memory.go`), not an imported label or binary attestation.
Nonblocking retained-FD reads refuse FIFOs before waiting and retain the existing
regular-file, link-count and size checks. No dynamic command or mutation authority
is added; the eight operational integrations and installed proof remain open.

The current source candidate wires Ma'at's managed liveness-watch Casebook
action through explicit default-store initialization, exact-scope preflight,
durable attempt, and exact post-observation. That action shares the rootless
local engine/HOME install and inspection boundary; it does not require a
developer checkout or call the legacy router-backed runner. Native Casebook
owns the confirmed first-use and inspect/re-observe controls. Qualification and
the other seven operational rows remain open.
