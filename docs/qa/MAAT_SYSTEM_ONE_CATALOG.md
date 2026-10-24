# Ma'at System One Catalog

**Wing:** `stacklab.wing.maat`

**Product role:** Pantheon's guaranteed local intelligence layer
**Runtime requirement:** none beyond the Pantheon binary and local state

Ma'at System One is the non-LLM foundation that ships with every Pantheon
installation. It has one authority boundary: recorded Ma'at facts are appended
to the decision journal, then Casebook derives a read-only local view. An LLM
agent may read this view or add an independently evidenced decision, but it is
never required for search, classification, prioritisation, or evidence links.

## Canonical local flow

```
observe -> assess -> append Ma'at decision -> Casebook -> CLI / MCP / Horus / terminal console / native app
```

`internal/maat` owns assessment semantics. `internal/maat/casebook` owns only
deterministic projection. The Casebook never writes a determination, starts a
job, reserves a resource, or silently turns a fact into authorization.

## Component recipe manifest

The machine-readable recipe is
[`contracts/stacklab/maat-system-one-recipe-v1.json`](../../contracts/stacklab/maat-system-one-recipe-v1.json).
It lists every source module, test suite, input, output, write boundary, and
safe independent upgrade recipe in this wing:

1. Ma'at core report and assessor interface
2. Canon linkage assessor
3. Coverage assessor and bounded cache
4. Pipeline/CI assessor and failure classifier
5. Strict local System One screen and calibration record
6. Local host-health observation and screen writer
7. Non-executing, hash-bound commercial-release source preflight
8. Non-secret Developer ID credential readiness and protected-notarization route
9. Pulse, proof, and platform integrity collectors
10. Append-only decision journal and report recorder
11. Deterministic local Casebook
12. Shared-machine and rail scheduler/conflict detector
13. CLI surfaces for audit, scheduling, screens, calibration, and search
14. Horus API and visible caseboard
15. Read-only MCP Casebook projection for agent clients
16. Five-screen terminal console Activity/Casebook mode
17. Native finding-to-review-to-acceptance resolution flow
18. Confirmed managed LaunchAgent disabled-override recovery and post-action recheck
19. The Stack Lab wing and recipe contracts themselves

An upgrade begins at one manifest component, follows that component's listed
tests and boundary, then updates the receipt/canonical contract. Components do
not get silently co-upgraded because they share a deity name.

## Accounted source surfaces

| Surface | Recorded unit | Current writer | Casebook treatment |
| --- | --- | --- | --- |
| Quality audit | one decision per assessment plus report summary | `sirsi maat audit` via `maat.RecordReport` | `assessment` cases, with pass/warning/fail priority |
| Canon linkage | audit assessment with ADR/rule standard | `CanonAssessor` through audit report | `assessment` cases, evidence is the report generation |
| Coverage | audit assessment with threshold and remediation | `CoverageAssessor` through audit report | `assessment` cases, failed cases are urgent |
| Pipeline/CI | audit assessment with the current CI run/log classification | `PipelineAssessor` through `sirsi maat audit` | `governance` or `assessment` cases, never synthetic success |
| Stack Lab guard/review | decision with a receipt reference | Ma'at/approved caller through the decision journal | `governance` cases linked to the receipt |
| ADR070 v2 source import | explicit `maat.adr070.source-screen`, exact required schema2 JSON, per-finding/recipe/calibration identities, declared-only floor, unqualified state and separately selected head | `sirsi maat screen --input <source-screen.json> --expect-head <head> --confirm` via `maat.ScreenSource` | `governance` case: failed/incomplete declared floor `block`, otherwise unqualified `escalate`; imported qualified claims reject. No real checks, model qualification, mutation or binding approval are inferred |
| Host-health System One screen | one complete local Doctor report and resolved local host identity, hashed before classification | `sirsi maat triage [--confirm]`, native **Observe this Mac**, or the named Stack Lab component handoff into the same native System One surface | `host-health` System One case with the exact diagnostic digest, active warning/critical findings, and a visible distinction between a recorded Casebook result and a healthy pass; the Stack Lab handoff never interprets recipe text as a command, and observation alone never repairs, kills, installs, or invokes an LLM |
| System One calibration | one recorded local auto-pass and a distinct independent final review outcome | `sirsi maat calibrate --screen-evidence <ref> --frontier-evidence <ref> --frontier-gate <pass|changes|block>` | durable calibration evidence and overturn-rate history; replay, missing screen, non-pass, and ambiguous links reject |
| Release-contract preflight | regular non-symlink local packaging/workflow/Stack Lab sources, captured and hashed in-process | `sirsi maat preflight release --root <checkout> [--confirm]` via `maat.PreflightReleaseContract`, including the native Ma'at System One inspect-then-confirm flow | delivery-bound System One evidence that never runs a build/package/signing command; source-floor failures retain a bounded repair hint, and a green source floor remains distinct from credentialed release proof |
| Release credential readiness | public local Developer ID certificate names and fingerprints for Team `9D382WV988`; no key or secret material | `sirsi maat preflight credentials [--confirm]` or the native Ma'at **Check signing readiness** flow | a hash-bound readiness screen with one exact protected-workflow route for missing Developer ID Application, Developer ID Installer, or notarization proof; a confirmed observation is projected through the same Casebook and never itself signs, packages, notarizes, publishes, or authorizes a release |
| Knowledge refresh | retained local knowledge cache through the legacy ingestion adapter | `sirsi maat knowledge refresh [--source … --since … --profile … --all-profiles]` and dashboard action `maat/knowledge/refresh`, presented as **Refresh Ma'at knowledge** | Ma'at remains the user-facing knowledge/decision authority; the compatibility adapter is not surfaced as a second operator product, and refresh cannot export knowledge or open a browser |
| Shared host, rail, and device capacity | explicit core and RAM-capacity/floor-share configuration plus reservation grant/refusal/queue/release | `sirsi maat capacity get/set`, `sirsi maat capacity memory get/set`, and `sirsi maat reserve` | `allocation` cases tied to resource and holder; unknown peer RAM remains unconfigured rather than guessed |
| Cede, handback, or counter-request | explicit router-derived decision record | authorized Ma'at producer | `allocation` cases; pending is high priority, never inferred as granted |
| Contention and incidents | explicit decision with evidence | approved Ma'at producer | `contention` cases; blocked/refused/failed is urgent |

Every row has the same required journal shape: time, host, kind, requester,
assessed facts, determination, explanation, and optional affected/resource/
evidence links. The evidence reference is preserved verbatim; Casebook does not
claim a local record is a remote attestation.

### Current producer compatibility correction (2026-10-24)

This is a correction to the existing `maat-decision-journal` component, not a
new component or authority. Conflict-check records the explicit `SIRSI_AGENT_ID`
caller; without one it records the command producer `sirsi maat conflict-check`,
not a human or the optional reservation holder. The holder remains `affected`.
A checker failure remains `unverified`. Cede and floor projections retain their
real actors and post-commit projection error reporting, but serialize `time`
instead of `ts`. Historical records and the strict read API remain unchanged.

Source QA trace (fixtures authored; normal/race/negative/vet qualification
**unrun** in this checkpoint):

| Contract | Existing producer source | Inert strict-reader fixture |
| --- | --- | --- |
| No-holder and checker-error records have truthful nonblank requester | `cmd/sirsi/maatschedule.go` | `TestMaatConflictCheckJournalStrictCompatibility` (caller, absent and whitespace identity; in-memory stores, no host probe) |
| Cede records retain actor/evidence with canonical timestamp | `cmd/sirsi/maatcede.go` | `TestCedeDecisionJournalStrictCompatibility` (owned journal only, no cede transition) |
| Floor records retain holder/affected/evidence | `cmd/sirsi/maatschedule.go` via the cede projection | `TestLogFloorGrantJournalStrictCompatibility` (owned journal, no actual reservation) |
| Committed projection failures never become false success | unchanged writer/error paths | existing `TestAppendCedeDecision_*`, `TestCedeDecisionLedger_*ProjectionFailureIsReported`, `TestLogFloorGrant_ProjectionFailureIsReported` |

The compatibility oracle is the existing `maat.FileDecisionJournal.Recent`, not
a duplicated validator. Generic map append remains unchanged. Source readiness
does not establish installed producer adoption, a repaired journal, attribution
of a loaded legacy writer, or native UI completion. Retain those limits in the
release and installed qualification receipts.

System One normalizes deterministic floor checks by name and findings by their
unique identity before it applies policy or computes the verdict receipt. An
equivalent observation therefore yields one stable evidence hash; duplicate
finding identities and noncanonical verdict replay reject rather than creating
ambiguous case history.

## Product contract

### ADR070 corrective source slice, not full completion

The existing `maat-system-one-screen` component now separates amended v2 source
imports from legacy v1 journal reads and deterministic host/preflight semantics.
The former implicit v1 source-import CLI path refuses: callers must explicitly
provide v2 and `--expect-head`. The existing native picker is migrated to that
same contract, with an editable expected-head field, copyable typed format,
non-writing `--preview`, and reviewed raw-digest confirmation. Errors retain
editable input and a choose-file/review recovery; they never silently downgrade.
Host observation and both closed host repairs remain model-free and unchanged. The shared
Casebook/CLI/MCP/Horus/TUI/native projections retain the v2 gate and journal
explanation; they do not gain execution or binding review authority.

`floor_evidence` and the journal explanation label v2 checks **declared-only**.
Import never executes or authenticates the floor, recipe, evidence or calibration
set. Lowercase 64-hex evidence/recipe/calibration identities are structural
declarations, not locally earned qualification. No imported qualified string is
accepted. All valid nonblocked imports escalate under R2; R1 failed/incomplete
floor still blocks first. Missing structural keys reject under R0.

Source QA trace, **unrun until separately admitted qualification**:

| Requirement | Source | Inert fixture |
| --- | --- | --- |
| R0–R8 exact precedence, qualified context private to policy fixtures | `internal/maat/sourcescreen.go` | `TestSourceScreenExactRulePrecedence` |
| Required zero/false fields, duplicate/unknown/case-alias/null/type/EOF/size rejection; stale expected head and evidence identities | same | `TestSourceScreenStrictJSONRequiredClosedKeys`, `TestSourceScreenRejectsInvalidSemanticInputs` |
| No qualified production/persistence path, no forged gate | same and version-dispatched `ValidateMaatVerdict` | `TestSourceScreenJournalAndLegacyCompatibility` |
| CLI v2 refusal/confirmation/head and declared floor | `cmd/sirsi/maatscreen.go` | `TestMaatScreen*` (owned regular inputs and injected journal only) |
| Shared projection review route and floor route retain provenance | `internal/maat/casebook` | `TestSourceScreenV2ProjectsReviewWithoutHostRepairAuthority` |
| V1 host warning and closed repairs unchanged, zero calibration requirement | unchanged host implementation | `TestMaatHostV1WarningRepairsRemainDeterministicWithoutCalibration`, existing host/preflight/calibration fixtures |
| Native picker migrated without provider-name trust, preserves exact file/head/digest consent and once-only execution/error recovery | `MaatSourceImport.swift`, `MaatCasebookView.swift` | `MaatSourceImportTests` (8 inert methods); shared fixture `contracts/maat/source-screen-v2-fixture.json` |
| Actual Go preview is read-only and its raw input digest denies same-head byte drift before confirmed record | `cmd/sirsi/maatscreen.go` | `TestMaatScreenNativePreviewExactBytesThenConfirmedImport` |

Authenticated qualification and independent outcome lifecycle (including sampled
pass/changes comparisons, drift revocation, model/prompt/recipe requalification),
installed native v2 UX qualification, and actual gate activation remain **OPEN**. Existing
calibration-link statistics do not supply that authority. No full ADR070/Ma'at,
installed adoption, test execution, three-home publication or wing byte-pin
completion is claimed by this source checkpoint. ADR070 §2.5 owns these limits.

The public caller survey is in ADR070 §2.5: only CLI/native import source JSON.
MCP and dashboard source-screen endpoints remain read-only; the dashboard's
closed `maat` action cannot append import args. TUI Casebook is read-only;
its separate confirmed health-review writer is not an import or amended gate.
No new source-import endpoint, legacy-v1 compatibility bypass, provider-name
qualification or native-only exemption is introduced. Preview and confirmed
record both call the same Go source validator with the independently entered
head. The digest pins bytes only, not their truth or external qualification.

- **Free and local:** no external JEV service, telemetry, model endpoint, or
  account is necessary.
- **Fast by construction:** JSONL append/read plus deterministic Go projection;
  no embedding, subprocess, or payload execution to inspect the casebook.
- **Portable:** each installation owns its local Casebook. A device contributes
  only records it can evidence; the router remains the cross-device handoff
  transport, not a copied policy store.
- **Inspectable:** `sirsi maat casebook [text] [--kind ...] [--status ...]`,
  `GET /api/maat/casebook`, the `maat_casebook` MCP tool, the Activity screen's
  `m` Casebook mode, and the native Casebook expose the same projection. For
  every System One case that projection includes the model provider, version,
  locality, latency, deterministic-floor outcome, and each floor-check detail;
  a verdict can never be presented as opaque automated authority.
- **Resolvable:** every open case exposes its retained evidence and a truthful
  next step. System One finding claims, locations, evidence, and prescribed
  recovery steps are projected unchanged through CLI, MCP, Horus, terminal,
  and native Casebook surfaces. The native Casebook and explicit confirmed CLI
  commands can record owner review/acceptance; the terminal console stays
  read-only so it never turns a keystroke into an unreviewed conclusion or a
  claimed repair. A failed deterministic floor has a canonical three-level
  route: inspect the exact failed evidence, apply the bounded producer
  correction, then re-screen and explicitly record the result. A
  producer-supplied recovery step is never auto-executed.
- **Release-contract guidance:** the native System One surface binds a
  preflight to the explicitly selected repository, shows every deterministic
  check and its repair hint, and requires a separate confirmation before it
  records one Casebook evidence row. The inspection is source-only: it never
  starts a build, package, signing, notarization, network, or release action.
- **Credential guidance:** Ma'at can inspect public Developer ID identity
  metadata for the exact Pantheon team without opening private keys or secrets.
  It names the precise protected-workflow prerequisite still missing and, after
  confirmation, projects that evidence through the same Casebook. A green
  public-identity floor is never substituted for notarization proof.
- **No stranded alarms:** when a native diagnostic alarm has no safe automatic
  repair, the finding view offers a confirmed Ma'at review, then connects to the
  shared Casebook acceptance flow. A conclusion is visibly distinct from a
  repair and an unsuccessful write leaves the original finding open with retry
  guidance.
- **Confirmed recovery:** when the diagnostic has an exact safe repair, the
  native surface presents the precise command and requires confirmation before
  it changes state. The disabled-launchd recovery is bounded to the captured
  disabled-label snapshot and to managed labels with a regular non-symlink
  LaunchAgent plist; it honors existing quarantine markers, never treats a
  confirmation as permission to revive an unrelated unloaded service, separates
  enable-only from bootstrap outcomes, and reruns the diagnostic.
- **Fail honest:** unavailable journal input returns an error/503. Unknown case
  status is rejected. Missing evidence stays missing; it is never invented.

## Stack Lab status

This catalog and `contracts/stacklab/maat-wing-v1.json` are source candidates
until merged to the owning repository's `origin/main` and byte-pinned by
`SirsiMaster/sirsi-stacklab`, per ADR-066. A local checkout or unpushed branch
is useful for review but is not a canonical wing record.
