# Design — Pantheon PT

Source authority flows from the Go engine to product surfaces, then to deterministic package and cask renderers. Release evidence is a composed DAG: source -> tests/build -> explicitly labelled development package or commercial signed package -> retained release inputs -> canonical cask bytes -> signing/notary -> remote release -> installed lifecycle. Development artifacts use `-dev` names and never satisfy a release node. Commercial filenames are available only after the release route requires Developer ID, notarization, stapling, and validation. A missing node leaves the release state open.

## Interaction and component contract

Black is the primary surface, emerald restrained operational emphasis, and gold
Sirsi brand/navigation emphasis. Use the actual bundled logo, native controls,
readable type and clear hierarchy. Raw diagnostics and decorative metrics cannot
replace an explanation and supported next action.

Each workflow covers entry, loading, empty, fresh/stale/unknown result, review,
bounded execution, cancellation, postcheck and retained recovery. Selection and
operation ownership survive navigation without double submission. Failed refresh
must not relabel old data fresh. An intentionally disabled historical service
must not be enabled simply because its status is visible.

Go owns business logic/effects. Swift, CLI, TUI, MCP and retained web surfaces
project the same typed inputs and receipts. A bash command closing a window is
not an app workflow. Horus owns the local instance; Ra the fabric; Ma'at local
assessment and resolution; Apollo and Mercury their qualified producer scope.

The component catalog defines source/dependency/replacement boundaries;
`DIAGRAM_INDEX.md` maps logical, data, lifecycle, sequence and failure views.
Source diagrams describe code, not a new runtime observation or performance
measurement. Recipes are inspectable composition contracts, not arbitrary code
execution permission. Certification requires independent evidence for the final
composition, as recorded in `CERTIFICATION.md`.

## Native readability and command menus — corrective cohort

The native workspace uses a 15 pt body, 14 pt secondary copy and a 12 pt minimum
for supporting text. Text size is a presentation preference: 100–150 percent in
10 percent steps, persisted locally, independent of window width. Changing it
must retain the selected project, workspace navigation, Apollo draft and active
operation state. At maximum size, content must wrap or scroll rather than hide
action labels or require shrinking text.

Diagnostic repair actions use a content-sized gold surface with dark text. Both
the action title and supporting command wrap at narrow widths and 150 percent
text size; a native single-line bezel must not crop this two-line content.
The control retains its Button semantics, pressed/disabled states and existing
evidence-bound confirmation before execution.

The application provides real macOS Pantheon, File, Edit, View, Window and Help
menus. File selects a Git project through the existing project workflow and
closes the existing window. Edit commands use the native first responder.
View exposes text sizing, workspace search and navigation. Window commands
operate the one retained workspace; Help opens the existing tools library.
Unavailable commands are disabled with native menu validation. There are no
placeholder document commands or separate menu-only workflow. Source tests,
compilation and installed keyboard/menu/maximum-size inspection are separate
proofs; this design does not claim that the installed 0.24.141 app has the fix.
