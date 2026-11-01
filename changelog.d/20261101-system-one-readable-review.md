### Fixed
- Open the audited memory-relief preview before asking to run a cache flush; other repairs retain explicit consent.
- Show System One case and repair approvals in full, scrollable native review sheets with exact command/project or case/evidence and persistent Cancel/action controls, instead of relying on macOS confirmation dialogs that omit review details.
- Explain changed repair evidence instead of silently ignoring an approval click.

- Refresh changing diagnostic operands when opening resolution instead of leaving a stale disabled review button.
- Show read-only case-history progress and a cancel action while retaining the exact history/case safeguards.

- Close native command-result sheets without changing navigation behind them, and show a recovery explanation when a reviewed action becomes stale.

- Revalidate approval when the review closes and show a return-to-finding route if it expires before the result opens; never present an empty repair sheet.

Refs: docs/ADR-033-REMEDIATION-CATALOG.md, docs/ADR-070-MAAT-STRUCTURED-VERDICT.md, PRODUCT.md
