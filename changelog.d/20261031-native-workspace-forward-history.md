### Fixed — native workspace history

- Restore Forward navigation within each workspace: Back and Forward retain the original detail identity, a new destination clears only that workspace’s forward branch, and Home clears its relevant history.
- Add View → Forward (Command-]) with availability and dispatch bound to the selected workspace. Inactive views remain unmounted; history is not restored across app restart.

Refs: docs/ADR-030-NATIVE-MENUBAR-POPOVER.md, docs/user-guides/native-workspace-navigation.md, contracts/stacklab/pantheon-native-workspace-v1.json
