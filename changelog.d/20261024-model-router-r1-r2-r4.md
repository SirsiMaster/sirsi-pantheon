- Correct model-router v1 completion: `sirsi ask` now uses the router's
  once-only execution path, which may fall down once to a qualified local
  provider for structured remote rate or recognized quota exhaustion. Add
  privacy-dominant per-request lane overrides, an explicit `--route` opt-in
  that leaves Doctor alarms visible, and a bounded private redacted decision
  journal. Unknown streaming/determinism/JSON capabilities and the separate
  provider-readiness correction remain unqualified. Refs: ADR-054,
  `docs/stacklab/MODEL_ROUTER_V1_RECIPE.md`,
  `docs/stacklab/pantheon-pt/canon/TRACEABILITY_MATRIX.md`.

- Preserve context-window qualification across local, remote-escalation and
  judgment routes, including malformed-class and negative-context refusal.
- Preserve Cobra-selected human/JSON writers and themed output while adding
  the new generation and streaming paths.
- Surface synthesized stream EOF/cancellation once after partial output,
  retaining decision-journal failures without fallback or replay. Combined
  release qualification remains separate from the donor's finite evidence.
