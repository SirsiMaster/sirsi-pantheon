# Pantheon Candidate Release Boundary — 2026-08-28

## Exact source identity

This evidence concerns the copied prefix-pressure candidate based on
`117d5d52ad59fca62678600054052f930d2761ae`, tree
`f915424315761e5be24c9a8c035c29155eb20af7`, plus the uncommitted source-only
native fixture changes recorded in
`PANTHEON_NATIVE_PREFIX_PRESSURE_ACCEPTANCE_20260828.md`.

`VERSION` is `0.23.9-beta`. The local `v0.23.9-beta` tag resolves instead to
`9ae4677226f63ce022344446c85394f37c990b69`; it does **not** bind this candidate
or its uncommitted files. Therefore neither this candidate nor its native
fixture evidence may be represented as the tagged release.

## Verified source gates

| Gate | Evidence |
| --- | --- |
| Go engine / CLI / TUI / MCP / E2E source suite | `go test ./...` exited `0` on 2026-08-28; only duplicate `-lobjc` linker warnings were emitted. |
| Native menu-bar package | `swift test --package-path macapp` exited `0`, 6/6 prefix-pressure tests passed. |
| Fixture-only native evidence | Four SHA-bound PNGs and exact Measure → confirmation → Authorize transport proof are recorded in `PANTHEON_NATIVE_PREFIX_PRESSURE_ACCEPTANCE_20260828.md`. |
| Diff hygiene | `git diff --check` exited `0`. |

## Release gates not proven

- `homebrew/Casks/sirsi-pantheon.rb` still names `0.23.8-beta` and SHA-256
  `113684229f1d866c1edd267af620455dc9e5d655fd774b5145e6d1413031e221`.
- No DMG built from this candidate has been demonstrated to match a tag, Cask,
  bundle version, and asset SHA.
- No Developer ID signature, notarization, stapling, clean installation,
  upgrade, rollback, uninstall, or sustained resource/crash receipt is bound
  to this candidate.
- Native fixture rendering proves the shown states, but not a live keyboard
  focus or VoiceOver traversal session.

## Decision

**Source/test-ready, not release-ready.** The first release-closing sequence is
to commit the reviewed candidate, bind a matching release tag and Cask asset,
then produce Developer-ID/notarized clean-install and lifecycle receipts. Those
steps must not be inferred from source tests or from the older tag.
