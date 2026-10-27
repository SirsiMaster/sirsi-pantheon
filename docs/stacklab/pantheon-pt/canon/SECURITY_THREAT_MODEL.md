# Security and threat model — Pantheon PT

Threats include sibling-engine drift, PATH substitution, release-input replacement, malformed cask records, credential leakage, unsigned artifacts presented as release-ready, and destructive lifecycle actions. Controls are canonical-engine identity, descriptor/byte-bound release inputs, no-follow/create-only publication, exact hash readback, explicit development/commercial modes with distinct names, mandatory Developer ID plus notarization/stapling for commercial artifacts, rollback receipts, and fail-closed lifecycle validation.

| Boundary / threat | Controls and required refusal evidence |
|---|---|
| Surface → engine substitution or stale result | Exact sibling/self identity, typed commands, bounded output, query/generation/freshness; changed engine, cancel and late-result refusals |
| File or release input replaced during read | Retained no-follow parent/leaf, exact bytes/hash, terminal metadata/name/namespace continuity; leaf/parent/substitution-back negatives; never execute payload to inventory it |
| Cleanup/updater/privileged maintenance affects unrelated work | Scoped preview, fresh effect consent, retained attempt/journal, exact selected identity; interruption, replacement, rollback and unknown-effect reconciliation; no automatic replay |
| Router or worker completes stale/foreign/claimed work | One canonical store, authenticated session/lease/CAS and joined duties; stale/foreign/duplicate refusal; original caller proof |
| Local intelligence requires an unannounced model or exports content | Deterministic Ma'at baseline and explicit optional qualified model route; offline/model-unavailable proof; no default remote fallback |
| Recipe or producer manufactures authority | Exact schema/source/evidence binding and independent promotion; malformed/unknown/stale selection refusal with supported recovery |
| False certification, stale registry pin or falsely current mirror | Exact origin/pin/roster readback, scoped independent review, stable mirror identifiers and sync state; unknown is not clean |
| Signing-secret exposure | Established delegated Team9D382WV988 route and public receipt checks; never print/export/import keys or infer missing credentials from historical prose |

This table defines required controls, not proof they have all been exercised.
Source, tests, independent review, installed behavior and final artifact checks
retain their own scope in `CERTIFICATION.md`. No canon/source review grants
service, security, transport or model mutation authority.
