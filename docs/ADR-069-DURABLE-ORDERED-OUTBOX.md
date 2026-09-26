# ADR-069 — Durable, ordered store-and-forward outbox (offline messages are held and released, never dropped)

- **Status:** Proposed — owner-directed 2026-09-26 ("messages that can't reach the cloud are still stored for future release, in order"). Delivery-semantics change → SSA review before merge.
- **Date:** 2026-09-26
- **Steward:** `ra`
- **Refs:** ADR-062 (router service + spool relay), the 2026-09-26 M5 split-brain (`project_fabric_split_brain_m5_local_db`), PANTHEON_RULES A35 (scope the check), A23 (measure thrice).

## 1. Context

The fabric split-brained: M5 lanes wrote to a **local full ledger** (`~/.sirsi/router.db`, a dead-end parallel store) instead of the cloud service, stranding 44 messages to `ra`. Root of the *design* gap the owner named: when the cloud is unreachable there was **no correct place for a message to wait**. Two wrong answers existed — a parallel local ledger (split-brain) or a hard error (message lost) — and no right one.

The spool relay is the store-and-forward layer, and it already gets **ordering** right (each lane's `req/` queue drains in file/id order). But its failure handling is wrong for an outage: `handleOne` renames `req→inflight`, calls `forward`, then **`os.Remove(inflight)` unconditionally** (spool.go:680). On a failed forward it publishes `OUTCOME UNKNOWN` and the message is **consumed and dropped** — it does not survive the outage. So the relay queues in order but does not *hold through* an outage.

## 2. Decision

Make the spool relay a **durable, ordered outbox**: a message that provably never reached the cloud is **held and re-forwarded in order when the cloud returns**, never dropped.

### 2.1 Failure classification (the safety hinge)

In `forward`, distinguish two failures — this is the whole correctness argument:

- **Never reached the service** — `rl.Client.Do` returned a **connection-establishment error** (dial refused, no route, DNS failure, TLS-during-dial, dial timeout). The request was **provably not sent**, so it is **not committed** and is **safe to retry for ANY method**. Classifier `neverReachedService(err)`: `errors.As` a `*net.OpError` with `Op=="dial"`, a `*net.DNSError`, or `errors.Is(err, syscall.ECONNREFUSED)`. This is exactly the owner's "can't reach the cloud".
- **Reached the service, outcome uncertain** — request sent, response lost/timed out mid-flight. The service **may have committed**. This stays **`OUTCOME UNKNOWN`** (unchanged) — auto-retrying a maybe-committed *non-idempotent* mutation would double-act. (Idempotent sends could safely retry via `idem_key`, but we do NOT rely on that here — the never-sent gate is sufficient and method-agnostic.)

### 2.2 Race-free ownership (the mechanism)

A new **relay-owned** directory `<spool>/<agent>/outbox/` holds retryable messages. The client **never touches** it, so there is no client/relay file-ownership race (the bug that makes a naive requeue-into-`req/` dangerous — the client may clean up `req/` concurrently).

`handleOne` becomes:
```
req/<id>.json  --rename-->  inflight/<id>.json   (relay owns it now)
sr, retryable := forward(...)
if retryable:
    rename inflight/<id>.json -> outbox/<id>.json   (held, relay-owned)
    publish QUEUED response (see 2.3)                (client learns it is durably accepted)
else:
    publish(sr); remove inflight/<id>.json           (today's behavior)
```
The lane worker drains **`req/` then retries `outbox/`**, both in id (time) order, so global order is preserved (a held earlier message is re-forwarded before newer ones). On a successful retry the `outbox/` file is removed; `idem_key` dedup at the service makes a retry that races a prior partial commit a safe no-op.

### 2.3 Client contract during an outage

On a held message the relay publishes a distinct **`QUEUED_FOR_RETRY`** response (not an error, not `OUTCOME UNKNOWN`): "durably accepted; will deliver when the cloud is reachable." The client returns *queued* to its caller instead of blocking on a cloud round-trip or reporting loss. The final service id is not returned during the outage (there is none yet); `idem_key` guarantees at-most-once delivery when it drains.

### 2.4 Backoff

A wake-driven lane worker must not hot-loop while the cloud is down. When a drain leaves anything in `outbox/`, schedule a delayed re-wake (`time.AfterFunc`, exponential 5s→…→cap 5m per lane), reset on any success. `Serve`/`serveOnce` unchanged except for consulting `outbox/`.

## 3. What this does NOT change

- **No local full ledger.** The dead-end `~/.sirsi/router.db` fallback stays forbidden (M1 000-dir, M5 chmod-000, and the code keystone that refuses `SIRSI_ROUTER_DB=~/.sirsi/router.db` on a cut-over host). The outbox is a *forwarding queue that always targets the one cloud ledger*, not a second ledger.
- **`recoverInflight`** (startup, a crashed relay's in-flight): stays `OUTCOME UNKNOWN` — those were mid-send (maybe committed), not never-sent.
- **Non-idempotent maybe-committed** post-send failures: stay `OUTCOME UNKNOWN`.

## 4. Verification contract (must ship with the code)

1. **Held-and-released, in order (the load-bearing test):** a lane sends A, B, C; the service is unreachable (injected dial error). Assert all three land in `outbox/`, none dropped, client got `QUEUED_FOR_RETRY`. Then the service comes up; assert A, B, C are forwarded **in order** and `outbox/` drains empty.
2. **Never-sent classifier, both directions:** a dial-refused error → retryable (held); a post-send read timeout → NOT retryable (`OUTCOME UNKNOWN`, not held). The negative control (post-send stays unknown) is as important as the positive.
3. **No double-delivery:** a message held then delivered, where a prior attempt had already committed at the service, appears **once** (idem_key), `occurrences` bumped.
4. **No hot-loop:** cloud down for N wakes → bounded forward attempts (backoff honored), not one-per-tick.
5. `go test -race -short` under a user-owned TMPDIR (the #761 lesson).

## 5. Neith's Triad (A22)

**Data flow:** `lane → req/ (client) → inflight/ (relay) → [reached? publish+remove | never-sent? outbox/ + QUEUED] → outbox retry (backoff, in order) → service (idem_key dedup) → one cloud ledger`.

**Build order:** P1 `neverReachedService` classifier + test → P2 `outbox/` + `handleOne` requeue + `QUEUED_FOR_RETRY` → P3 lane-worker drains outbox with backoff → P4 the in-order held-and-released integration test. Minimum viable = P1–P4 (all required; this is a delivery guarantee).

**Key decisions:**

| Question | Options | Recommendation |
|---|---|---|
| What is safe to retry? | (a) any failure, (b) never-sent only, (c) idempotent methods only | **(b)** — provably-not-committed, method-agnostic; avoids the double-send trap of (a) |
| Where do held messages live? | (a) back into `req/`, (b) relay-owned `outbox/` | **(b)** — (a) races the client's own `req/` cleanup |
| Client contract during outage | (a) block, (b) error/unknown, (c) `QUEUED_FOR_RETRY` | **(c)** — honest "durably accepted", non-blocking, not loss |

## 6. Rejected alternatives

- **Requeue on *any* forward error:** double-sends maybe-committed non-idempotent mutations (claim/complete).
- **Requeue into `req/`:** client/relay file-ownership race.
- **A local ledger as the offline store:** the split-brain this whole effort exists to end.
