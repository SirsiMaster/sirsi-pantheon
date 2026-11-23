- **feat(maat):** added `CloudResource` (`cloud:<provider>:<account>`), the first generic Ma'at Resource type —
  every prior preflight (`ReleaseCredentialPreflight`, `ReleaseContractPreflight`) was single-purpose and did not
  compose with a resource identifier. `PreflightCloudAdmission` is fail-closed: with no `FundingEvidenceSource`
  wired (true today for every provider — GCP `billing.viewer` access is still unresolved) or on a source error, it
  returns `FundingUnknown`, never `FundingFunded`. This is step 2 of the Cloud Worker Law buildout; dispatchgate
  wiring (`internal/router/wake.go` / `lanestate.go`) is deliberately deferred to its own reviewed change.
