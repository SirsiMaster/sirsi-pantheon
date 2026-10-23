# Native workspace navigation

Each sidebar workspace retains its own drill-down history while Pantheon is
open. Switching back, or selecting the same workspace again, preserves its
detail. Back returns one step; the workspace home action clears only the
active workspace's drill-downs.

Restarting restores the last selected workspace, not operations or confirmation
dialogs. Unknown stored selections recover to Overview. Inactive workspace
views are unmounted: navigation retention does not keep hidden polling tasks
running. Root-local drafts, scroll position and transient query results are
not guaranteed across workspace switches or process restart.

Provenance: the independent navigation idea and initial three tests were
reconciled from the preserved worker patch
`native-navigation-e546ff00-router-custody-20261023.patch`, not from an available
Git object. The current implementation retains the new desktop design and its
current persistence key, reads the legacy key as a fallback, forwards stack
changes to both header and detail, and keeps per-frame identity on drill-downs.
