### Fixed

- Add a finite own-lane task evidence relocation operation for already-preserved
  local artifacts, without generic task-update widening or task lifecycle,
  timestamp, accounting, history, scope or lease changes. Exact public-row CAS,
  transactional provenance and exact-manifest/after-state replay refuse drift.
- Default CLI planning checks a nofollow preservation map and replacement hashes
  offline; explicit inspection/application uses the existing configured canonical
  service or relay, including localhost, never an alternate local database.
  Review references and artifact hashes are authenticated actor attestations,
  not server-certified review/filesystem proof. Source fixtures and recipe added;
  execution, publication and any reviewed live repair remain separately admitted.
- Preserve relocated references across ordinary legacy task updates: capture
  raw links with the decoded row, CAS both writers, and boundedly reload/remerge
  reference-only conflicts server-side without new request fields or duplicate
  accounting. Retain status/lease refusal fences, including concurrent renewal.
  Relocation refuses all retained ownership, including expired custody, without
  clearing it: a prior-live delayed completion can otherwise still land. Canonical
  server-writer deployment/compatibility must precede future application; ordinary
  older clients are not gated or required to adopt new flags/roles.
