### Fixed

- Keep each native workspace's vertical scroll position when switching to
  another workspace and back during the same window session. Positions remain
  in memory only; workspace selection remains the only persisted navigation
  value, and inactive views remain unmounted.

Refs: `docs/stacklab/pantheon-pt/canon/DIAGRAMS_RUNTIME_NATIVE.md` PT-P16,
`docs/ADR-018-NATIVE-MAC-APP.md`, and
`contracts/stacklab/pantheon-native-workspace-v1.json`.
