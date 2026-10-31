### Fixed

The canonical Ma’at journal append now rejects short writes, attempts one close after write or sync failures, and reports close-only errors. Write or sync failures remain the primary error when close also fails. Per-call injected I/O fixtures cover complete writes and failure ordering without a package-global provider.

Refs: `docs/ADR-004-MAAT-QA-GOVERNANCE.md`, `docs/ADR-061-DURABLE-CONTINUOUS-EXECUTION-ENFORCEMENT.md`, `docs/stacklab/maat/canon/TRACEABILITY_MATRIX.md`, `contracts/stacklab/maat-system-one-recipe-v1.json`
