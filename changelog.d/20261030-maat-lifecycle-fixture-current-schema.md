### Ma’at lifecycle qualification fixtures

- Update inert receipt fixtures to include the strict empty `host_binding`
  member alongside `boundary`; retain explicit negative checks for both keys.
- Keep the selected-head acceptance fixture pinned to its exact canonical raw
  receipt, and inventory the shared packaged-engine authority implementation
  plus existing setup authority fixtures in the System One recipe.

Refs: `contracts/stacklab/maat-system-one-recipe-v1.json`, `docs/stacklab/pantheon-pt/canon/MAAT_SOURCE_LIFECYCLE.md`, `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, ADR-004
