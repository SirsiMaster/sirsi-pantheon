# Pantheon Native Prefix-Cache Pressure Acceptance — 2026-08-28

## Scope and provenance

This evidence is bound to the copied Pantheon candidate based on commit
`117d5d52ad59fca62678600054052f930d2761ae`, tree
`f915424315761e5be24c9a8c035c29155eb20af7`. It is a source/test acceptance
record only. It does not claim an installed application, a released package,
or live SNE execution.

The SNE-owned copied-package receipt gate is the governing external boundary:
`SNE_PANTHEON_PREFIX_PRESSURE_COPIED_INTEGRATION_GATE_V1_20260828.md`,
SHA-256 `0f5ab8219099922b41fffd2777b310bcfff48b327b31677d2c0e3d0d3b7e894d`.
Pantheon measures and requests owner authorization; SNE alone decides,
executes, recovers, replays, and retains.

## Native surface contract accepted by source and fixtures

- The SNE control screen renders a measured lifecycle observation when one is
  present, including host, request identity, pressure source, and a truncated
  observation SHA.
- **Measure cache pressure** is an explicit native owner action. It prepares a
  fresh observation and confirmation; it does not start SNE or alter cache
  state.
- **Authorize SNE evaluation** appears only after the owner-visible prepared
  confirmation. Its POST contains the exact request ID, observation SHA,
  single-use confirmation token, and action hash.
- The accepted authorization is rendered as external SNE decision/execution
  work, never as a Pantheon cache decision or execution result.
- Execution and retention reads require an operator-entered exact identity.
  The view cannot enumerate, infer, or construct receipt paths.
- Unavailable evidence is rendered as unknown. The UI makes no retry, live
  pressure, completed, or retention claim from an unavailable response.
- The surface has identifiers and VoiceOver labels/hints for measure,
  authorization, exact receipt fields, and receipt reads. The hints state the
  ownership boundary and absence of hidden mutation.

## Fixture verification

Command run from this candidate:

```text
swift test --package-path macapp
```

Result: exit `0`; six `SNEPrefixCachePressureTests` passed with zero failures.
The package now has a real `SirsiMenubarTests` target rather than reporting
`no tests found`.

The fixtures prove decoding and source presentation boundaries for:

1. owner-confirmation-required with no authorization;
2. authorization-accepted bound to the exact request ID and observation SHA;
3. unavailable execution evidence with no receipt; and
4. failed/interrupted execution plus retention receipt fields without treating
   either as a live metric; and
5. exact receipt/retention identities that reject empty values and path
   traversal before a read-only route can be requested.
6. the fixture-only Measure → owner-confirmation → Authorize interaction,
   including the exact GET/POST routes, local capability boundary, request ID,
   observation SHA, confirmation token, and action hash.

`git diff --check` also exited `0`.

## Fixture-only native visual evidence

The native renderer was deliberately invoked through the new
`--prefix-pressure-fixture-snapshot` mode, not the existing live snapshot
harness. It makes no Sirsi command, network request, token read, model action,
or permission request. The command exited `0` and produced these PNGs:

| State | PNG SHA-256 |
| --- | --- |
| Measure | `9bacd1bc579dc380bf3cc20fcd3e73c58293ef56f5997e0354eb9ece78dd340f` |
| Owner confirmation | `c36abeef9b6b2d5c9130e6f661b4f81224b13d22332f95e7d6eec799746ffe75` |
| Authorization with unavailable execution evidence | `f89dd45d5eaf234e24432097e02b994da0854fa969d1b8ff55b0f81f80e329bf` |
| Terminal execution plus retention | `ef2c199176e84c1817d718d24efd09ce7a20e841c50df08228ef97031d3aa90f` |

The files are held under
`/private/tmp/pantheon-prefix-pressure-native-fixture-20260828/`. They show
the real SwiftUI surface for the reviewable measure, confirmation,
authorization/unavailable, receipt, and retention states. They are fixture
evidence, not an installed-app or live-host screenshot.

The fixture renderer also rejects malformed appearance arguments before an
application instance is created: `--appearance wrong` exited `2` with
`--appearance requires light or dark`.

## Honest remaining proof

This is **partial acceptance**, not full interaction/accessibility acceptance.
The fixture renderer and transport prove native rendering plus the exact
owner-action transition without live state. They do not prove keyboard focus
order or VoiceOver traversal in a running accessibility session, so neither is
claimed. That remaining proof requires an admitted native accessibility session
bound to this candidate. No deployment, service restart, permission/security/
Tailscale change, or SNE runtime-mathematics change occurred.
