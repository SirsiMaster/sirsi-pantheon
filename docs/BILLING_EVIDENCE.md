# Retained billing evidence

`sirsi maat billing-evidence [observation.json] --max-age 24h` emits JSON for an
operator-supplied aggregate. It never queries a provider, changes credentials,
enables export, runs a billing job or grants a workload. No input is UNKNOWN,
not zero or unlimited; available budget and credit balance remain null and
enforcement is always `none`.

The strict `sirsi-billing-observation-v1` schema names billing_account,
export_table, a three-letter uppercase currency, window_start/window_end,
export_as_of, collected_at, complete and integer amount_micros. Timestamps use
RFC3339; the complete query window must end by collection time, collection/export
timestamps cannot be in the future, and windows must be nonempty. Duplicate,
case-aliased, unknown, trailing or oversized JSON is rejected. Amounts remain
integer micros; zero is retained when explicitly supplied, and signed net costs
are not clamped because exported credits can exceed charges.

`complete` attests that the retained aggregate's query/pagination completed; it
does not prove every accrued charge is in a delayed provider export. A complete
amount is `reported`, or `delayed` when export_as_of exceeds max-age. Missing
amount/incomplete aggregate is `unknown`. Provenance is reported, not provider
authenticated; a recent timestamp is not proof of source integrity or a live cap.

No real-time remaining-budget, grant-expiry, credit-balance or enforcement claim
is supported. The existing billing-account describe path is distinct from
BigQuery export entitlement and row freshness. The Oct29 read-only metadata probe
in known project sirsi-nexus-live returned200 with complete dataset-list
pagination and no billing/cost/export-named dataset. Differently named datasets
or other projects were not ruled out; no rows or billing queries were run.

Verification covers missing/partial observations, retained zero, delayed negative
cost, invalid/future metadata, ambiguous/oversized JSON, and command no-input/
missing-file/invalid-age behavior. Recovery is a normal source revert; no data or
service rollback is needed for this pure read-only reporting command.
