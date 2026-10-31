- Honor the global `--json` contract for `sirsi router show` and `router reassign`: show emits the canonical typed item, and a successful reassign emits a compact item/status receipt. Preserve human output and return operation errors without success output. Add injected Cobra fixtures for both modes. Source candidate only; qualification and installed CLI verification remain separate.

Refs: `docs/user-guides/router-service.md`, `docs/stacklab/ra/canon/TRACEABILITY_MATRIX.md`, ADR-068, ADR-072 C5
