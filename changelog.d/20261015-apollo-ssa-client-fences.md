## Unreleased — Apollo client platform groundwork

- Reconcile service950c778d text preparation with disabled-by-default enrolled authentication and endpoint binding.
- Bind status/cancel/replay to original execution receipts; permit historical expired receipts for recovery while requiring live fresh admissions.
- Preserve authorized typed token content and raw events; require final release/orphan evidence and return incomplete-stream cursors on interruption.
- Validate prepared rank count, contiguous ranks and unique-host live fences; exact host/budget matching remains conditional service-side enforcement.
- Process JSON and human chat completion independently of rendering; cover both success and failure with shared stream fixtures.
- Add complete Apollo package race checks and in-process HTTP fixtures; document unresolved stale-target cancellation, authentication, private persistence, CI and installed qualification gates.

Refs: PANTHEON_RULES.md, docs/SAFETY_DESIGN.md, docs/ADR-055-SNE-LICENSING-ANUBIS-RA-SEAM.md, docs/architecture/APOLLO_CLUSTER_CLIENT_V1.md
