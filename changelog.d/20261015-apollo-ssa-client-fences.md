## Unreleased — Apollo client source safeguards

- Refuse cluster CLI operations while enrolled authentication and verified plan integration are absent.
- Compare admission rank/host sets with caller-supplied authenticated frozen plan inventory.
- Bind streamed events to the original execution tuple and preserve typed token text and zero-valued token IDs/positions.
- Native authenticated client, authoritative text preparation, signing and installed qualification remain unverified.

Refs: PANTHEON_RULES.md, docs/SAFETY_DESIGN.md, docs/ADR-055-SNE-LICENSING-ANUBIS-RA-SEAM.md, docs/architecture/APOLLO_CLUSTER_CLIENT_V1.md

- Add developer and user guidance for disabled cluster commands and source-only safeguards, plus a CLI refusal regression test.

- Preserve the service cancellation wire enum explicitly in lint checks.

- Require frozen admission receipts for status and cancellation; reject changed execution/request/plan/epoch on those lifecycle responses too.

- Permit historical expired receipts for recovery while retaining fresh-admission expiry checks.
- Require terminal stream evidence; return typed incomplete outcomes with cursor for early EOF, transport breaks or oversized frames; remove cumulative live-stream truncation.
- Document the unresolved server-side stale-target cancellation precondition; correct independent endpoint-negative fixtures and lifecycle receipt examples.

- Reconcile actual 950c778d service preparation with disabled auth, frozen lifecycle receipts and typed incomplete-stream recovery; process JSON chat outcome independently of rendering.
- Run the complete Apollo package suite and replace socket-bound fixture servers with injected in-process HTTP fixtures. Host placement remains service-side enforcement, not client verification of an absent wire inventory.
