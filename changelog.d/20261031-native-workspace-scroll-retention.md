### Fixed

- Keep Ma'at's root System One primary-pane vertical position when switching
  away and back during the same window session. Recovery and detail panes do
  not share that retained viewport. Restore waits for a ready layout, clamps to
  the available range once, and yields to user movement while waiting. The
  offset remains in memory only; workspace selection remains the only persisted
  navigation value, and inactive views remain unmounted.

Refs: `docs/stacklab/pantheon-pt/canon/DIAGRAMS_RUNTIME_NATIVE.md` PT-P16,
`docs/ADR-018-NATIVE-MAC-APP.md`, and
`contracts/stacklab/pantheon-native-workspace-v1.json`.
