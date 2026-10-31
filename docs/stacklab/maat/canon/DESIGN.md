# Design — Ma'at System One

The canonical flow is `observe -> normalize/hash -> prepare retained evidence -> create/commit immutable evidence -> exact-scope preflight -> assess -> append decision -> Casebook -> CLI/Horus API`. `internal/maat` owns assessment semantics and local failure-memory integrity; Casebook derives a read-only view. Every governed leaf has a hash-bound prepared/committed write record; unresolved preparation is recovery debt and fails closed. Recovery instructions remain evidence-linked presentation data, never executable input. Unknown status, unavailable evidence or journal input, and unsupported writes return errors rather than synthetic success.

The cross-process decision-journal mutation lock is created relative to its
retained parent descriptor with `O_CREAT|O_EXCL`; only `EEXIST` permits one
no-create open of that exact leaf. Errors from either operation, including
`ENOENT`, propagate without retry; there is no pathname fallback. This avoids
the Darwin concurrent `openat(O_CREAT)` without `O_EXCL` first-create failure
documented in the
[primary Entire CLI fix](https://github.com/entireio/cli/commit/9c18f31b23a62749b804718125b17c236420a28a).
Before flock, the opened lock must be an owned, private, singly-linked regular
file; invalid leaves fail closed without chmod or repair.
The targeted qualification reproduction receipt records 98 failures and 2
passes across 100 isolated race runs before this correction:
`~/.sirsi/evidence/pantheon-freeze-zero-ledger-20261030/qualification-lock-94a87488-isolated-reproduction.jsonl`.

Failure-memory preflight dispatches the closed pure `maat.active-incident/v1`
guard only for active exact matches. It rejects recurrence from verified retained
evidence, not from a new host-health probe. Unsupported matching guards are
unverifiable; terminal/out-of-scope incidents never become executed checks.
Evaluated guard identity binds embedded implementation and dispatch source bytes
(`memoryguard.go`, NUL, `memory.go`), not an imported label or binary attestation.
Nonblocking retained-FD reads refuse FIFOs before waiting and retain the existing
regular-file, link-count and size checks. No dynamic command or mutation authority
is added; the eight operational integrations and installed proof remain open.

One source candidate wires Ma'at's managed liveness-watch Casebook action
through explicit default-store initialization, exact-scope preflight, durable
attempt, and exact post-observation. That action shares the rootless local
engine/HOME install and inspection boundary; it does not require a developer
checkout or call the legacy router-backed runner. Native Casebook owns the
confirmed first-use and inspect/re-observe controls. A separate unqualified
candidate wires the existing macOS whole-app `--app`/`--cli` route through an
exact signed/notarized PKG, retained preflight and attempt, native Installer
handoff, and installed readback. Installer handoff is not success and unresolved
outcomes are never replayed. Both candidates remain unqualified; the other six
operational rows, remaining projections, and installed proof remain open.

The O2/O3 corrective donor closes six source findings on its exact 7c8 parent:
both update owners fence each other's pending attempts; installed readback binds
the staged app CodeDirectory hash and both helper payload hashes; known adapter
refusal before native handoff has a retained `not-invoked` outcome; target-present
app-update WAL leaves recover only their exact bytes; package identity is hashed
again after trust inspection; writer validation matches the retained attempt and
uses the existing candidate/live schema ceiling check. This donor is not the final
composition: the older preflight caller must be composed onto the published 137
baseline and use G02's stored opaque receipt/readback API. No G02 API is widened,
and raw self-update, alternate installer routes, original O4 rollback, terminal
projection/native debt controls, and qualification remain open.
