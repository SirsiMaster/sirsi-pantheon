<!-- agent: ra | workstream: router-service (ADR-062) | ledger: rs-35-a2a-contract-assessment | authority: this file is the authority for the A2A property-by-property assessment; router items are correspondence about it, not the record. -->

# Router A2A Contract Assessment

**Status:** Assessment complete, sent to sirsi-hardware-admin (SHA); awaiting response before any implementation.
**Trigger:** SHA item `20260913-043215` ("Repair router body-loss regression; formalize minimal A2A contract"), second half — after the body-loss regression itself was fixed in `rs-34` / PR #754.
**Classification:** Architecture assessment under Stack Lab methodology (see `ROUTER_STACK_LAB_RECIPE.md` for the rubric this file follows: distinct evidence states, identified components, no sprawl).
**Instruction being honored:** SHA's own item text — *"Do not add ceremony that duplicates a verified queue guarantee. If the existing mechanism can satisfy the contract, harden it; if not, replace the failing transport boundary."* This assessment exists to answer that question with evidence per property, not to propose a protocol on priors.

## Method

SHA named six properties a formal A2A (agent-to-agent) contract needs: durable sender/recipient identity, message body integrity, idempotency, delivery/read acknowledgement, correlation IDs, and error semantics. Each is checked against the actual code — file, line, and the mechanism it implements — and labelled by evidence state per the Stack Lab rubric: **verified** (read the code, mechanism confirmed present), **verified — negative control** (a test proves it fails without the mechanism, not just that it passes with it), **gap** (no mechanism found).

## Property-by-property

### 1. Durable sender/recipient identity — verified

`internal/dispatch/facade.go:195`, `Facade.ValidateAgent(party, id string) error`. Looks up the caller's `id` in `agents.json` (ADR-054 declared identity), refuses an empty id, refuses the legacy `"user"` alias by name, and refuses an id not present in the registry (`undeclaredAgentError`). Both `From` and `To` on every `SendReq` are subject to this check before a send is accepted — identity is a registry lookup, not a caller-supplied claim.

### 2. Idempotency — verified

`internal/routerstore/facade.go:38-47`, `(s *SQLiteStore) idemKey(r SendReq, now time.Time) string`: the key is `(from, to, type, subject_key, source_item, time_bucket)`, hour-bucketed. `sendGuardedOnce` (`facade.go:87-160`) looks up this key inside an immediate transaction (`facade.go:105`) before ever inserting; a match bumps `occurrences` and returns the **existing** item id with `deduped=true` rather than creating a duplicate. The key is enforced by a database constraint, not just application logic: `internal/routerstore/store.go:279`, `CREATE UNIQUE INDEX idx_items_idem ON items(idem_key) WHERE idem_key <> '';` — a resend can only ever hit the existing row.

### 3. Message body integrity — verified (as of this assessment)

This was the actual gap SHA reported and rs-34 (PR #754) closed: `cmd/sirsi/routercmd.go`'s `loadOrLiteral` now refuses an empty/whitespace-only body from either the literal or `@file` path, gated by a `Flags().Changed()` check (`loadOrLiteralIfSet`) so a legitimately *omitted* flag (an idempotent no-op close) is not confused with a flag that was passed and silently corrupted by shell substitution. See rs-34 for the full repro/fix/verification chain. Listed here because it is one of the six named properties, not because it is still open.

### 4. Correlation IDs — verified

`internal/routerstore/facade.go:35`, `SendReq.SourceItem string`: "ties emissions to the item that caused them" (the field's own doc comment). Persisted on insert (`facade.go:159-160`, the `source_item` column) and readable back per item. This is a real, structured correlation field — not just a human writing "RESPONSE to X" into the prose body (which the `respond` command *also* does for readability, at `routercmd.go` around the `RESPONSE to your request` format string — but the structured `SourceItem` field is what a machine can actually join on).

### 5. Error semantics — verified

Typed, distinguishable failure classes, not just `error` strings: `ErrOverQuota` (`facade.go:29`) for backpressure — a healthy sender that hit its rate limit, resolved via a singleton throttle item, never trips the sender's own circuit breaker (see the comment at `facade.go:139-146` explaining why quota and breaker failures are deliberately kept separate — a lesson from a prior incident where over-quota drops were miscounted as delivery faults). `ErrBreakerOpen` is the separate class for genuine delivery faults (dead-letters), each carrying "a retrievable cause receipt" per rs-28's own record. A caller can branch on `errors.Is` against a specific class rather than parsing message text.

### 6. Delivery/read acknowledgement — gap

This is the one property with no existing mechanism. `Item` (`internal/work/work.go:21-35`) carries `WakeStatus`/`WakeAttemptedAt`/`WakeAdapter`/`WakeError` — but these prove an attempt was made to **wake the recipient's process**, not that the recipient's own code path actually **read** the item content before acting on it (or closing it). `Status: open/closed` is the only recipient-side signal, and it's coarse: closing conflates "I read this and did the work" with "I read this and am acknowledging it" with, in principle, "I closed this without reading the body" (nothing currently prevents that last case, though nothing incentivizes it either).

## Conclusion

Five of six named properties are already implemented, verified against the actual mechanism (not inferred from behavior), and none of them need replacing — SHA's own instruction against ceremony that duplicates a verified guarantee applies directly: idempotency, identity, correlation, and error semantics are all real database- or registry-backed invariants already, not conventions an agent could accidentally bypass.

The sixth (delivery/read acknowledgement) is a genuine, narrow gap. Recommendation: **harden, do not replace.** Add an explicit `read_at` (or `acked_at`) timestamp to `Item`, set by a new, small verb (e.g. `sirsi router ack <id>`) that a recipient calls after reading an item's body and before doing the work — distinct from `close`, which still marks completion. This is additive to the existing schema (one column, one verb) and requires no new transport: the spool/store path SHA asked to assess already carries everything else needed. No new protocol, no new wire format, no ceremony beyond the one field the assessment actually found missing.

This recommendation has **not** been implemented. Awaiting SHA's response (sent as router item, see Change Log) before writing any code — this is an architecture decision, not a bugfix, and per PANTHEON_RULES.md §2.23 (A26) large workstream decisions route through this correspondence before implementation.

## Change Log

- 2026-09-13 — assessment written and sent to sirsi-hardware-admin. Ledger: `rs-35-a2a-contract-assessment` (blocked-by `rs-34-router-body-loss-empty-guard`, which this assessment's property 3 depends on).
