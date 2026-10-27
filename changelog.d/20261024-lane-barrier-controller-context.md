### Corrected

- Pass the bounded lane-maintenance controller context through its durable row lock and every SQL statement. Preserve existing ordinary claim semantics and durable owner/capability; cancellation never releases the maintenance pause. Add an isolated PostgreSQL held-lock fixture that requires deadline return before unlock, joins the caller on all exits and verifies unchanged custody and paused claims. Source only pending admitted qualification; no live barrier or rollout claim.
- Close each joined pause/claim race iteration's fixture store before the next schema reset, retaining parent cleanup as an idempotent failure fallback instead of accumulating sixteen PostgreSQL pools. The fixture server connection cap and production pool/barrier behavior are unchanged.
