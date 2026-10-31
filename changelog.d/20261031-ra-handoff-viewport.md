### Fixed

Bounded the Ra handoff sheet so long fabric recipient lists stay in a horizontal scroll viewport instead of widening the sheet across the display. The form scrolls independently while Cancel and Send remain fixed and visible at narrow widths and enlarged text sizes. Mounted geometry checks cover narrow widths and enlarged text independently. Strict Cancel/Send AX visibility checks first require an independent SwiftUI Text/Button control; unavailable AX is reported as unverified, while missing production controls still fail whenever the oracle is healthy.

Refs: `contracts/stacklab/pantheon-native-workspace-v1.json`, `macapp/Sources/SirsiMenubar/Views.swift`, `macapp/Tests/SirsiMenubarTests/RaHandoffViewportTests.swift`, `docs/stacklab/pantheon-pt/canon/DESIGN.md`, ADR-030
