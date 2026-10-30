### Ma’at installed lifecycle authority composition

- Move the existing exact packaged-engine identity, no-follow inspection, and
  bounded Developer-ID verification into a dependency-leaf package shared by
  setup and Ma’at, breaking the Darwin import cycle without changing authority.
- Keep setup compatibility entry points and the opaque Ma’at token minting
  boundary; canonical paths, Team ID, executable identity checks, and the empty
  empirical acceptance inventory remain unchanged.

Refs: `docs/stacklab/pantheon-pt/canon/MAAT_SOURCE_LIFECYCLE.md`, `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, ADR-004
