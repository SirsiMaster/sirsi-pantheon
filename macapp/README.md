# Pantheon native Mac application

SwiftUI workspace and menu-bar companion, backed by the same bundled Go engine as the CLI. `Sources/SirsiMenubar/main.swift` selects the bounded command mode before starting `NSApplication`; `AppDelegate` owns the single application lifecycle and standard menus.

Workspace navigation belongs to `WorkspaceSession` and `Nav`: Back and Forward retain their exact frames within each workspace; new navigation clears its forward branch. `ApplicationMenu` resolves the selected workspace when an action is invoked. Views render authoritative engine responses rather than implementing their own repair policy.

`RemediationExecution` bounds capture and retains execution status. Its MainActor Sendable authorization callback is evaluated immediately before launch. Missing, changed or unavailable evidence must refuse rather than reuse a previous approval.

## Native identity protocol

`version --json` returns this native bundle’s stamped version, logical binary name and own executable URL without initializing the app. `NativeVersionProtocol` validates the bundle identifier, executable name and matching release/build stamp. Missing or malformed identity exits unsuccessfully; the response never borrows the adjacent Go executable’s identity. No Go-only commit or toolchain metadata is fabricated.

## Qualification and packaging

`Tests/SirsiMenubarTests` contains inert parser, navigation, capture, layout and resource-lifetime fixtures. Use the existing Ma’at-admitted native build/test route with isolated scratch storage. Run changed suites before the normal protected CI and release pipeline. Native version identity, successful source tests and viewport fixtures do not certify a signed installed release.

The normal release workflow builds this target and the canonical Go engine as one payload. SSA owns signing, notarization, publication and supported installation; never patch an installed bundle. After normal installation, verify the loaded app identity and the affected workflows on the actual app.

See `../docs/user-guides/native-workspace-navigation.md`, `../contracts/stacklab/pantheon-native-workspace-v1.json` and `../docs/stacklab/pantheon-pt/canon/CERTIFICATION.md` for user behavior, replaceable recipe boundaries and release acceptance.
