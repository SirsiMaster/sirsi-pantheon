# Traceability matrix — Ma'at

| Requirement | Source | Verification/evidence | State |
|---|---|---|---|
| Assessments preserve facts and explanation | `internal/maat/`, `docs/qa/MAAT_SYSTEM_ONE_CATALOG.md` | focused Ma'at tests and receipts | source-bound; current evidence required |
| Decisions are append-only | Ma'at journal recorder | journal write/readback tests | source-bound |
| Casebook is read-only | `internal/maat/casebook/` | projection and API tests | source-bound |
| Native diagnostics always resolve or complete | `macapp/Sources/SirsiMenubar/Views.swift`, `SirsiEngine.swift` | `CoreContractsTests.testEveryDiagnosticHasAClosedNativeResolutionRoute`; SwiftUI surface review | source-bound |
| Credential findings have a protected recovery route | `internal/maat/credentialpreflight.go`, `cmd/sirsi/maatpreflight.go`, `macapp/Sources/SirsiMenubar/MaatCasebookView.swift` | Go recovery-plan tests; `CoreContractsTests.testCredentialPreflightAlwaysProvidesAProtectedRecoveryPlan`; native recheck/Stack Lab route | source-bound; commercial proof remains separate |
| Missing evidence fails honestly | catalog contract | negative tests and release receipt | source-bound |
| Installed/release behavior | release workflow and host evidence | exact release/host receipts | OPEN |
