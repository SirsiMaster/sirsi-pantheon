# Sirsi Menubar

The native macOS operator surface is a SwiftUI application backed by the `sirsi` CLI. Go remains authoritative for health findings, bounded repairs, and Ma'at decisions; Swift presents that evidence and runs only the action already declared by the CLI.

## Health resolution flow

`ControlCenter.swift` links the health status to `HealthResolutionView` in `Views.swift`. Each `FindingView` uses the same numbered resolution levels: a safe action only when the latest observation is current and the finding permits it; guided recovery with a fresh recheck; and a contextual Ma'at Casebook review that retains the exact check, message, and detail.

`SirsiEngine` keeps the canonical green/amber/red status separate from observation freshness. A missing or undecodable report becomes unknown before any successful observation and stale afterward; the previous findings and status are retained. `ResultView` executes its initial command once, preserves undecodable output, and verifies a finding only from a fresh readable diagnosis. Review recording and command completion do not claim a repair.

The root `Nav` stack stays mounted while the status window is hidden, so reopening preserves the current finding and its visible action outcome. The action screen has an explicit recheck control instead of repeating the original repair command.

## Verification

Run focused native contract coverage and build from this directory:

```sh
swift test --filter CoreContractsTests
swift build
```
