- Fix Ma'at root System One scroll retention across sidebar workspace switches.
  The session now rejects late native offset callbacks unless Ma'at is still
  selected at its root. Add a mounted desktop/AppKit fixture for scroll,
  unmount, stale callback, and remount restoration; the fixture waits until the
  native observer is attached and initial restoration is complete before
  scrolling. The offset remains window-local and recovery/detail panes remain
  independent.
- Refs: `docs/stacklab/pantheon-pt/canon/DIAGRAMS_RUNTIME_NATIVE.md` PT-P16,
  `contracts/stacklab/pantheon-native-workspace-v1.json`, and
  `docs/ADR-018-NATIVE-MAC-APP.md`.
