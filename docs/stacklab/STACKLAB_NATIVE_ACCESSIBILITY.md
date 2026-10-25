# Native recipe accessibility correction

Installed 134 visually rendered and scrolled the Ma'at recipe while repeated native
accessibility reads exposed only an empty scroll area. The short wing detail and
catalog buttons remained accessible. This is an observed accessibility defect;
the source diagnosis requires an old/new native comparison.

`StackLabCatalogView.swift` owns the correction. Detail content has an explicit
accessibility container, separate header/handoff/card containers with contained
children, and the original eager section stack. Text and typed navigation buttons remain
separate elements; no flattened replacement label substitutes for their actions.
The shared `MaybeScroll`, `NavLink`, engine, and catalog authority are unchanged.
Four detail/model types are internal solely so tests can host the real views.

`StackLabAccessibilityTests.swift` hosts owned, on-screen nonactivating panels,
with snapshot mode disabled. It reads AppKit's native accessibility children and
values, scrolls the real document within finite bounds, and presses the real
handoff buttons against an inert navigation owner. It uses the full source Ma'at
and release recipes and checks short wing and component text separately. No
selected repository, engine command, service, or installed-app interaction is
needed. Navigation destinations are stored, not mounted or executed.

Main's first four-test execution on 644 compiled successfully but failed every
SwiftUI text/action assertion, including the short wing and isolated card. That
does not establish an old/new production result. Preserve that failed receipt:
`/private/tmp/pantheon-stacklab-644-swift.p0kftgwl/receipt.json`, SHA256
`d411632ade2408b242b5ebdb9b50f7bede6e3499173c048511063c9af0c16885`.

The corrective fixture adds an independent, unmodified SwiftUI Text/Button
positive control. Every dependent AX test runs a fresh control before reading
any product recipe or hosting product content. It must read the original public
text/button and press the real SwiftUI Button exactly once. If this independent
control cannot be hosted, read or pressed through the public in-process reader,
that test reports explicit `XCTSkip` oracle-unavailable diagnostics. This is not
a product pass, absence verdict or completion. No product output decides whether
to skip, and no suite-wide cached control result is reused. Once the control
works, missing product content/actions, failed navigation and incomplete scroll
traversal remain test failures. Non-AX semantic/catalog tests stay active.
It uses the same native reader and host as the recipe tests. The reader uses
AppKit's public informal AX attribute/action bridge, including virtual children
that need not formally conform to the complete NSAccessibility protocol. It does
not walk NSView subviews for content, introspect SwiftUI models, substitute labels,
or grant OS accessibility permissions. Bounded node/class/role/child-count logs
distinguish an empty host or reader failure from a recipe regression. The host
stays within the existing display and never activates or makes itself key; it is
closed by each fixture. Component fields retain all assertions and are now
collected through the same live scroll traversal rather than a tall offscreen
card. No production bytes changed in this fixture correction.

Qualification remains Main-owned: an independent external control, strict
Release build, and the original four fixtures' content/action expectations
observed through the external native client. A causal baseline failure must
report missing original text/actions, not a new label or compilation failure.
Preserve baseline failure, corrected logs, source pins, and diagnostics. The
in-process assertions remain intact but do not establish external AX absence.
Full runtime closure and installed confirmation remain separate obligations.

Main subsequently established that an external native AX client can expose real
SwiftUI Text/Button elements and execute the actual button once while the same
host's in-process reader still reports AXUnknown with zero children. The local
reader is therefore not an accepted oracle for these native controls. Preserve
the existing fixture assertions; do not count their failures as proof of a
production accessibility regression or replace the native text/actions.

The real-source external pair built both original and corrected variants with
identical full recipe data and wrappers. Main observed original rendering with
an empty recipe scroll tree, while239 exposed the original title/facts, both
handoffs and initial components. Separate external actions produced exactly one
real Nav.push for Apollo and Ma'at without mounting destination tasks. This is
finite top-content/action evidence, not completion of the full accessibility
suite or installed confirmation.

Direct pre-navigation AXScrollToBottom on239 then caused SIGTRAP in the SwiftUI
LazyLayout/accessibility scroll path (owned PID76524, native981b04c772824e25abf92196c6815cc7;
retained crash RecipeAXHost-2026-10-24-222208.000.ips). This child changes only the
detail stack's runtime layout from LazyVStack back to VStack, retaining every
contained section, original field and handoff. At that checkpoint eager layout
plus containment was an unrun candidate. It must expose first/last components and complete
card fields across actual external scrolling without a crash, retain both real
handoffs, and pass release/short-wing/card controls. Prior failures, partial
external passes, crash reports and the source239 tuple remain separate and
immutable. No full AX-suite pass is claimed.

## External evidence when the in-process oracle is unavailable

All five original cases remain obligations. Skipped in-process checks are
reported separately from passes; a green test process with skips is not AX
completion. Use an independently working public external AX client, exact
source/data/binary/PID/window binding and actual action callbacks for:

| Original case | Required separately bound observation |
| --- | --- |
| Independent control | Public Text/Button readable; one actual button callback |
| Full Ma'at recipe | All29 component IDs for the current recipe, top/bottom/return scrolling, distinct Apollo/Ma'at pushes and Back |
| Release recipe | Original fields/components and real release-preflight handoff |
| Short wing | Original complete wing text/next action |
| Full component card | Original source/tests/inputs/outputs/writes/upgrade fields |

Main's finite eager-source external observations are retained at
`stacklab-36e9-standard-window-scroll-20261025/receipt.json` (SHA256
`04beec391151986362b5d6c6196eef1f7de573ade080dd50f49bcbb7a83f71e6`)
and `stacklab-36e9-external-release-wing-card-20261025/receipt.json` (SHA256
`f3f63ffee09cebaa527e6815695952f15a591110b73c249fd87d471fbbb38cca`)
under the corrective evidence root. These are Main-transcribed CUA observations
with separately retained raw host lifecycle/action evidence, not persisted raw
AX-tree or screenshot files and not an in-process oracle pass. Prior644/239
failures, the lazy scroll crash and earlier timeouts remain immutable.

This test-only oracle successor preserves production views and fixture recipe
data byte-for-byte from36e9. Main may compose those exact external receipts only
after checking source/data parity and their finite scope; changed production or
fixture inputs need newly bound evidence. Installed-product, automated external
driver and broader all-host acceptance are not inferred.
