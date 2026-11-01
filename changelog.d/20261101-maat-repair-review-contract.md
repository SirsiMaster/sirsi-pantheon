### Fixed
- Keep the native repair source contract aligned with the full Action Review sheet, exact reviewed argv, cancellation/confirmation controls, and execution reauthorization; exercise those invariants with closed source fixtures.
- Require the reviewed scope and detail to be rendered in the scrollable sheet, and bind Casebook action reviews to their frozen command, explanation, and evidence scope.
- Pin title, confirmation detail, and case/evidence/conclusion scope at MaatCaseDetailView's actual review-sheet call site; keep ResultView authorization labeled separately.

Refs: scripts/verify-maat-native-resolution-contract.sh, scripts/verify-maat-native-resolution-contract.test.sh, docs/ADR-070-MAAT-STRUCTURED-VERDICT.md
