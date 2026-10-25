# O7 diagnostic request correlation

Source continuation only. This composes the existing initialized current-user
Ma'at store, scoped preflight, record intent and journal. It adds no router DB,
service, identity, generic executor, exporter or mandatory legacy-client flag.
The successful System One verdict/Decision wire remains v1. Existing triage
without a request ID remains supported; it cannot acquire invented correlation.

## Closed CLI and native handoff

All operations below are `sirsi maat triage`, with `--json`. UUID means a new
lowercase canonical UUID generated and retained by the native model BEFORE its
first preparation call. A request key is not admission or consent.

| Flags | Effect |
|---|---|
| `--prepare-record-request UUID` | Create/verify only the durable metadata ticket, no diagnostic or append. |
| `--record-request UUID --confirm` | Consume the prepared key once, before diagnosis. Ordinary full preflight/receipt and exact record readback remain required. |
| `--read-record-request UUID` | Exact read-only lookup, including completed outcomes. No observation, append or metadata recovery. |
| `--resolve-record-request UUID --confirm` | Durably fence that exact key and reconcile its journal/result data under the existing memory lock. Never observe, append, or reopen the key. |
| `--inspect-record-requests [--request-limit 1..128] [--request-cursor TOKEN]` | One bounded read-only discovery page of pending/completed requests and historical record evidence after restart. Default page size50. |

Select one request operation. Metadata inspection/preparation refuses record
confirmation; record/resolve require it. Request operations cannot be combined
with legacy pending-record inspection/reconciliation. Invalid input is refused
before diagnosis. All existing legacy operations and their consent remain.
Explicit empty `--prepare-record-request=`, `--record-request=`,
`--read-record-request=` and `--resolve-record-request=` are errors even with
`--confirm`: flag presence is not interpreted as omission or legacy mode.
Omitting all request selectors retains the ordinary legacy command. Page flags
are valid only with discovery; an empty discovery cursor starts a fresh query.

Preparation, exact lookup and explicit resolution return `DiagnosticRequestRead`:

* `request`: `{schema:"sirsi.maat.diagnostic-request.v1", id, uid, store_uuid}`.
  The actor/store are derived locally, never supplied by flags.
* `state`: exactly `prepared`, `interrupted`, `pending`, `recorded_verified`,
  `not_submitted`, `not_recorded` or `fenced_not_recorded`.
* `intent` (optional): existing complete `DiagnosticRecordIntent`, including
  manifest and exact retained preflight reference. Its `id` is the record ID,
  distinct from the request UUID.
* `result` (optional): existing `DiagnosticRecordResult`, with its exact record
  `id`, `intent_sha256`, `decision_evidence`, `verdict_sha256` and `state`.

`prepared` means the key has not been consumed. `interrupted` means a consumed
invocation released its writer lock without a verified record result. Neither
permits resubmission; explicit resolve fences it. `pending` identifies exact
retained record evidence needing data-only reconciliation. Missing/malformed
custody is an error, never synthesized as a safe empty lookup.

`recorded_verified` requires the existing strict journal/verdict/result checks.
`not_submitted` retains its original meaning: journal was not called.
`not_recorded` is deliberately distinct: after a durable request fence and
exclusive writer serialization, complete journal lookup found no matching row.
It does NOT claim that no call was attempted. Its result contains no invented
Decision evidence/verdict digest and requires an exact retained request fence
whenever read. `fenced_not_recorded` is a fenced request with no committed
record intent, also not a statement that the process never started.

Discovery returns `{schema:"sirsi.maat.diagnostic-request.v1", requests:[],
records:[], history:{...}}`. `records` contains `{intent,state,result?}` for historical
records, including those created before request correlation. Do not infer a
request-to-history association from time, list order, source similarity or an
empty pending list. No old completed record is relabeled as a new request.

### Discovery continuation and truthful completion

`history` contains `schema:"sirsi.maat.diagnostic-discovery.v1"`, `query_id`,
`snapshot`, `coverage:"partial"|"complete"`, optional `next_cursor`,
`after_cursor`, `returned`, and `count_scope:"page"`. Counts describe this page,
never a whole-query total. The limit applies to BOTH collections jointly; a
512KiB compact row-data budget may make the returned page smaller than the row
limit. There is no128-item limit on total history. Records sort by
`record:<record-sha256>`, then requests by `request:<UUID>`; collection arrays
retain that deterministic key order. Both arrays are explicitly present even
when empty. Empty `requests` on an early page is not an empty whole history.

Follow `next_cursor` with the SAME `--request-limit` until a complete page has
no continuation. Keep one loader generation, query ID and snapshot throughout;
reject duplicate rows or differing query/snapshot metadata. A complete page
after a cursor means that remaining range is exhausted, not that this page
contains the whole query. Only a loader that began at an empty cursor and
successfully followed the complete chain can mark its accumulated discovery
complete. Retain completed-operation receipts independently of page loading.

The read-only cursor is canonical bounded JSON/base64 bound to query version,
local actor/store/root identity, page limit, snapshot and exact last key. It is
not action authority. Every page retains the existing strict full diagnostic
namespace and WAL validation. Snapshot identity reuses the existing Casebook
inode/ctime helper (excluding read-atime), covering root and all attempt,
evidence and WAL object metadata before/after projection. Earlier-page in-place
rewrites, new results/requests and root replacement invalidate continuation.
This bounds returned rows/bytes; it does not claim a bounded total-history I/O
pass or hostile same-UID atomicity. No store/journal/guard validation is skipped
to meet a page budget.

On snapshot/query drift, CLI exits nonzero with
`{schema:"sirsi.maat.diagnostic-request.v1",error_code:"snapshot_changed",
restart_required:true}` and NO successful page. Stop that chain, retain its
partial status/history honestly, then explicitly restart with
`--request-cursor=` and a new loader generation. Never splice old partial rows
into the new snapshot or clear a current unknown request from unrelated rows.
Malformed cursors/custody are refusal, not a complete empty list. Known exact
UUID lookup remains independently available during discovery; its identity and
disposition must still match the selected operation.

## Required native sequence (Nash-owned adapter)

Retain the request UUID in the engine-owned operation before preparation, and
preserve it on cancel/navigation/lost output. Verify returned schema, UUID,
current-user/store binding and exact intent/result relation. Only a verified
`prepared` response permits the separately confirmed record submission. Do not
resubmit a consumed key, even if its first reply is lost.

After any unknown record/preparation reply, offer exact read and confirmed
resolve for the retained UUID. If preparation never reached its first leaf,
resolve creates AND fences that same key under the writer lock; a delayed
preparation cannot reopen it. Once a terminal disposition is verified, preserve
that completed receipt independently of readiness refresh and use a NEW UUID,
fresh observation and new consent for subsequent work. No health promotion is
inferred from record/reconciliation success. Refresh Casebook only for a proven
recorded commit, through the query-owned refresh route.

Discovery supplies truthful completed history after model/app restart. Prior
uncorrelated unknown invocations remain explicitly uncorrelated. They may be
inspected and their exact existing record IDs reconciled; the adapter must not
fabricate which invocation produced them or silently clear an old uncertainty.
Unknown/corrupt/missing byte custody remains inspectable debt, not permission to
reset, delete, reconstruct data or replay a record.

The existing dedicated native resolver's fresh-screen recording entry also
needs the same shared correlation adapter at composition; F1 host matching in
that consumer is corrected here, but its legacy command arguments are preserved
until Nash's adapter is integrated. This source donor is not a whole-O7 claim.

## Serialization and crash boundaries

Ticket/start/binding/fence are closed create-only objects in the existing
`attempts` namespace and governed by its existing private no-follow WAL writer.
The typed request session retains an independent open-file description of the
SAME already-bound store, holding its existing `.lock` from before diagnosis
through synchronous journal readback. This avoids another method on the caller
handle converting/releasing the session's flock. No new lock file/store exists.
Session close is idempotent; duplicate begin refuses before diagnosis.

The request-to-record mapping commits before the record intent/effect. Explicit
resolve acquires that writer lock, so an active invocation completes/terminates
first. Its fence commits before any absent-row disposition, denying late starts
and old-key repeats. The existing exact append-once and intent/result checks
also deny replay of the same captured observation through the legacy API.
Matching complete result WAL can be finished data-only; unrelated or malformed
WAL is refused. Missing bytes are not recovered from hashes or reconstructed.
In particular, complete retained `not_submitted` target bytes take the existing
exact result-reconciliation fast path before any empty-journal inference or
new fence requirement. Its exact WAL commit completes, then the request fence
is persisted normally. No journal lookup/append or diagnostic is needed for
that already-proven pre-append refusal. Recorded results still require strict
journal verification; corrupt, unrelated or missing targets do not acquire
this fast path.

## Producer/consumer privacy compatibility

New retained diagnostic observations use
`sirsi.maat.local-diagnostic-observation.v2`. Their v1 System One subject has
`host_binding:"sirsi.maat.local-host-sha256.v1"` and
`repo:"local-host-" + SHA256(trimmed locally read hostname)`. Go and native
consumers recognize ONLY that closed representation and compare against an
independent current local read. Historical subjects without `host_binding`
retain exact legacy label matching. The existing security/delivery/identity
`boundary` field and escalation policy are unchanged.

Unknown safe codes retain SHA256(trimmed original check); finding ID/evidence
take the original first six digest bytes, rather than hashing the redacted code
again. No raw unknown name/prose/path is persisted to restore identity. Changed
checks/hosts, malformed bindings and dedicated-custody bypasses still refuse.
Historical observation-v1 projection bytes reconstruct exactly as originally
stored; they are not silently upgraded, rehashed or promoted to current custody.

## Qualification and composition

No tests/build/model/service/journal action has been executed by this source
author for this donor. Main qualifies normal/race, static checks and native
consumer interop. To exercise ACTUAL Go-to-native bytes, run
`TestMaatTriagePrivateRecordedHostReachesExistingBinaryDriftConsumer` with a new
private create-only `PANTHEON_TEST_DIAGNOSTIC_WIRE_OUTPUT` path, then provide
that exact artifact to
`MaatDiagnosticResolutionTests/testActualGoRecordedPrivateProjectionRoundTripsNativeCustodyAndFreshScreen`.
The Swift method explicitly skips without that artifact; it is not a fake-wire
PASS. Use separate output leaves per Go normal/race invocation.

This donor's catalogue parent predates G03/G04's current 31-component catalogue.
Its recipe edit changes ONLY existing `maat-host-health-screen` metadata, not IDs
or count. Sagan's three-way composition must retain all newer G03/G04 metadata,
G01 native DI and independently authored UI1/UI2/correlation adapter. No old
29-component whole-file replacement or tautological expected-ID test is allowed.
