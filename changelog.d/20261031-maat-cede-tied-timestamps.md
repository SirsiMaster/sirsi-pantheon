### Fixed

Cede journal fixtures now identify request and response projections by their kind and cede evidence ID, so equal-second timestamps cannot change which record an assertion evaluates. Duplicate and missing projections remain failures; record-order assumptions were removed.

Refs: `docs/ADR-004-MAAT-QA-GOVERNANCE.md`, `docs/stacklab/maat/canon/TRACEABILITY_MATRIX.md`, `contracts/stacklab/maat-system-one-recipe-v1.json`
