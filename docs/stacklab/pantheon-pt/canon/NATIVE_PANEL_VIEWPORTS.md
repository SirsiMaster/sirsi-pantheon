# Native feature-panel viewports

This corrective composition follows DESIGN.md and ADR-015. It changes native
presentation and one mislabeled navigation destination. It does not change
command arguments, operation approval, evidence validation or execution owners.

## Composition

| Surface | Viewport and control contract | Source |
| --- | --- | --- |
| Ma’at workspace | Local memory and diagnostic recovery share a scroll region capped at 220 points; the selected section retains its own viewport and state. Existing receipt scrolling remains independently bounded. | `MaatWorkspaceRecoveryFrame.swift` |
| Ma’at System One unavailable state | Error explanation, retained-evidence retry, decisions, import and maintenance routes belong to one scrolling document. | `MaatCasebookView.swift` |
| Full Disk Access guidance | Back stays outside the scrolling guide; all steps and the existing return-to-Trash control remain reachable. Opening the guide never grants access. | `Views.swift`, `FDAGuideView` |
| Osiris unavailable state | Long raw failure and project-selection guidance scroll above the existing footer. | `Views.swift`, `OsirisView` |
| Apollo recovery | Long capacity failures scroll. Telemetry recovery controls use natural-height buttons and fall back from a horizontal row to a vertical arrangement when their intrinsic widths do not fit. | `ApolloRunPlannerView.swift` |
| Stack Lab catalog and Doctor | Long failure explanations scroll. Recovery controls use natural-height buttons; the catalog’s Open Stack Lab Doctor control opens `StackLabDoctorView`, preserving the shared project and navigation history. | `StackLabCatalogView.swift`, `StackLabView.swift` |

The three newly styled navigation controls apply the style to their actual
Button and call the existing Nav owner. Shared NavLink behavior is unchanged.
ResultActionButtonStyle remains the replaceable presentation dependency; the Go
engine and the existing operation models retain business and side-effect authority.

## Qualification and remaining checks

Use live AppKit hosting with `snapshotMode=false` for viewport tests. Snapshot
rendering intentionally bypasses MaybeScroll and cannot prove scroll reachability.
At 100 and 150 percent text size, narrow and minimum-window layouts must retain
complete labels, a usable selected-section viewport, and a path to the final
control. Rendering and scrolling must not invoke a repair, inference, permission
change, evidence submission or destructive operation.

The installed 0.24.145 baseline was inspected through native accessibility and
screenshots. Overview, Activity, Apollo, Mercury, Ra, Ma’at, Stack Lab, Release,
Horus, Anubis, Osiris, Work, Fleet and All Tools opened; additional tool panels
included Thoth, Net, Vault, RTK, Insight and Ask Sirsi. This is opening coverage,
not a claim that every conditional state or action passed. Net reproduced the
shared clipped result-action text addressed by PR #1272. The Ra handoff sheet
was opened and cancelled without submission. Cleanup review and leftover-app
lists were opened without deleting or moving files. Ask Sirsi was readable at
150 percent; the original text-size preference was restored.

Ma’at Proposals displayed a retained-provenance validation error, and Fleet
initially displayed an unavailable/reading refresh state. Those are distinct
functional observations, not passing layout tests or grounds to weaken evidence
validation. Keep them on the task ledger until causally resolved.

Source review, native tests, protected CI, signed publication and installed-app
verification remain distinct evidence nodes. Post-update loading, final-control
visibility and nested keyboard/wheel traversal require actual installed readback.
