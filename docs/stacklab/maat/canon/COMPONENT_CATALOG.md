# Component catalog — Ma'at

| Component | Source surface | Write boundary | Evidence / next action |
|---|---|---|---|
| Assessors | `internal/maat` | append decision through recorder | focused tests; keep receipt current |
| Journal | Ma'at recorder | append-only JSONL | readback and integrity tests |
| Casebook | `internal/maat/casebook` | none | deterministic projection tests |
| Host-health System One | `cmd/sirsi/maattriage.go` + native Ma'at workspace + Stack Lab handoff | confirmed local decision-journal append only | hash one complete Doctor report; test preview/confirmation, Stack Lab named-surface handoff, and pass/changes/block UI truthfulness |
| Native diagnostic resolution | `macapp/Sources/SirsiMenubar/Views.swift` + `SirsiEngine.swift` | safe repair requires explicit confirmation; Ma'at review writes only through its named CLI boundary | every diagnostic has exactly one visible outcome: bounded repair, documented command, Ma'at evidence review, or accepted completion; no informational dead-end |
| Protected release recovery | `macapp/Sources/SirsiMenubar/MaatCasebookView.swift` + `internal/maat/credentialpreflight.go` | public metadata observation and explicit Casebook confirmation only; protected workflow owns credentials | missing Application, Installer, or notarization proof resolves to an ordered handoff plus native recheck; no secret/key access in Ma'at |
| Terminal Health resolution | `internal/tui/screen_health.go` + canonical console runner | confirmed Ma'at review only; no guidance command is auto-run | every guidance-only finding enters confirmation-gated evidence review, then Casebook/confirmed CLI owns explicit conclusion |
| CLI/API | `cmd/sirsi`, Horus API | request-scoped outputs | command/API tests |
| Wing recipe | `contracts/stacklab/maat-*` | source contract only | keep schema and implementation aligned |

Owner: Pantheon Ma'at lane. Release state: source cataloged; installed-runtime evidence remains open.
