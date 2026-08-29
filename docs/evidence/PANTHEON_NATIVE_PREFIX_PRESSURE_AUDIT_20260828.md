# Pantheon Native Prefix-Pressure Audit — 2026-08-28

## Scope and provenance

- Candidate: `520eb5d3a1e53ce2acf76603ebd4692920a2361b`
- Tree: `25429ff231952a0ff9f5a0ebbaa03fb38138a87b`
- Surface: `macapp/Sources/SirsiMenubar/SNEControl.swift`
- Scope: fixture-only SwiftUI rendering and source/test review. No SNE service,
  model, local capability token, permission, Tailscale, or host state was read
  or changed.

## Evidence

`swift test --package-path macapp` exited 0: nine XCTest cases passed. The
tests cover exact lifecycle identity, locked-session decoding, unavailable
owner-action recovery wording, owner confirmation, authorization binding,
unavailable evidence, terminal execution/retention receipt decoding, and
receipt-identity traversal rejection.

The built-in renderer was run twice with
`--prefix-pressure-fixture-snapshot`, once at the declared narrow width (360pt)
and once at the wide width (900pt). The owner-confirmation image hashes are:

| Width | Path | SHA-256 |
| --- | --- | --- |
| 360pt | `/private/tmp/pantheon-v2314-prefix-pressure-360/prefix-pressure-confirmation.png` | `af8f4177231f3df9a2e653bc62a971f5dfdff7bb8481f9bc32fda467530cd85d` |
| 900pt | `/private/tmp/pantheon-v2314-prefix-pressure-900/prefix-pressure-confirmation.png` | `9e2429e786c9872469afa201671119151ab916ddfbc65bfc38c6fb3ccfb6f534` |

The revised 380pt confirmation image is
`/private/tmp/pantheon-v2314-prefix-pressure-snapshots-r2/prefix-pressure-confirmation.png`,
SHA-256 `5d2a977910ff076ae04daeff58528d32b1852f5adcd3e4fb968d895cf88faf41`.
It replaces the renderer's malformed editable-field blocks with the truthful
fixture-only label `Exact ID required before read`. Live text fields and read
actions remain unchanged.

## Native technical audit

| Dimension | Score | Evidence-bound result |
| --- | ---: | --- |
| Accessibility | 3/4 | Source provides labels/identifiers and explicit unavailable/owner-confirmation copy; live VoiceOver traversal is unobserved. |
| Performance | 3/4 | The fixture renderer is side-effect-free and the screen uses bounded catalog data; live resize/scroll profiling is unobserved. |
| Appearance and theming | 3/4 | Both narrow and wide dark fixtures render without clipping; a live light-appearance inspection remains unobserved. |
| Platform conformance | 3/4 | The surface is native SwiftUI/AppKit with standard buttons/text fields; its custom in-panel back navigation needs an observed interactive session. |
| Adaptivity | 3/4 | 360pt and 900pt fixture images rendered without text or control overflow; keyboard/input behavior at those sizes is unobserved. |
| **Total** | **15/20** | **Good source/fixture readiness; not an interactive accessibility acceptance.** |

## Findings

### P1 — live accessibility traversal remains unproven

The fixture output and accessibility labels do not prove keyboard focus order,
VoiceOver traversal, or the dynamic live state of enabled receipt controls.
This is an acceptance gap, not evidence of a broken traversal. It requires a
separately admitted local accessibility session against the exact candidate.

### P2 — fixture and live editable receipt controls intentionally differ

ImageRenderer cannot faithfully rasterize AppKit-backed editable fields. In
snapshot mode, the panel therefore renders the explicit prerequisite rather
than an interactive input. The live branch retains the exact-ID TextField and
Read action. The distinction is intentional and must remain stated wherever
these images are used.

## Positive findings

- The authorization copy makes the ownership boundary explicit: Pantheon
  records visible owner authorization while SNE owns decision and execution.
- Unavailable receipt evidence renders as unknown, never as a guessed action.
- The snapshot harness does not invoke any command, network request, model
  launch, or permission prompt.
- Narrow and wide fixture images preserve hierarchy without clipping.

## Disposition

Source and fixture visual evidence are accepted for the exact candidate above.
Interactive VoiceOver/keyboard acceptance remains owner-gated and unclaimed.
