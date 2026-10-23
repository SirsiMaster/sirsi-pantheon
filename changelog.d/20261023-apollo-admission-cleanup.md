## Apollo native admission cleanup

- Own successful native admission before rank binding and fenced-plan encoding, and release it exactly once on early errors, cancellation, normal completion, or panic with an independent fifteen-second cleanup context. Preserve both work and release errors.
- Reject shorter or longer admission rank lists before indexing. Read measured metrics before release and publish them only after work and release both succeed; panic cleanup never publishes success metrics.
- Add callback-only lifecycle regressions without tokenizer, model, backend, network, or GPU execution. Runtime proof is performed separately by the release owner.

Refs: `docs/evidence/APOLLO_NATIVE_PACKAGE_COMPOSITION_20261015.md`, `docs/ADR-004-MAAT-QA-GOVERNANCE.md`.
