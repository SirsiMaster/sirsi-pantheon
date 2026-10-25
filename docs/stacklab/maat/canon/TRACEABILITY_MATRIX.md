# Traceability matrix — Ma'at

G02 prepared-receipt reachability: `memory_prepared_receipts.go` and
`maatmemory_prepared.go` extend the existing memory inspector with a verified
complete historical reader and exact intent/store target before any Casebook
preflight projection. `MaatPreparedPreflightRecovery` is owned by the existing
operation lifetime. The f0e liveness-detail mount alone still required a case;
the successor mounts `MaatWorkspaceRecoveryFrame` directly as the ordinary
`MaatWorkspaceView.body`, outside every content branch. Its always-visible
Inspect control calls the existing inspector before any model state exists.
Authored
`TestPreparedReceipt*`,
`TestMaatPreparedReceiptInspectorAndConfirmedCommandWithoutCasebookRow` and
`MaatPreparedPreflightRecoveryTests` cover first-retention interruption, immutable
inspection, exact consent, changed/incomplete/unrelated debt, cancellation and
fresh-assessment separation without a receipt Casebook row.
`MaatWorkspaceRecoveryEntryTests` additionally requires the actual root body/frame
and production Inspect/confirmation-refresh callbacks across empty/unavailable
history, success, failure, cancellation and remount; omitting the root frame
fails this fixture. No AX oracle/skip or visible-window claim is used. External
normal-window evidence remains separately required. Existing canonical
receipt recovery remains the only writer. SOURCE ONLY pending Main execution;
855's 164 normal/164 race and 67 Swift positive receipt stays baseline-bound.
No installed/full-eight-operation claim follows from this child.

HW10 shares `MaatHistoryRefreshStore` and its atomic state commit between both
native history loaders. Authored controlled-response fixtures cover delayed
success/failure after a newer commit, cancellation before/after a new snapshot,
current failure retaining the latest projection, and query/cursor/snapshot
continuation pairing. Only the current generation may publish projection,
query, cursor, error and loading state. Source review and execution remain
required for this child; no prior compiler or MainD5 receipt is relabelled.

History HW01–09 corrective scope additionally binds
`TestSearchHistoryExactEvidenceCountsHiddenDuplicateIdentity`,
`TestQueryHistoryRetainsBoundedCandidatesAndChargesMaterialization`,
`TestHistoryMaterializationChargesOneAtomicBundleAndDefersRest`,
`TestMaatAcceptResolutionHistoricalCommandConfirmsOnce` and the native
generation/cancellation/retained-continuation fixtures. Qualification and the
production-only truncate-before-filter negative remain Main-owned and unrun;
no release, installed-state or broader Ma'at completion follows from this source.

G04 bounded MCP/Horus/TUI history and canonical archive read surfaces are mapped in
`G04_BOUNDED_SURFACES.md` and recipe component `maat-bounded-history-read-surfaces`.
The 2,012-record private-journal, same-snapshot links, partial/empty distinctions,
archive cursor/privacy and stale/cancel ownership fixtures are authored but unrun.
No G01/G02 learning workflow or full original-scope completion is claimed.

| Requirement | Source | Verification/evidence | State |
|---|---|---|---|
| Assessments preserve facts and explanation | `internal/maat/`, `docs/qa/MAAT_SYSTEM_ONE_CATALOG.md` | focused Ma'at tests and receipts | source-bound; current evidence required |
| Decisions are append-only | Ma'at journal recorder | journal write/readback tests | source-bound |
| Failure memory binds evidence to exact scope | `internal/maat/memory.go` | deterministic signature, retained no-follow evidence, create-only prepared/committed leaves, predecessor-bound terminal events, scope-isolation, tamper and race tests | source-bound; surface projection required |
| Failure memory executes only closed matching guards and cannot block on FIFO open | `internal/maat/memoryguard.go`, `memory.go`; `memoryguard_test.go` | typed-call observation, implementation-byte digest, unsupported/mixed/scope/terminal controls; FIFO/size refusal; reopen and failed-write debt; independent concurrent writers; one-way journal failure | corrective SOURCE ONLY, tests not executed here; eight operation integrations and installed proof OPEN |
| Binary-drift update/recovery consumes failure memory at the authoritative Cobra boundary | `cmd/sirsi/maatrepair_binary_drift.go`, `internal/maat/memory_binary_update.go`, existing `internal/maat/memory_receipts.go`; raw self-update in `cmd/sirsi/selfupdate.go`, `cmd/sirsi/selfupdate_maat.go`, and `internal/maat/memory_raw_self_update.go` | Homebrew route: exact reject/unverifiable zero-effect checks, canonical typed full-receipt projection plus exact Store attempt reference before one injected updater call, strict FileDecisionJournal acceptance, real running schema-ceiling observation and persisted `version.Info`, historical freshness and aged reopen, manifest/registry drift, concurrent preview variants, exact postcheck failure, typed invocation barrier, unknown-launch no-replay, owned pre-invocation reconciliation, refused/mismatched claim cannot discharge an older case-A claim or admit case B, and terminal/projection/readback recovery with zero updater calls. Separate existing raw self-update Cobra route: binding displayed before interactive confirmation, retained receipt and durable attempt/read-back before invocation, inert SafeReplace seam, owner rejection, exact target and schema postcheck, terminal projection debt reconciled without replacement. | source authored; gofmt and diff-only checks by subtask author; no tests/builds executed here; Main qualification remains open; commercial whole-app update/install and O4 rollback remain separate OPEN scope |
| Ma'at liveness-watch remains rootless and does not strand first-use/interrupted operators in CLI output | `internal/maat/memory_root.go`, `memory_liveness.go`, `cmd/sirsi/maatrepair_liveness.go`, `internal/liveness/localinstall_unix.go`, native `MaatCasebookView.swift` | canonical engine/HOME local APIs; explicit confirmed first-use genesis; read-only inspect; confirmed no-replay re-observation; native review/recheck and fixed engine arguments | source candidate only; unqualified until focused Go/native qualification and installed evidence; other seven rows OPEN |
| Casebook is read-only | `internal/maat/casebook/` | projection and API tests | source-bound |
| Retained Ma'at history is bounded, scoped, and recoverable in the native surface | `internal/maat/history_query.go`, `internal/maat/casebook/casebook.go`, `internal/maat/diagnostic_resolution.go`, `cmd/sirsi/maatcasebook.go`, native `MaatCasebookView.swift` and `MaatOperationState.swift`; `contracts/stacklab/maat-system-one-recipe-v1.json` | explicit-empty JSON arrays, physically late match after a budget chunk, canonical owner-review/acceptance without resolving the original multi-finding screen, pre-limit kind/text/status, >50 screen/calibration pagination with query-versus-tail count provenance, final-materialization cancellation, strict old-review lookup beyond `Recent(1000)` with malformed-history refusal, and native zero-exit/partial/failed-refresh/same-snapshot continuation fixtures | corrective source candidate only; Go/native qualification not run; no journal contents ingested; `Recent` behavior is unchanged and browse cursors do not authorize mutation |
| Ma'at owns one private canonical archive with retained, privacy-filtered local knowledge | `internal/maat/knowledge/{archive,archive_search,legacy,refresh}.go`, Darwin-only `archive_vfs_darwin.go`, `archive_vfs_darwin_abi.go`, `archive_vfs_darwin_copy_cgo.go`, and fail-closed `archive_vfs_darwin_nocgo.go`, `internal/seshat/adapter_claude.go`, `cmd/sirsi/maatknowledge.go`, knowledge-only sections in `MaatCasebookView.swift` | authored archive, Darwin VFS identity/file-control/sidecar/retirement and bounded ABI-copy fixtures including generated-global GC/stack-growth coverage, cross-process lock-close fixture, and native coverage/cursor/digest/consent/projection fixtures; focused package/CLI/native qualification remains pending; see [source map](KNOWLEDGE_ARCHIVE_V1.md) | corrective source authored; tests not executed; online readers, richer downstream workflows and deeper conflict review remain OPEN |
| Native diagnostics always resolve or complete | `macapp/Sources/SirsiMenubar/Views.swift`, `SirsiEngine.swift` | `CoreContractsTests.testEveryDiagnosticHasAClosedNativeResolutionRoute`; SwiftUI surface review | source-bound |
| Retained activity never strands a person in raw terminal output | `macapp/Sources/SirsiMenubar/{Views,SirsiEngine}.swift` | `CoreContractsTests.testActivityResolutionAlwaysClosesAnAmbiguousOrFailedOutcome`; native Activity detail review | source-bound |
| Credential findings have a protected recovery route | `internal/maat/credentialpreflight.go`, `cmd/sirsi/maatpreflight.go`, `macapp/Sources/SirsiMenubar/{MaatCasebookView,ReleaseWorkspaceView}.swift` | Go required-kind/observation-failure tests; `CoreContractsTests.testMaatCredentialProjectionKeepsAdditionalIdentityNeutralAndNotarizationUnknown`; native recheck/Stack Lab route | source-bound; commercial proof remains separate |
| Missing evidence fails honestly | catalog contract | negative tests and release receipt | source-bound |
| Scoped lifecycle cannot become fabric-wide restart or ambient elevation | `internal/setup/launchdservice*`, `cmd/sirsi/launchdservice*`, `contracts/stacklab/maat-system-one-recipe-v1.json` | closed-label/native actor/retained artifact/instance/per-service lock tests; recipe boundary assertions; [runbook](../../../runbooks/SCOPED-SERVICE-LIFECYCLE.md) | source candidate; installed application drain/health/rollback and signed deployment OPEN |
| Installed/release behavior | release workflow and host evidence | exact release/host receipts | OPEN |
| A release can bootstrap without future helper/old CLI flag deadlock | `maat-native-release-bootstrap`, `.github/workflows/release.yml`, `test_release_admission.py` | actual shell selects quoted installed supervisor, forwards bounded executor arguments and fails before payload when absent; omission controls | source-qualified only; legacy conservative admission and signed native build must be read back separately |
| A finite CPU-only build does not seize the model slot | native launcher core propagation; CLI `--cores`; canonical scheduler | `test_native_cpu_request_reaches_reservation`, `TestReserveCLIPropagatesBoundedCoresAndPreservesLegacyOmission`, `TestActualCPUFirstReserveDoesNotOwnModelSlot`, conservative conflict matrix | post124 corrective source; protected full checks and installed/server behavior OPEN |

G04's 2ee runtime failure is retained in its original terminal packet. The
corrective child adds bounded shared candidate qualification in
`history_query_batch.go`, cursor-v4 validation and charged-work/cancel fixtures;
original80-call MCP tests are unchanged. This core change is an independent
Astra review subject, not a passed-runtime or full original-scope claim.

G04-C3 exact transport key/enum membership maps to both `maat_reads.go` adapters
and their new `maat_membership_test.go` fixtures. Malformed composites and
prefix/suffix/substrings must fail before any private reader call; literal/default
compatibility is retained. Source corrective only; Main review/execution pending.

The same child corrects the independently observed legacy memory test-root
setup in `memory_test.go`/`memoryguard_test.go`, using the exact855 private helper
and public0755 no-chmod refusal fixture without its API adaptations. Production
mode policy and all old assertions remain unchanged; whole-core normal/race
under022 is required rather than narrowing away these tests.
