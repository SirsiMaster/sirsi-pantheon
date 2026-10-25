# Production runbook — Ma'at

Before a change, verify clean source identity and the relevant recipe component. Run focused tests, then the repository gates. Inspect generated journal and Casebook output for evidence links and honest failure states. Publish receipts only after readback. On failure, preserve the record and stop; never rewrite prior decisions or delete another lane's state.

## Failure-memory corrective upgrade

Keep immutable incident/evidence/write-journal bytes. The supported local guard
is `maat.active-incident/v1`: exact matching active evidence rejects recurrence,
not host health. Old arbitrary guard labels remain unsupported, not migrated.
An unsupported matching guard needs a reviewed incident disposition through the
existing predecessor-bound transition path; never rewrite its introduction or
invent a successful probe. Unrelated scopes and terminal incidents do not dispatch
that guard. A source digest is not an authenticated issuer or an operation grant.

Qualify the core and CLI with owned temporary stores/journals only. Include actual
typed dispatch, unsupported id/version, mixed/scoped and terminal cases, FIFO/size
refusal, failed-write/reopen debt, independent concurrent writers and journal
append failure. Preserve normal/race and causal-negative receipts. Historical
`34252dca` execution custody stays missing unless its original bytes are recovered;
new current-source evidence must not be backdated. All eight authoritative
operation integrations and installed execution remain separate obligations.

The managed liveness-watch source candidate uses only the fixed per-user default
store and the shared rootless local engine/HOME installer and inspector. First-use
initialization and incomplete-attempt re-observation are separate confirmed
controls in native Casebook; damaged or interrupted initialization is inspect-only
and must never be reset or adopted. This integration remains source-only until its
focused Go, strict native build/tests, review, and installed evidence are
separately recorded.
