### Ma’at lifecycle import and authority cleanup

- Normalize imports in the four lifecycle files flagged by the protected lint
  gate and remove the unused setup validation wrapper after confirming it has no
  callers; validation remains in the shared packaged-engine leaf.

Refs: `contracts/stacklab/maat-system-one-recipe-v1.json`, `docs/stacklab/pantheon-pt/canon/MAAT_SOURCE_LIFECYCLE.md`, `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, ADR-004
