### Fixed

- Casebook search now matches canonical case IDs before linked-state qualification, and native review results can open an existing case only after a unique complete ID-and-evidence match. Query edits clear stale exact selection while bounded refresh and continuation retain their current query and snapshot contract; the native source guard now checks the typed handoff and browse fallback.

Refs: `docs/stacklab/maat/canon/G04_BOUNDED_SURFACES.md`, `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`
