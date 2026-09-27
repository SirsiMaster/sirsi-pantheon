# Component catalog — Ma'at

| Component | Source surface | Write boundary | Evidence / next action |
|---|---|---|---|
| Assessors | `internal/maat` | append decision through recorder | focused tests; keep receipt current |
| Journal | Ma'at recorder | append-only JSONL | readback and integrity tests |
| Casebook | `internal/maat/casebook` | none | deterministic projection tests |
| CLI/API | `cmd/sirsi`, Horus API | request-scoped outputs | command/API tests |
| Wing recipe | `contracts/stacklab/maat-*` | source contract only | keep schema and implementation aligned |

Owner: Pantheon Ma'at lane. Release state: source cataloged; installed-runtime evidence remains open.
