# Component catalog — Ma'at

| Component | Source surface | Write boundary | Evidence / next action |
|---|---|---|---|
| Assessors | `internal/maat` | append decision through recorder | focused tests; keep receipt current |
| Journal | Ma'at recorder | append-only JSONL | readback and integrity tests |
| Failure memory | `internal/maat/memory.go` | retained-descriptor, create-only evidence and incident namespaces | deterministic signature/hash, exact-scope preflight, tamper, symlink, collision, and race fixtures; no imported action execution |
| Casebook | `internal/maat/casebook` | none | deterministic projection tests |
| Host-health System One | `cmd/sirsi/maattriage.go` + native Ma'at workspace + Stack Lab handoff | confirmed local decision-journal append only | hash one complete Doctor report; test preview/confirmation, Stack Lab named-surface handoff, and pass/changes/block UI truthfulness |
| Native diagnostic resolution | `macapp/Sources/SirsiMenubar/Views.swift` + `SirsiEngine.swift` | safe repair requires explicit confirmation; Ma'at review writes only through its named CLI boundary | every diagnostic has exactly one visible outcome: bounded repair, documented command, Ma'at evidence review, or accepted completion; no informational dead-end |
| Protected release recovery | `internal/maat/credentialpreflight.go` projected through CLI/MCP/TUI/native Ma'at surfaces | public metadata observation and explicit Casebook confirmation only; protected workflow owns credentials | canonical `recovery_plan` names missing Application, Installer, and notarization proof, then recheck; no secret/key access in Ma'at |
| Terminal Health resolution | `internal/tui/screen_health.go` + canonical console runner | confirmed Ma'at review only; no guidance command is auto-run | every guidance-only finding enters confirmation-gated evidence review, then Casebook/confirmed CLI owns explicit conclusion |
| CLI/API | `cmd/sirsi`, Horus API | request-scoped outputs | command/API tests |
| Stack Lab integration recipe | `contracts/stacklab/maat-*` | source contract only; Stack Lab governance is SSA-owned | keep schema and implementation aligned without claiming platform authority |

Integration owner: Pantheon Ma'at lane. Stack Lab governance owner: SSA. Release state: source cataloged; installed-runtime evidence remains open.
