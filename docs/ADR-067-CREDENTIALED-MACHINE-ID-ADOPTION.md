# ADR-067 — Credentialed machine-id adoption for host identity (rs-42/rs-43)

- **Status:** Proposed — **security-boundary change, gated on SSA review before merge** (most-legs rule: security stays an owner/SSA gate).
- **Date:** 2026-09-25
- **Steward:** `ra` (router-service lane, thread `thr-df2a8cd5b1c61290`)
- **Reviewer (required before merge):** `sirsi-software-admin` (SSA)
- **Supersedes design intent of:** the *rejected* runtime shape bridge (see `docs/router-service/IDENTITY_ARCHITECTURE.md` §4; `internal/machineid.InTransition` doc; `internal/routerstore/serve.go` `threadAuthority` NOTE 2026-09-15).
- **Refs:** ADR-062 (router service, host tokens rs-11), ADR-065; ledger rs-42, rs-43; PANTHEON_RULES A34 (bind ≠ verdict), A35 (scope the check to the claim).

## 1. Context — the bug, stated once

Router identity has four layers (`IDENTITY_ARCHITECTURE.md` §1). Two of them — session mint and
`threadAuthority` — key on `os.Hostname()` (via `sess.Host`), which macOS lets DHCP/mDNS rewrite at
any time with no notification. Layer 4 (`internal/machineid.MachineID()`, the hardware
`IOPlatformUUID`) is the only fixed point, and it was wired into the LOCAL reaping half
(`SameMachine`, PR #223) but never into the SERVICE-facing half.

Consequence, observed live and fabric-wide: the same two physical Macs answer to four different host
strings (`M1.local`, `MacBookPro`, `Mac`, empty). Thread records stamped under one string are
"foreign" to a session claiming another, so:

- reaped/orphaned threads cannot be re-registered under the drifted name (Ra's own
  `thr-df2a8cd5b1c61290`, bound to stale `MacBookPro`);
- `claude-finalwishes-helper` on the M5 cannot register at all;
- ~dozens of live threads carry no host (`?`) because they registered before host stamping.

## 2. The rejected non-solution (do not re-attempt)

A runtime **shape** heuristic: "if one identity string looks like a hostname and the other looks
like a machine-id UUID, treat the mismatch as the same host mid-migration and allow it."

It is unsafe and this repo already proves it: `TestThreadAuthorityIsHostScoped` constructs an
authenticated session on host A touching a record naming host B — the exact attack the check exists
to refuse. The bridge's only signal ("the two strings have different shapes") is *also* true of that
attack. There is no **string-content** signal that discriminates "an attacker" from "me, renamed."
Running the drafted bridge against the suite passes a cross-host rewrite that must fail. That result
is the whole argument.

## 3. Decision — identity is adopted as *data*, by an authenticated act, never inferred from shape

The one thing the remote service can actually verify is the **host token** it minted. `MintSession`
already refuses unless the presented token is *for* the claimed host (`ErrHostMismatch`), so every
authenticated session **provably controls its host's token**. That existing proof is the credential
the safe migration needs — no new secret, no new handshake.

We add one authenticated verb and one recorded fact:

### 3.1 `AdoptTokenMachineID(host, machineID)` — the credentialed verb

- Client surface: `sirsi thread adopt` (also reachable as `sirsi identity adopt`). The client supplies
  **only** `machineID` (its live `machineid.MachineID()`).
- Server injects `host = sess.Host` from the **authenticated** session, overwriting any client value —
  identical to the `DeleteThreadCAS` host-injection already in `threadAuthority`. The client cannot
  assert a host it did not authenticate as.
- Effect: records the adoption `machineID ⇄ host` in `host_tokens.machine_id` for the caller's
  non-revoked token. **Recorded once**; idempotent when re-adopting the same `(host, machineID)`.

### 3.2 The exclusivity invariant — one machine-id per live token

`host_tokens.machine_id` is **UNIQUE among non-revoked tokens** (partial unique index). Adopting a
`machineID` already held by a *different* non-revoked token is refused (`ErrMachineIDClaimed`).

This is the property SSA must weigh. Its consequence:

- **No impersonation.** To reach host V's records, a caller must resolve to V's identity. That only
  happens if V's own token-holder adopted the shared `machineID`. An attacker holding host A's token
  who claims V's `machineID` first merely *blocks* V's adoption (V's records never migrate to a
  machine-id the attacker squats) — the attacker's alias therefore reaches nothing of V's.
- **Worst case is a detectable DoS by an already-trusted fleet member**, recoverable by revoking the
  offending token (which frees the `machineID` via the `revoked=''` predicate). A `machineID` is
  client-claimed and unverifiable by a remote service; this invariant is what converts that
  unavoidable weakness from silent impersonation into a loud, reversible denial. **`ponytail:` accepted
  ceiling — a fleet-internal migration verb, not an anonymous-attack surface; SSA rules on the ceiling.**

### 3.3 Resolution — `threadAuthority` reads the recorded alias, not the string shape

`mismatch(recordHost)` changes from `recordHost != sess.Host` to: *false* when `recordHost` and
`sess.Host` resolve to the **same adopted `machineID`** (both non-revoked host tokens carry it), and
otherwise unchanged. Session mint gains the same resolution so a token whose hostname has drifted
still mints once it has adopted its machine-id.

`TestThreadAuthorityIsHostScoped` still passes unchanged: the attacker never recorded an alias tying
A to B, so resolution finds no shared machine-id and the mismatch stands. The check reads **data an
authenticated act created**, which is exactly what the shape heuristic could not do.

## 4. Scope-the-check audit (A35)

| The claim | What it actually reads | Gap → closed by |
|---|---|---|
| "these two host strings are the same machine" | a recorded `machine_id` shared by two **non-revoked** tokens | not shape; not a single window — the negative control (§6) exercises the *attacker* path, not only the happy path |
| "only the real machine can merge its identities" | possession of a server-minted token + one-machine-id-per-live-token | the exclusivity index; residual (client-claimed id → DoS) named in §3.2, not hidden |

## 5. What this does NOT do

- It does **not** backfill the ~322 legacy records automatically. Records under a host string the
  caller's token was never minted for have **no proof of ownership** and are left as-is (mostly reaped
  tombstones — harmless). Reclaim is available only for records whose host the token authenticates.
- It does **not** flip `SIRSI_ROUTER_USE_MACHINE_ID` fleet-wide (that is IDENTITY_ARCHITECTURE §7 step 5,
  after one host runs through real network churn). This ADR builds step 2 only.
- It does **not** touch the scoped/read-only token gap (rs-44) — orthogonal.

## 6. Verification contract (must ship with the code)

1. **Negative control (the load-bearing test):** `TestThreadAuthorityIsHostScoped` passes **unchanged**.
   A new test asserts an attacker session (host A, its own valid token) is still refused a record on
   host B even after A adopts a machine-id — resolution must not merge A and B.
2. **Migration works:** a session on host H that adopted machine-id M may register/resume a thread
   whose record host is H; after a *second* token for the same physical box adopts the same M, either
   identity resolves to the same records.
3. **Exclusivity fails closed:** adopting an M already held by a different non-revoked token returns
   `ErrMachineIDClaimed`; revoking that token frees M.
4. **Race/CI honesty:** `go test -race -short` under a user-owned `TMPDIR` (the #761 lesson), both
   SQLite and the Postgres schema path.

## 7. Decision points (Neith's Triad — A22)

| Question | Options | Recommendation |
|---|---|---|
| Anchor of trust for migration | (a) hardware UUID shape, (b) server-minted host token, (c) new migration secret | **(b)** — the only thing the remote service verifies today; no new secret to leak |
| One machine-id → many tokens? | (a) many live tokens share an id, (b) at most one live token per id | **(b)** — (a) reintroduces the impersonation merge; (b) caps the worst case at reversible DoS |
| Backfill legacy 322 records? | (a) auto-map by hostname, (b) only what the token proves | **(b)** — (a) is the rejected shape bridge wearing a migration hat |

## 8. Rejected alternatives

- **Shape bridge (`InTransition` at the authority check):** §2. Fails the existing attack test.
- **Trust client-claimed `machineID` directly in `threadAuthority`:** same weakness as the shape
  bridge — a client can claim any id; without the token anchor there is no proof.
- **Auto-backfill by hostname:** §5 / §7 — indistinguishable from impersonation for any host string
  the caller cannot authenticate.
