# Traceability matrix — Ma'at

| Requirement | Source | Verification/evidence | State |
|---|---|---|---|
| Assessments preserve facts and explanation | `internal/maat/`, `docs/qa/MAAT_SYSTEM_ONE_CATALOG.md` | focused Ma'at tests and receipts | source-bound; current evidence required |
| Decisions are append-only | Ma'at journal recorder | journal write/readback tests | source-bound |
| Casebook is read-only | `internal/maat/casebook/` | projection and API tests | source-bound |
| Missing evidence fails honestly | catalog contract | negative tests and release receipt | source-bound |
| Installed/release behavior | release workflow and host evidence | exact release/host receipts | OPEN |
