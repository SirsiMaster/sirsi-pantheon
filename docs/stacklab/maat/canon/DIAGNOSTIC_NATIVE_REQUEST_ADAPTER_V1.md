# Native O7 request adapter — source candidate

This consumer child starts at independently qualified UI1/UI2 source1b05. It
requires accepted Huygens73d53097 (Go API and privacy host/check correction) at
composition; it does not replace or implement that Go authority. No source
qualification or installed/full-O7 closure follows from this document.

`MaatDiagnosticWire` projects canonical request identity, immutable intent and
preflight reference, terminal result, and discovery metadata. Native verifies
UID/store/request UUID, record/result relation, schemas, closed states, digest
forms and exact receipt scope; it does not manufacture record IDs, hashes,
missing source bodies or journal evidence. The constructor retains its existing reader seam and a typed UUID factory
(default `UUID()`); Main wire fixtures bind the latter to their actual Go
request UUID, without a CLI/environment override or an alternate executor.
The native UUID is only the existing
protocol's invocation key. It is generated and retained in the engine-owned
model before the preparation call. Neither a key nor a UI approval bypasses the
canonical Go store/preflight/journal authority.

The sequence is inspect current memory → complete discovery → review/confirm →
recheck unchanged memory and full-namespace snapshot → retain UUID → prepare →
verify prepared exact tuple → record once → exact read. A lost preparation or
record response, timeout or cancellation retains the key. Only a new user read
or confirmed data-only resolution can settle it. A prepared/interrupted/pending
request blocks new records. Explicit resolution uses the selected exact
UID/store/request and known state/intent/result tuple; changed known custody
requires new review. Missing unknown preparation can be durably fenced by the
canonical resolve API. No resend, second diagnosis or append is issued by read
or resolve. A completed disposition needs fresh inspection and a new UUID and
confirmation for subsequent work. Legacy uncorrelated records remain separate;
the adapter never associates them by time, order or similarity.

Discovery has page limit50, a512KiB canonical row budget and eight page reads per
UI turn. Continue retains query ID/snapshot/page size and follows server cursors;
duplicate rows/cursors, malformed arrays, changed snapshots or cancellation do
not produce complete empty history. More than128 total rows are supported.
Only a chain begun at the empty cursor and exhausted normally is complete.
Before recording, one fresh canonical first page must have that same full-store
snapshot/query identity (the producer validates the whole namespace/WAL for each
page). This avoids restarting a large complete chain at page one forever while
retaining actual Go preflight at preparation/record time. This is not a claim of
bounded total-history producer I/O or hostile same-UID atomicity.

Typed last-completion receipts live independently of readiness/discovery and
retain request/actor/store/state, exact record/intent/Decision/verdict IDs. Empty,
partial, failed or stale reads cannot clear an unknown request or erase a
completion. Unrelated exact history reads do not overwrite the last operation
receipt. Root query-owned history refresh occurs only after a proven recorded
commit, including exact resolution/readback; no remount reset or healthy-gate
promotion. The dedicated resolver fresh-screen action uses this same
engine-owned adapter, and exposes the same recovery controls after lost output.

## Replaceable boundary and composition

- Consumer: `MaatDiagnosticCorrelation.swift`, `MaatDiagnosticRecordModel.swift`,
  `MaatDiagnosticRecordView.swift`, `MaatDiagnosticResolution.swift`.
- Existing ownership/root: `MaatOperationState.swift`,
  `MaatWorkspaceRecoveryFrame.swift`, query-owned stores in
  `MaatCasebookView.swift`. Their ownership is retained; the latter receives only
  the accepted optional host-binding DTO field in this child.
- Producer: accepted73d `maattriage_request.go`,
  `memory_diagnostic_request.go`, `memory_diagnostic_discovery.go`; compose those
  existing operations rather than invent a native store or another executor.
- Fixtures: `MaatDiagnosticRecordTests`, `MaatDiagnosticCorrelationTests`,
  `MaatDiagnosticResolutionTests`, shared inert
  `MaatDiagnosticCorrelationFixture`. The old named record/ownership/refresh
  cases remain, with request-wire assertions replacing legacy submit arguments.
- External Main packet supplies a Go test-only overlay that exports actual
  preparation, result/read, resolve and discovery JSON under private fixtures.
  Native interop explicitly skips without its opt-in file. A skip is not proof.
- Apply recipe delta only to existing `maat-host-health-screen` component and
  union its source/test/upgrade fields. The parent recipe predates the current
  31-component canonical recipe; never install the older whole recipe or change
  its component membership/count expectations.

Main owns strict Release build/native tests, Go-to-native actual wire checks,
compiled semantic controls, and inert normal-window end-to-end acceptance.
No author tests, builds, services, signing, models, user-store operations or
installed-app actions were performed. Existing full-scope obligations remain.
