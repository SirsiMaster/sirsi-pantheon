# Pantheon v0.23.14 Explicit Local Control

**Status:** source and focused-test accepted; no live runtime or release claim

## Purpose

The native macOS shell owns an optional local control-plane child only after the
operator selects **Start local control**. This closes the packaged-surface
ambiguity where the embedded Go control engine could otherwise carry legacy
resident menu-bar duties despite the Swift app being the shipped UI.

## Source boundary

- `cmd/sirsi-menubar/main.go` recognizes `SIRSI_HEADLESS=1` solely for the
  Swift-owned loopback child. In that mode it does not start the guard bridge,
  periodic scan, live-refresh loop, or CTR resident thread.
- `macapp/Sources/SirsiMenubar/SNEControl.swift` resolves only the embedded
  `Contents/Library/Helpers/pantheon-engine`, starts it only through a visible
  native action, polls a bounded fifteen times, and accepts readiness only from
  a fresh successful loopback response.
- `macapp/Sources/SirsiMenubar/AppDelegate.swift` terminates the owned child on
  native-app termination. The child is not installed as a service or detached.
- `macapp/Tests/SirsiMenubarTests/SNEControlReadModelTests.swift` proves that a
  missing or non-executable helper is not resolved.

The native action explicitly says it starts only local loopback control and
does not start SNE inference. No automatic retry, runtime mutation, permission
prompt, Tailscale interaction, or ambient SNE launch was added.

## Verification

Run in the isolated candidate before commit:

```text
swift test --package-path macapp
exit 0 — 11 XCTest cases passed

go test ./cmd/sirsi-menubar
exit 0 — package passed (linker warning: duplicate -lobjc only)

git diff --check
exit 0

bash scripts/verify-menubar-release-contract.sh
exit 0 — accepted=true; canonical_entrypoint=cmd/sirsi-menubar;
channels=dmg,standalone; permission_silence=persisted_projection
```

## Boundaries still unproven

No local helper process was launched for this evidence: doing so would mutate
the protected runtime and create local control-plane state. Consequently this
record does not claim live loopback readiness, child termination observation,
accessibility traversal, signing/notarization, installation lifecycle, M1/M5
sustained resource behavior, or public release readiness.

## Release disposition

This is a source/test closure for explicit ownership and containment only. The
published `v0.23.13-beta` identity remains unchanged. `v0.23.14-beta` remains
an untagged local candidate until its independent release gates are satisfied.
