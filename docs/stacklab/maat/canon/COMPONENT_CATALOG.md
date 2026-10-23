# Component catalog — Ma'at

| Component | Source surface | Write boundary | Evidence / next action |
|---|---|---|---|
| Assessors | `internal/maat` | append decision through recorder | focused tests; keep receipt current |
| Journal | Ma'at recorder | append-only JSONL | readback and integrity tests |
| Failure memory | `internal/maat/memory.go` | retained-descriptor, create-only prepared/committed evidence, incident, and transition namespaces | deterministic signature/hash, exact-scope preflight, unresolved-write debt, predecessor-bound terminal events, tamper, symlink, collision, and race fixtures; no imported action execution |
| Known-failure proposals | `internal/maat/knownfail/proposal.go` + `cmd/sirsi/maatknownfail.go` + native Ma'at workspace | create-only, readback-verified local proposal under `~/.sirsi/maat/known-failures/proposals` after explicit confirmation | local evidence is visible in the native Proposals workspace and CLI JSON; it is never a recognition rule until Stack Lab review promotes it |
| Casebook | `internal/maat/casebook` | none | deterministic projection tests |
| Host-health System One | `cmd/sirsi/maattriage.go` + native Ma'at workspace + Stack Lab handoff | confirmed local decision-journal append only | hash one complete Doctor report; test preview/confirmation, Stack Lab named-surface handoff, and pass/changes/block UI truthfulness |
| Native diagnostic resolution | `macapp/Sources/SirsiMenubar/Views.swift` + `SirsiEngine.swift` | safe repair requires explicit confirmation; Ma'at review writes only through its named CLI boundary | every diagnostic has exactly one visible outcome: bounded repair, documented command, Ma'at evidence review, or accepted completion; no informational dead-end |
| Activity resolution | `macapp/Sources/SirsiMenubar/{Views,SirsiEngine}.swift` | activity is append-only local provenance; ambiguous or failed records do not execute an action themselves | every record is classified as completed, needs review, or evidence verification; the detail view retains the exact command as technical evidence and routes unresolved records into native Ma'at guided review |
| Protected release recovery | `internal/maat/credentialpreflight.go` projected through CLI/MCP/TUI/native Ma'at surfaces | public metadata observation and explicit Casebook confirmation only; protected workflow owns credentials | canonical `recovery_plan` names missing Application, Installer, and notarization proof, then recheck; no secret/key access in Ma'at |
| Terminal Health resolution | `internal/tui/screen_health.go` + canonical console runner | confirmed Ma'at review only; no guidance command is auto-run | every guidance-only finding enters confirmation-gated evidence review, then Casebook/confirmed CLI owns explicit conclusion |
| CLI/API | `cmd/sirsi`, Horus API | request-scoped outputs | command/API tests |
| Native release bootstrap | `maat-native-release-bootstrap` in the System One recipe; release workflow and release-admission guards | trusted release job uses its matching installed supervisor/CLI, retaining actual executor admission and one bounded payload; never an admission bypass | quoted space-containing home, absent supervisor and omission controls; legacy full build grant is not finite CPU/model sharing proof; signed execution remains separate |
| Finite CPU admission | `scripts/maat/admitted-run.py`, `cmd/sirsi/maatschedule.go`, `internal/maat/schedule` | one actual executor's explicit CPU request; never mutation or retirement of another holder | launcher propagation, real CLI persisted request, actual CPU-first/model-second admission and conservative quiet/memory/unknown/unbounded negatives; corrected source is not installed qualification |
| Scoped service lifecycle | `internal/setup/launchdservice*`, `cmd/sirsi/launchdservice*` | one explicitly selected allowlisted service; default read-only, native identity/grant/four pins/drain assertion and per-service serialization required | `maat-scoped-service-lifecycle` in the System One recipe; [runbook](../../../runbooks/SCOPED-SERVICE-LIFECYCLE.md); source tests do not establish signed installed deployment or application health |
| Stack Lab integration recipe | `contracts/stacklab/maat-*` | source contract only; Stack Lab governance is SSA-owned | keep schema and implementation aligned without claiming platform authority |

Integration and software Stack Lab governance owner: SSA / Ma'at. Pantheon
implements the integrated product surfaces; Ra owns router transport, not a
second Ma'at governance authority. SHA retains hardware observation and controls.
Release state: source cataloged; installed-runtime evidence remains open.
