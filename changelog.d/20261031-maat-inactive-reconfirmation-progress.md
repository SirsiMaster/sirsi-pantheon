## Fixed

- Preserve the bounded terminal-page read cursor across explicit review and confirmation only for the typed binary-drift inactive disposition. Confirmation performs a new casebook read; the cursor is not current-state proof. The existing command validates the exact host and strict journal, then rechecks conflicting or duplicate rows under its mutation lock before recording. Other actions do not inherit this cursor path.

Refs: `docs/stacklab/maat/canon/TRACEABILITY_MATRIX.md` (retained Ma’at history), ADR-070.
