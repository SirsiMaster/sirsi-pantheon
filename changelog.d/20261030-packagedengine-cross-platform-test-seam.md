### Packaged-engine authority test seam

- Keep the fixed codesign argument assertion in the shared Darwin/Linux leaf
  package so Linux lint sees the same production command coverage as Darwin.
- Remove the Darwin-test-only setup wrapper; exported setup entry points and the
  packaged-engine verification checks remain unchanged.

Refs: `docs/stacklab/pantheon-pt/canon/MAAT_SOURCE_LIFECYCLE.md`, `contracts/stacklab/maat-system-one-recipe-v1.json`, ADR-004
