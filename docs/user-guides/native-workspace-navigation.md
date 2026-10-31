# Native workspace navigation

Each sidebar workspace retains its own drill-down history while Pantheon is
open. Switching back, or selecting the same workspace again, preserves its
detail. View → Back (Command-[) returns one step; View → Forward (Command-])
returns to the detail you left. Opening a new detail after Back clears that
workspace's forward branch. A workspace home action clears both history
directions for that workspace. The application Home command opens Overview
and clears Overview's history; other workspaces keep their independent paths.

Restarting restores the last selected workspace, not operations or confirmation
dialogs. Unknown stored selections recover to Overview. Inactive workspace
views are unmounted: navigation retention does not keep hidden polling tasks
running. Ma'at's root System One primary pane retains its vertical scroll
position when switching to another workspace and back during the same window
session. Its recovery pane and navigation details keep separate native scroll
ownership. Only native offset callbacks received while Ma'at is still selected
at its root can update the retained position; a late callback from an unmounted
workspace is ignored. The offset is not persisted across app restarts. Root-local
drafts and transient query results are not guaranteed across workspace switches
or process restart.

Provenance: the independent navigation idea and initial three tests were
reconciled from the preserved worker patch
`native-navigation-e546ff00-router-custody-20261023.patch`, not from an available
Git object. The current implementation retains the new desktop design and its
current persistence key, reads the legacy key as a fallback, forwards stack
changes to both header and detail, and keeps per-frame identity on drill-downs.

## Native version inspection

Pantheon’s native executable supports `version --json` for the local sibling-health inventory. It reports its own stamped app version and executable path without opening another window or menu-bar instance. A missing or malformed native bundle stamp returns an error; it never borrows the adjacent CLI’s version.
