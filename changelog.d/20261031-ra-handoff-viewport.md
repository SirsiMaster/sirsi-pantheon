### Fixed

Bounded the Ra handoff sheet so long fabric recipient lists stay in a horizontal scroll viewport instead of widening the sheet across the display. The form scrolls independently while Cancel and Send remain fixed and visible at narrow widths and enlarged text sizes. The mounted regression uses the established visible, nonactivating panel setup, checks an inert native-button AX control, and retains a bounded tree diagnostic when production controls are missing.

Refs: `contracts/stacklab/pantheon-native-workspace-v1.json`, `macapp/Sources/SirsiMenubar/Views.swift`, `macapp/Tests/SirsiMenubarTests/RaHandoffViewportTests.swift`, `docs/stacklab/pantheon-pt/canon/DESIGN.md`, ADR-030
