# O7 local diagnostic record boundary

Current bounded donor qualification, 2026-10-25. This covers confirmed local
triage and the exact request prepare/record/read/resolve workflow. Authorized
export, installed adoption and whole-O7 acceptance remain open. The new
post137 integrated head requires its own checks; donor results are not a rerun
of that head.

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

## Native first use and retained delivery

The engine-owned `MaatDiagnosticRecordModel` lives with case operations, outside
Casebook loading/error/empty branches. The ordinary workspace root exposes its
local observation and record-recovery controls without constructing a case.
The System One entry uses the same model and callbacks, not a second writer.
Unconfirmed observation invokes only `maat triage --json`. Confirmed recording
uses stdout plus actual exit provenance with the existing thirty-second bound;
stderr is never concatenated into a purported JSON record.

Readiness inspection uses existing local memory inspection and complete
query/snapshot-bound request/history discovery, with explicit continuation and
restart rather than an empty-first-page or total128-row assumption. Genuine uninitialized custody offers the
existing typed first-use initialization, with separate confirmation, a repeated
pre-effect inspection and store-UUID readback. No import, adoption, reset,
service action or automatic record follows initialization. Corrupt/unverifiable
memory keeps observation available and the adjacent prepared-receipt recovery
entry accessible. It cannot be replaced or silently initialized.

Selected diagnostic debt retains its complete typed intent and store binding.
The confirmation is generation-bound, consumed before suspension, and rechecks
the exact selected bytes before the existing data-only reconciliation command.
Only that captured ID is submitted; changed/foreign custody is refused. A
verified result requires matching ID, schema and result digests. Neither empty
inspection nor reconciliation of an unrelated ID clears an unknown delivery.
The successful v1 Decision/verdict wire and its recorded summary are unchanged;
refusal envelopes retain the safe observation without acquiring that summary.

The lost-reply-before-returned-ID limitation is closed for new correlated
invocations. The engine-owned adapter selects and retains the request UUID
before preparation. It verifies preparation before issuing exactly one
`maat triage --record-request UUID --confirm --json`, then reads the same
request independently. Preparation/read use `--prepare-record-request` and
`--read-record-request`; explicit data-only fencing/resolution uses
`--resolve-record-request UUID --confirm`. These literal command names are
checked against actual Go-accepted argv, not generated by repeating `record`.

A lost preparation or record reply preserves the key and unknown state. Exact
read or separately confirmed resolution can settle that request without a
second diagnostic, journal append or resubmission. Completed outcomes remain
readable and retain typed evidence across inspection, remount and navigation.
Missing or contradictory custody remains unknown. Older uncorrelated invocations
stay historical; no request mapping or permission to retry is invented from
absence. See [request protocol](DIAGNOSTIC_REQUEST_V1.md) and
[native boundary](DIAGNOSTIC_NATIVE_REQUEST_ADAPTER_V1.md).

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
known non-submission, WAL result-write failure and data-only recovery. Their actual bounded execution is recorded below; author source preparation
itself did not run them. No model, service, installed journal or authorized export
operation is inferred. Other seven operation rows and full-O7/installed/export
obligations are not retired by these fixtures.

## Native result and refresh retention

Readiness inspection and the last verified completed operation are separate.
One bounded typed historical result retains initialization's exact store/ready
readback/no-import fact, or a recorded/reconciled ID and existing evidence/digest
fields. Automatic inspection, a failed later operation, and navigation do not
replace that result with a generic readiness label. Its historical heading does
not label a newer refusal or observation as recorded, healthy or repaired.
The canonical request API supplies correlation; reconciliation never synthesizes
an observation. The earlier uncorrelated source-stage lost-ID limitation was
superseded by qualified Go57/native458 request custody and the normal-window
observations below. It remains a historical limitation for old uncorrelated
records, not a reason to resend them.

The ordinary workspace owns its existing System One/Decisions history stores
and injects them into the unchanged browse views. Both diagnostic entry points
refresh those stores after verified completion using their selected queries and
a new snapshot, without replaying an old cursor or remounting the workspace.
An in-flight selected query takes precedence over an older committed filter;
existing generation/cancellation fences retain the latest page on late failure,
success or cancellation. Standalone browse views retain their default ownership.
The only history read seam is per-instance read-only runJSONRead with its existing
bounded canonical argv. No generic executor/store/mutation override is added.

Seven authored regressions extend MaatDiagnosticRecordTests: actual root
confirmed -> automatic inspect -> query-owned history refresh/remount; exact
recorded_verified/not_submitted reconciliation result retention; initialization
readback retention; late refresh vs new filter; pending selected-query refresh;
canceled root refresh retaining its completed result and prior page; and
oversized completion evidence refusing without clearing unknown delivery.
Those original result/refresh regressions remain in the selected native61
methods. Earlier authored/unrun statements describe their historical source
handoffs; they are not the current qualification disposition.

## Exact bounded evidence (2026-10-25)

Evidence filenames are private retained receipts, not runtime configuration or
publication authority. They are joined and hash-pinned in the post137 source
composition receipt.

- Go57 `57daec25992bd0dd8e02beffc27f2b8129eda217`: Main native
  `0c64b55bcc454aefa8b6a842bc70fedc`, qualified receipt SHA256
  `0d0cbba159a0f26fabd0feda4c4d640ad3ea87802fd1603287477904303d926c`.
  Four fresh rows cover module verification, CLI normal, core race and CLI race;
  core49 normal is explicitly carried from the prior0249 run, not relabelled.
- `maat-O7-458107f7-Main-native61-six-controls-qualified-20261025.json`,
  SHA256 `dc014b2bd9396068f39ec510b9c87b42894c8b26d4d993432cfa4fbb75264d46`:
  Main native `6f219f90a376485b9248e30a37cf0d13`, all12 rows pass, native61,
  two race-artifact interop methods, and six compiled production-only controls.
- `maat-O7-458107f7-Main-four-owned-window-observations-20261025.json`,
  SHA256 `fb24f16b8719bfd54d4072c9208af494c416cc67a19c2b13691cb5162f820e54`:
  Main physical normal-window observations cover review Cancel with zero new
  calls, one canonical confirmed record with UUID retained first, lost preparation
  read/explicit fencing with zero record calls, lost-record exact completion
  without replay, and400-partial/drift-restart/ninth-page449-history plus exact
  request completion. Remount and actual push/Back preserve typed completion.
  These are owned inert windows with captured Go JSON and labelled synthetic
  history, not installed journals, live diagnostics or a whole-product verdict.

The integrated successor also joins O7's closed attempt types to the app-update
scanner through the existing typed owner validators. New cross-owner regressions
are source-only until Main qualifies that integrated head. Unknown objects,
corrupt custody and unrelated prepared WAL still refuse; this is not an action
grant or a new global policy framework. Authorized export, installed operation,
other diagnostic entrypoints and whole-O7 acceptance remain explicitly open.
