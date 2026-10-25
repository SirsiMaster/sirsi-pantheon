# O7 local diagnostic record boundary

Source continuation, 2026-10-25. This covers `sirsi maat triage --confirm`,
not all Diagnostics acceptance, installed operation or authorized sharing.

## Original obligation and delivered scope

The original failure-memory contract's Diagnostics row requires provenance and
freshness verification and redaction before authorized export. Its O7 map names
the existing triage/`Screen`/`RecordSystemOne` entry. The adopted scope here is
that existing confirmed local record; no diagnostic-specific authorized export
owner was found at9350. Generic JSON output/archive helpers do not establish
export authority. No remote exporter, account, vendor or alternate store is
added. Approved derived export and other diagnostic entrypoints remain open.

## Observation and privacy

Unconfirmed triage remains available without initialized/healthy failure memory.
It reports observation provenance, memory initialization state and
`memory_preflight:not_performed` separately; a healthy status is never a memory
admission. Missing, empty, expired or future provenance cannot admit a record.

The raw local doctor report is hashed in memory only. The bounded retained
projection contains an allowlist of check codes, severities, closed repair IDs,
score, original observation time and source/host-label digests. Unknown check
names become digests. Producer messages, detail, fix strings, durations, prompts,
process arguments and diagnostic paths never enter the new manifest, verdict,
receipt or journal payload. Host-label digests are local provenance, not a new
machine-identity authority. The existing journal retains its normal local
Time/Host envelope. No hash is permission to publish private evidence.

## One record and data-only recovery

Only the initialized current-user default Ma'at memory is used. A closed
`DiagnosticRecordManifest` binds actor, store UUID and complete selected status
projection. `PreflightDiagnosticRecord` calls the existing full-manifest bound
preflight for exact scope `pantheon/local-macos/record-system-one-diagnostic`;
scope-only issuance is insufficient. Its complete opaque retained receipt must
match before a record can begin. Registry and selected source provenance are
revalidated, not imported from a caller's claimed pass.

Lock order is memory's existing exclusive lock, then the existing journal's
mutation/read lock. The command's per-call verifier only re-hashes its captured
report and current local label; it must not acquire store/journal locks or
execute a payload. No generic action callback or recovery command is accepted.

The same memory `attempts` namespace commits a deterministic diagnostic intent
before the existing `RecordSystemOne` call. The latter uses the journal's already
implemented `AppendOnce`; its existing `OriginEvidence` links intent and full
preflight receipt, without changing Decision validation or introducing another
authority column. Complete strict `FindByEvidence` readback must match every
stable decision/verdict/link field before `recorded_verified` is persisted.
Journal-owned Time/Host are not rewritten. Receipt/result persistence uses the
existing create-only WAL/private no-follow opened-FD rules.

The shared liveness reader recognizes only the exact diagnostic suffixes and
validates their closed schema/store/receipt/result bindings. It does not skip
unknown objects or weaken liveness admission. Other operation authors must
compose their own closed namespace registrations; this adds no global gate.

Known failure after intent but before any journal call durably records
`not_submitted`. The same observation is not replayed; a fresh explicitly
confirmed observation can proceed. If that result cannot be persisted, WAL debt
remains honest. Any uncertain append/acknowledgement or wrong/missing journal
readback remains unresolved. A new record is refused while such diagnostic
delivery debt exists; pure observation stays available.

`maat triage --inspect-records --json` exposes verified pending IDs without
observing the host or appending. `maat triage --reconcile-record ID --confirm
--json` reads the exact existing journal row and completes only its deterministic
memory result. It never appends, fabricates a missing row, re-observes, initializes
memory or executes guidance. It can retry only its own exact result WAL tuple;
unrelated prepared writes remain refusal. Missing/corrupt proof stays unknown.

Confirmed refusal has an explicit non-recorded envelope containing the safe
observation, not a top-level recorded-verdict response. Existing native record
decoders therefore cannot mislabel an unknown/refused record as recorded.

## Upgrade and qualification

Preserve v1 diagnostic schema/field validation, source freshness limits and
private custody. Compose the two exact shared-attempt reader cases with sibling
operation registrations; do not bypass their guards or genericize this adapter.
Never replace strict lookup with a Recent window, append exit or metadata-only
assumption. Never reconcile by resending. New privacy fields require explicit
allowlist/fixture review; raw report text is not a fallback.

Authored actual-command and file-store fixtures cover missing memory with
observation preserved, privacy canaries across output and owned records,
provenance/registry drift, scoped/unsupported guards, intent-before-append,
concurrent exact observation, lost acknowledgement, unknown/mismatched readback,
known non-submission, WAL result-write failure and data-only recovery. These are
source fixtures, not execution evidence. Main owns normal/race/vet/lint/build and
independent review. No model, service, installed journal or authorized export
operation was run by the author. Full O7/other seven rows and original historical
receipt/installed obligations are not retired by this child.
