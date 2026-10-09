# ADR-077: Ownerless Recovery — Signed LAN Anchor

## Status
**Proposed, revision 9** — 2026-10-09. Design only; no code, no key material,
no new `authorized_keys` entries. Routed for SHA (hardware) + SSA (software)
review before any implementation, per the owner directive that created this
task (SHA 20260915-012036, ledger `rs-41-ownerless-recovery-signed-lan-anchor`).
Revision 2 responded to SSA's and SHA's CHANGES_REQUESTED verdicts on
revision 1 (exact head `e22478e1`); revision 3 responded to SSA's
CHANGES_REQUESTED verdict on revision 2 (exact head `c5680351`); revision 4
responded to both SHA's and SSA's CHANGES_REQUESTED verdicts on revision 3
(exact head `1087bf15`); revision 5 responded to both SHA's and SSA's
CHANGES_REQUESTED verdicts on revision 4 (exact head `f2d03042`); revision 6
responded to SSA's CHANGES_REQUESTED verdict on revision 5 (exact head
`7449476b`). SSA independently reviewed revision 6 (exact head `5b4e7de3`)
and returned CHANGES_REQUESTED on four items — see "Review History" below.
(SHA's parallel revision-6 review returned BLOCKED on bundle delivery to its
worker host, not a verdict on the text; see the open cross-host-delivery
item — ra has no filesystem or network path onto SHA's host, so this
blocker is escalated to the owner rather than re-attempted with another
bundle cut from this side.) SSA's revision-6 findings: (1) **R6-1 P1** — the
synchronous stat added in revision 6 compares `mtime`+`size`, which is not
content identity (a same-length, restored-mtime replacement is
indistinguishable from "unchanged"), and the shared guard it runs under
covers the sync publisher and the Executor, not an account editing
`authorized_keys` or root editing `anchor-grants.allow` directly — a raw
edit was never defined as "committed" against a distinct controlled
commitment; (2) **R6-2 P1** — the seven-field envelope carries no epoch, so
"the Verifier rejects an envelope whose claimed admission epoch is older
than current" has nothing authenticated to check: a captured, never-before-
admitted envelope gets implicitly assigned the *current* epoch on first
verification rather than a distinguishable prior one, and minting an epoch
did not specify how the ratchet lets new post-override requests through
without re-opening the retired window; (3) **R6-3 P1** — the pruning
transaction advances the retirement floor to the deleted row's `expires_at`,
not to a wall-clock observation made at prune time, so a crash/restart at a
rolled-back clock value at or before that `expires_at` passes both the
ratchet check (floor not exceeded) and the replay check (row already gone)
— a concrete counterexample with TTL/skew/prune-time constants is given
inline below; (4) **R6-4 P2** — §1's active fallback prose
(not the test-coverage list or matrix, already fixed in revision 6) still
omits the unchanged-source qualifier, plus a stale "(this text)" self-label
on the revision-5 history entry. This revision: (a) replaces the mtime+size
stat with a **synchronous, in-guard invocation of the same content-hash
validate function the async sync step uses**, run against both
`anchor-grants.allow` and the signing fingerprint's `authorized_keys`
content on every pre-dispatch revalidation — making that synchronous
validate call, not a raw file write, the authoritative commit boundary for
both root-owned grants and account-writable restrictions; (b) adds an
eighth, signed envelope field, `epoch` (integer, must equal the daemon's
current protected epoch at verification time, checked again at pre-dispatch
under the same guard — never the caller's claimed admission epoch), and
defines an override as both minting the next epoch **and** re-anchoring the
wall-clock ratchet to the daemon's actual current wall clock in the same
guarded transaction, so epoch-bound fresh requests are admissible again
while every prior-epoch envelope stays permanently rejected regardless of
its own `expires_at`; (c) changes the pruning floor formula from
`max(current floor, row.expires_at)` to `max(current floor, the daemon's
trusted current wall-clock observation taken at prune time)`, persisted
atomically with the row deletion; and (d) adds the missing qualifier to
§1's active fallback prose and corrects the revision-5 history entry's
self-label. It remains proposed, not accepted, pending a fresh SHA+SSA pass
on this exact text.
SSA independently reviewed revision 7 (exact head `68dc688c`) and returned
CHANGES_REQUESTED on two items — see "Review History" below. SSA's
revision-7 findings: (1) **R7-1 P1** — the raw-edit commitment boundary
still asserted both that the synchronous validate call is authoritative and
that "no write-side process is left unfenced," which is false for a raw
writer that never acquires the guard the validate call runs under; (2)
**R7-2 P2** — the rollback override's new-epoch forward progress was
contradicted by the retained global admission-order high-water mark, which
can deny a fresh, correctly-epoched, post-override request whose `issued_at`
falls below a mark set before the override. This revision: (a) redefines the
raw-edit boundary so that `anchor-grants.allow` and `authorized_keys` hold
**proposed** policy at all times, and only the most recent synchronous
validate call's content-hash snapshot, taken and compared under the shared
guard, is **active** authority for dispatch — a raw edit landing after that
snapshot was read but before the guard releases is not yet a committed
revocation, it is input to the *next* validate call; drops the "no process
on the write side left unfenced" and "never act on a source state that has
already changed" claims as false of any writer that bypasses the guard;
and defines how an operator or account learns a proposed edit was actually
committed (closes SSA R7-1 P1); (b) scopes the admission-order high-water
mark into the protected-epoch domain introduced in revision 7, so an
owner-gated override both mints the next epoch and initializes that new
epoch's own high-water floor from the daemon's current wall-clock
observation, while the prior epoch's floor is retained historically and
never consulted for a current-epoch admission (closes SSA R7-2 P2). It
remains proposed, not accepted, pending a fresh SHA+SSA pass on this exact
text.
SSA independently reviewed revision 8 (exact head `9054b085`) and returned
CHANGES_REQUESTED on two items — see "Review History" below. SSA's
revision-8 findings: (1) **R8-1 P1** — the raw-edit section still asserted
an impossible mutual-exclusion invariant ("exactly two timings," "cannot
occur mid-read") over writers that never acquire the guard, restating
R7-1's defect rather than removing it; the commitment-acknowledgement
query also conflated "a validate call observed this hash" with "this hash
is active," which a denied or dirty validation would have falsely
satisfied; (2) **R8-2 P2** — the claim that the ratchet is "never reset
backward, only forward" contradicted the override transaction's own
re-anchor from a false-future reading to real time, and a test/matrix row
wrongly attributed an ordering-floor denial to the ratchet check (which
never reads an envelope's `issued_at`), plus one matrix row described an
impossible "dispatch with no subsequent validate call" schedule. This
revision: (a) removes the mutual-exclusion claim entirely — raw writers
may write at any time including mid-validate-read, and the guard
serializes only validation/publication against dispatch, never against
raw writes; redefines the acknowledgement query to report the active
published snapshot's hash/generation separately from dirty/invalid state,
so a mere validation observation never advances the reported active hash
(closes SSA R8-1 P1); (b) makes the ratchet epoch-scoped like the mark —
the retired epoch's ratchet is preserved unmutated while the new epoch's
ratchet is freshly initialized and may be numerically lower, correcting
the false universal-monotonicity claim; re-attributes the `issued_at`-based
test to the ordering-floor/mark check and adds a genuine, envelope-
independent rollback control against the ratchet; and replaces the
impossible matrix row with the actual schedule — a later dispatch always
performs its own mandatory validate call against the current source
(closes SSA R8-2 P2). It remains proposed, not accepted, pending a fresh
SHA+SSA pass on this exact text.
Number note: ADR-076 is claimed by an open, unmerged PR (#1017,
`maat/trust-boundary-gate`) and does not exist on `origin/main` (A37 — a
record exists only on origin). This document takes ADR-077 to avoid a
collision if/when #1017 merges first.

## Context
Owner direction: every Sirsi machine must be recoverable without an
interactive owner physically present. Today that recovery happens two ways,
neither of them a designed path: (a) a cloud agent revives a dead broker or
service over whatever port happens to be open (the exact inversion ADR-045/
ADR-046 were written to prevent for the local-LLM broker specifically, not
generalized), or (b) the owner drives recovery by hand. Both are ad hoc.

A sibling design, ADR-075 (Desktop Recovery Issuer Ingress), already covers
the *interactive* case: an operator-authenticated browser bridge to a full
Apple Screen Sharing session, gated by a signed, short-lived admission. That
is the right tool when a human needs the whole desktop. It is the wrong tool
for mechanical recovery (restart a stuck service, re-arm a supervisor, pull a
known-good config) — a full remote-desktop bridge is a larger attack surface
and a heavier credential-brokerage problem than a fixed set of recovery verbs
needs.

This ADR covers that narrower, automated case.

## Decision
Adopt **Option A — per-node signed LAN anchor** as the operational path;
document **Option B — physical out-of-band floor** as the fallback when every
network lane in Option A is down.

**Option A (to be built):**

1. **What "signed" means (closes SSA-1 / SHA-1), and its trust mapping
   (closes SSA R2-1).** SSH authenticates a *transport and account*; it does
   not by itself authenticate a *request*, and it does not tell the daemon
   which key authenticated the caller or whether the caller arrived over SSH
   at all. This ADR defines "signed" as a separate, self-contained property,
   verified independently of SSH:
   - **Encoding and namespace, field types pinned (closes SSA's
     implementation elaboration).** The envelope is
     `{node_id, verb, target, config_digest, request_id, issued_at, expires_at, epoch}`
     with field types fixed, not left to the implementation to infer:
     `node_id` (string, this node's pinned identity), `verb` (string, one of
     the enumerated allowlist names — §item 3), `target` (string, the
     verb's approved service label or volume UUID), `config_digest` (string,
     lowercase-hex `sha256`, exactly 64 characters), `request_id` (string,
     UUIDv4 canonical form), `issued_at`/`expires_at` (integer, Unix seconds
     UTC — never a float, never a string, never a different unit),
     `epoch` (integer, the daemon's **protected epoch** the signer read
     before signing — closes SSA R6-2 P1). `epoch` is an eighth, signed
     field, not an unsigned claim: the Verifier rejects, before any other
     check, an envelope whose `epoch` does not exactly equal the daemon's
     current protected epoch at verification time — not the caller's
     "admission epoch," not the epoch current when the envelope predates an
     override. A signer with a stale epoch gets a hard reject and must
     re-read the current epoch (published alongside the derived
     allowed-signers mapping) and re-sign; there is no partial-credit or
     grace window for a one-epoch-old envelope. This makes epoch a property
     the Verifier can check the same way it checks every other field —
     never an inferred admission-time fact. The
     parser rejects a duplicate JSON key or an unrecognized field in the
     envelope **before** canonicalization or signature verification runs —
     an envelope is well-formed-or-rejected, never "ignore what you don't
     recognize." Serialized as key-sorted, UTF-8, newline-terminated
     canonical JSON (no insignificant whitespace, fixed field order); the
     implementation publishes byte-level fixture examples of a valid
     canonical envelope alongside the code, because key-sorting and a
     trailing newline alone do not pin every JSON encoder's whitespace and
     escaping choices. The signer signs this exact byte sequence using
     `ssh-keygen -Y sign -n sirsi-recovery-anchor`
     (a **dedicated signature namespace**, distinct from any other use of
     the same key — `ssh-keygen -Y verify` is namespace-bound and rejects a
     signature produced under a different `-n`). An envelope whose signature
     verifies under any other namespace, or whose key format is not one the
     daemon explicitly enumerates (ed25519 and ecdsa-sha2-nistp256 at
     launch; nothing else), is rejected before any further check — unknown
     key or certificate formats never reach the allowlist step.
   - **The trust mapping's sole grant source is root-owned, not
     `authorized_keys` (closes SHA R3-1 / SSA R3-1).** An ordinary account's
     `authorized_keys` is account-writable — if membership in it could by
     itself create an anchor grant, any account able to add its own SSH key
     could enroll that key for root recovery verbs, turning an everyday SSH
     capability into a privilege escalation. This ADR therefore splits grant
     and restriction into two inputs with different owners and different
     powers, and neither can do the other's job:
     - **`anchor-grants.allow`** — root-owned, mode `0600`, located outside
       any account's home directory, writable only by the owner (directly,
       or via an owner-authorized enrollment flow that itself writes as
       root). Every line is an explicit `(ssh_pubkey_fingerprint, local UID,
       permitted verbs)` tuple. This file is the **only** source from which
       a grant can originate — not "the key appears somewhere in
       `authorized_keys`," not "the key can log in as that account." A key
       absent from `anchor-grants.allow` has no anchor grant regardless of
       its `authorized_keys` status, including an entirely unrestricted
       `authorized_keys` entry.
     - **`authorized_keys`** — account-writable, consulted for **narrowing
       only**. If a fingerprint holds a grant in `anchor-grants.allow` and
       that same fingerprint's `authorized_keys` entry carries a `from=`,
       `command=`, or other restriction, the restriction **excludes that
       fingerprint from the derived mapping by default** — a restriction
       written for SSH-session authorization is inherited as an anchor
       exclusion, never silently discarded. The only way to grant anchor
       use to an already-restricted key is an explicit `override: true`
       flag on that exact fingerprint's `anchor-grants.allow` line — a
       second, deliberate owner act naming the override, never an
       inference from the restriction's mere absence or from the grant
       entry's existence alone. Either way, `authorized_keys` can only take
       a grant away or scope it down; it can never add one, and an account
       editing its own `authorized_keys` can never create or enlarge an
       anchor grant for itself.
     - The **derived, anchor-local allowed-signers file** consumed by
       `ssh-keygen -Y verify` is rebuilt by a privileged sync step that
       reads both inputs under this rule and writes the output via
       **atomic replace** (write to a temp path, `fsync`, `rename` over the
       live path) — never an in-place edit a partial write could corrupt.
       The sync step **rejects and refuses to publish** a malformed or
       ambiguous input (duplicate fingerprint with conflicting verb sets,
       unparseable line, wrong file mode/ownership on either source file)
       rather than publishing a best-effort partial mapping; on rejection,
       **while the source it was deriving from is otherwise unchanged
       since the last successful publish** (closes SSA R6-4 P2 — this
       qualifier was already present in the test-coverage list and matrix
       but missing from this active prose), or if the previous successful
       sync is older than a bounded **freshness ceiling** with the source
       likewise unchanged, the daemon serves the last-known-good mapping if
       still within ceiling, or **fails closed** (denies all requests) once
       the ceiling is exceeded — a stale or broken policy input must never
       silently keep granting, and a source that *has* changed is the
       `dirty` case below, never this fallback.
     - A key present in `authorized_keys` with a `from=`/`command=`
       restriction and no `override: true` on its `anchor-grants.allow`
       line remains **absent from the derived mapping**: the restriction is
       inherited as exclusion, never as a narrowed-but-present grant.
     - **Self-restoration via one's own `authorized_keys` edit, scoped
       explicitly (closes SSA's implementation elaboration on lines
       109-123).** The categorical statement above — "an account editing
       its own `authorized_keys` can never create or enlarge an anchor
       grant for itself" — covers creating a grant that never existed in
       `anchor-grants.allow`. It does not by itself cover the narrower case
       of an account *removing* a `from=`/`command=` restriction on a key
       that already holds a root-owned grant: because exclusion here is
       derived live from the current `authorized_keys` content rather than
       recorded as its own persistent fact, removing the restriction
       removes the exclusion and the pre-existing root grant becomes
       effective again on the next sync — not a new grant, but the
       restoration of one the account did not itself create. This ADR
       permits exactly that and no more: restoring a pre-existing
       `anchor-grants.allow` grant by removing one's own restriction is
       within the account's existing power over its own
       `authorized_keys` entry, identical in kind to the account's
       pre-existing power to narrow or widen its own SSH login
       restrictions. It remains true, unchanged, that no `authorized_keys`
       edit can create a grant absent from `anchor-grants.allow` or widen a
       grant's permitted-verb set beyond what that root-owned line states.
   - **The ingress boundary is same-UID-plus-signature, not "arrived over
     SSH."** Kernel peer-credential checks on the local IPC socket (item 2
     below) can only ever prove the connecting process's UID — never which
     key authenticated it, nor whether an SSH session was involved. This ADR
     states that boundary explicitly instead of asserting an SSH-only
     ingress it cannot enforce: **any local process running as a UID present
     in the derived mapping, presenting an envelope whose signature verifies
     against that UID's mapped fingerprint, is authorized** — whether that
     process was forked from an SSH session or invoked the socket directly
     as that same local account. A different UID, or the right UID with a
     signature that does not verify against its mapped fingerprint, is
     denied regardless of transport. Multiple keys may map to one UID (one
     account, several enrolled devices); the mapping is keyed by fingerprint
     and permitted-verb set, not "the account's known public key" singular.
     This replaces, rather than inherits, SSH's own `from=`/forced-command
     semantics for anchor purposes — those remain fully enforced for
     ordinary SSH login, independent of and unaffected by this ADR.
   - The anchor daemon is the **verifier**: namespace, key format, and
     signature validity first (above); `expires_at`/`issued_at` against wall
     clock within a server-bounded TTL and skew (item 1a below); then
     `request_id` against the durable, atomically-reserved replay record
     (closes SSA R2-2, detailed in item 1a). A request that fails any check
     is denied and logged; it never reaches the verb dispatcher.
   - This gives the design a real, falsifiable meaning for "signed" — a
     forged or replayed request is rejected even by someone who has
     captured a live SSH session's output, because the signature is bound
     to `request_id` + `expires_at` + the specific verb/target/digest under
     a dedicated namespace, not to the transport. If a future revision drops
     this envelope and relies on SSH authentication alone, it must say so
     explicitly and drop "signed" from the title.
   - This does **not** change how an automated caller (a cloud agent, a
     supervisor) invokes the anchor: it uses the account's existing
     SSH key and existing `ssh-agent` (or an on-disk key it is already
     authorized to use) to produce the request signature — no owner login
     credential borrowed, no new secret issued to the caller. Whether that
     key can sign *without* an interactive unlock/touch confirmation is a
     per caller/device-tuple property to be verified against the actual
     key storage in use (software key, `ssh-agent`-held key, or
     hardware-backed key with a user-presence requirement) — it is not
     inherited from "this account is already authorized for SSH," and a
     tuple that requires interactive confirmation stays candidate, not
     qualified, until proven otherwise (§8).
   - **Test coverage required before qualification**: a different UID
     presenting a validly-signed envelope (must deny); a UID that holds a
     valid anchor grant under one fingerprint, presenting a different
     fingerprint's otherwise-valid signature mapped to a *different* UID
     (must deny — the presenting UID must equal the signing fingerprint's
     own mapped UID, not merely hold *some* valid grant); two keys enrolled
     on one UID, each independently valid (both must work, each under its
     own mapped permitted-verb set); local same-UID invocation with no SSH
     session involved (must succeed identically to an SSH-forked
     invocation); a key present in `authorized_keys` with a `from=`/
     `command=` restriction and no `override: true` on its
     `anchor-grants.allow` line (must be absent from the derived mapping,
     i.e. deny as unknown key); a
     key added to `authorized_keys` by the account itself with no matching
     `anchor-grants.allow` entry (must be absent from the derived mapping —
     an account cannot self-enroll); the privileged sync step given a
     malformed or ambiguous input **while the source it is deriving from
     is otherwise unchanged since the last successful publish** (must
     refuse to publish, serve last-known-good within the freshness
     ceiling, then fail closed once the ceiling is exceeded with no
     successful sync) — this fallback never applies once the source has
     changed and the republish attempting to catch up with that change
     fails; that case is `dirty` (closes SSA R5-3 P2, aligning this list
     with item 1a's dirty-state rule and the matrix row below).

1a. **Execution-time admission and replay atomicity (closes SSA R2-2).** The
   checks above are necessary but, taken alone, leave two gaps: a
   check-then-act race between validating a request and dispatching it, and
   an admission window that does not re-confirm validity immediately before
   execution. This item closes both.
   - **Scope of the Verifier's checks, made explicit.** Beyond signature/
     namespace/key-format (item 1): `node_id` must equal this node's own
     locally-pinned identity (a request signed for a different node is
     denied here, not forwarded); `target` and `config_digest` must equal
     the selected verb's build-time-approved identity (§item 3) — the
     Verifier rejects a mismatch before the Authorizer or Custody check ever
     runs; `issued_at`/`expires_at` must fall within a **server-bounded
     maximum TTL** (a fixed constant, not caller-supplied) and a **bounded
     clock-skew allowance** on `issued_at` — a request claiming a TTL longer
     than the constant, or an `issued_at` outside the skew window, is denied
     at the Verifier regardless of signature validity.
   - **Atomic admission, reservation key corrected (closes SHA R3-2 /
     SSA R3-2 implementation note).** The reservation's unique key is
     `UNIQUE(signing fingerprint, request_id)` — **not** the whole
     `(fingerprint, request_id, content-hash)` triple, which would let two
     different-content requests sharing a `request_id` both admit under
     different hashes. `sha256(canonical envelope bytes)` is stored as a
     **checked value against the existing unique row**, not folded into the
     uniqueness constraint. The reservation and the `pending` audit write
     (§item 4) happen in the **same durable transaction**, under the
     existing per-node serialization lock — there is no separate
     cache-check followed by a later cache-write. A `request_id` already
     reserved for that fingerprint with a *different* content hash fails
     immediately (replay of the ID with altered content is not treated as a
     fresh request); the *same* content hash within the replay window
     resolves to the existing record's outcome as a no-op (§item 3's
     idempotency rule), never a second admission. **Residual first-use risk,
     stated rather than eliminated:** a request whose `request_id` has not
     yet been reserved is not yet protected by this mechanism — a captured
     but never-yet-submitted valid signed envelope can still win first
     admission any time before `expires_at`, the same as any bearer
     credential with an expiry. This mechanism makes a `request_id`
     unreplayable **after** its first reservation; it does not make an
     unreserved, still-valid envelope un-usable before that point.
   - **Clock-rollback defense survives record pruning and equal-mark replay
     (closes SHA R4-1 P1 / SSA R4-2 P1 — revision 4's high-water mark was
     itself a replayable value).** Revision 4's mark was derived from
     admitted requests' own `issued_at` and compared with a strict `<`:
     admit envelope with `issued_at=T`, mark becomes `T`; after real expiry
     and pruning, roll the clock back to `T+1` and resubmit the identical
     envelope — `T` is not "behind" mark `T` (strict `<` is false), the
     replay row is gone, and the envelope's own signed `expires_at` is still
     in the future relative to the rolled-back clock, so TTL passes too.
     Changing the comparison to `<=` does not fix this alone: a mark that
     advances on *every* admission would then wrongly deny a second,
     legitimate, distinct `request_id` admitted in the same `issued_at`
     second (SSA's explicit caution). The defect is using a
     **request-derived** value as the rollback floor at all — a value an
     attacker-controlled or rolled-back clock can reproduce by construction.
     This revision instead persists a **daemon-maintained wall-clock
     ratchet**, independent of any individual request: `last_observed_wallclock`
     is updated to `max(current value, current wall-clock reading)` (a) in
     the same transaction as every admission, (b) on a periodic durable tick
     (a fixed interval, e.g. every 30s, written whether or not any request
     arrived) so the ratchet keeps advancing even during idle periods, and
     (c) at daemon startup before serving any request. The rollback check
     runs **at every admission and at startup** (not startup only): if the
     current wall-clock reading is behind `last_observed_wallclock` minus the
     max permitted skew, the daemon treats this as a detected clock rollback
     and **fails closed** — denies all admissions — until the condition
     clears (clock resynchronized past the ratchet, or an explicit
     owner-gated override). Because the ratchet advances from the daemon's
     own continuous observation of wall-clock time rather than from any one
     request's claimed `issued_at`, the counterexample above is caught
     regardless of pruning: by the time the original envelope's
     `expires_at` passed in real time, the ratchet had already advanced past
     it, and a clock rolled back to `T+1` reads as "behind the ratchet" even
     though no replay row and no request-derived mark survived. This
     coexists with, and does not replace, the per-fingerprint/global
     high-water mark from admitted `issued_at` values (§ above) — the
     ratchet catches rollback independent of admissions; the mark still
     orders admissions relative to each other. The ratchet is a single
     **global, daemon-wide** value, never a per-fingerprint one — it is
     derived from the daemon's own observation of wall-clock time, not
     from any individual fingerprint's history. A fingerprint's first-ever
     admission on this daemon is therefore checked against the *same*
     current ratchet value as every other admission; revision 5's
     "a new principal has no rollback floor yet and is evaluated on
     TTL/skew alone" exemption is removed (closes SSA R5-2 P1, first
     clause) — there is no case in which the rollback check is skipped,
     because the thing being checked against was never per-principal.
   - **Rollback check rechecked at pre-dispatch, not admission/startup
     alone (closes SSA R5-2 P1, second clause).** Revision 5 ran the
     ratchet comparison at admission and at daemon startup, while
     pre-dispatch revalidation (below) checked only expiry, generation,
     and `dirty`. A request admitted while the ratchet was still behind a
     rolled-back clock, then queued while the rollback is detected and the
     daemon fails closed, must not slip through at dispatch simply because
     the rollback check does not run there. Pre-dispatch revalidation
     therefore re-runs the identical "current wall clock behind
     `last_observed_wallclock` minus max skew" comparison, under the same
     shared guard as the generation/dirty/freshness checks, immediately
     before the Executor invocation — with no state reset between
     admission and dispatch. A rollback detected between a request's
     admission and its dispatch denies at pre-dispatch revalidation, the
     same way an intervening revocation does.
   - **Minimum replay-retention floor restored (closes SSA's "R4 deleted
     R3's TTL+skew-past-expiry requirement" note), pruning made atomic with
     the floor it depends on (closes SSA R5-2 P1, third clause).** A replay
     row (and any per-fingerprint value derived solely from it) is not
     eligible for pruning until at least `max permitted TTL + max permitted
     skew` has elapsed past that row's `expires_at`, persisted and checked
     by the pruning routine itself — pruning on any shorter schedule is a
     bug in the pruning routine, not a design choice the operator can tune
     away. This bounds how far pruning can ever run ahead of the ratchet
     above, so the two defenses are reinforcing rather than one silently
     substituting for the other. Concretely: the pruning routine, in the
     **same durable transaction** that deletes a row, persists
     `last_observed_wallclock = max(current value, trusted current
     wall-clock observation taken at prune time)` — **not**
     `max(current value, that row's expires_at)` (closes SSA R6-3 P1: a
     floor pinned to the deleted row's own `expires_at` is itself a
     request-derived value bounded by that row's TTL, so a restart at a
     rolled-back clock sitting at or before `expires_at` passes the
     ratchet check — floor not exceeded — at the exact moment the one row
     that could have caught the replay by `request_id` is already gone.
     Concrete countermodel: max TTL 60s,
     max skew 5s, an envelope issued at wall-clock 940 with a 1s TTL
     (`expires_at` 941), a ratchet stalled at 940 because its periodic tick
     missed a cycle, pruning running at 1006 — satisfying
     `expires_at + maxTTL + skew = 1006` — deletes the row and would have
     advanced the floor only to 941 under the old formula; a crash/restart
     at wall-clock 940 then re-admits the identical `issued_at`, since 940
     is not behind `941 - skew`. Advancing the floor to the *trusted
     wall-clock reading pruning itself took* (1006, not 941) closes this:
     any subsequent admission claiming wall-clock 940 fails the ratchet
     check outright, independent of whether its replay row still exists).
     The ratchet is therefore advanced to at least the trusted wall clock
     pruning observed *before or atomically with* each deletion, never
     after, and never bounded by the deleted row's own expiry. A periodic
     pruning tick that stalls, crashes, or restarts between computing
     eligible rows and committing their deletion must re-derive eligibility
     from the persisted floor on resume, not from an in-memory list
     computed before the interruption — there is no window in which rows
     are gone but the ratchet has not yet absorbed what they protected.
   - **Owner-gated override defined as a new protected epoch bound to a
     signed envelope field, with the ratchet re-anchored to real wall
     clock in the same transaction (closes SSA R5-2 P1 fourth clause and
     SSA R6-2 P1).** An owner-gated override that clears a
     detected-rollback fail-closed state must not simply lower or clear
     `last_observed_wallclock` — doing so would reopen exactly the replay
     window the ratchet exists to close, for every envelope whose validity
     interval the ratchet had already retired. Instead, an override mints
     a new, strictly-increasing **protected epoch** value, persisted
     alongside the ratchet. Revision 6 bound this to an *implicit*
     admission-time epoch, which SSA correctly found unverifiable: nothing
     in the seven-field envelope is epoch, so a captured, never-before-
     admitted envelope has no prior admission record to compare against,
     and binding it to "the daemon's current epoch at verification time"
     when first seen simply assigns it the new epoch — it can never be
     distinguished from a legitimately fresh one. This revision instead
     makes `epoch` the envelope's eighth **signed** field (closes SSA
     R6-2 P1 — see the envelope definition above): the Verifier rejects,
     before any other check, an envelope whose `epoch` field does not
     exactly equal the daemon's current protected epoch, both at admission
     and, under the same shared guard, again at pre-dispatch revalidation
     — so an admitted, queued request whose epoch is retired by an
     intervening override is denied before dispatch, not merely at
     admission. A signer always reads the current epoch (published
     alongside the derived allowed-signers mapping) before signing; an
     envelope signed under a retired epoch fails signature-field matching
     outright, with no implicit inference involved on either side.
     Minting a new epoch alone would leave every admission denied forever
     if the wall-clock ratchet from revision 5 still reads the pre-rollback
     clock as "behind" — SSA's "how rollback recovery resumes" question —
     so the same guarded override transaction that mints the new epoch
     also re-anchors `last_observed_wallclock` to the daemon's actual
     current wall-clock reading. **The ratchet, like the mark below, is a
     property of the current protected epoch, not one value monotone
     across the daemon's entire history (closes SSA R8-2 P2 — a prior
     draft claimed the ratchet is "never reset backward, only forward,"
     which is false of exactly this transaction: a false-future reading
     of `1000` corrected by an override to the real reading of `940` is a
     numeric decrease).** Within one epoch, the ratchet is monotone —
     every admission and every periodic tick only ever advances it,
     exactly as revision 5 defined. An authorized epoch transition is the
     one deliberate exception: it atomically preserves the retired
     epoch's ratchet value and admission history for audit, then
     initializes the new epoch's ratchet from one trusted current
     wall-clock observation — which may be numerically lower than the
     retired epoch's final value, because the two are independent epochs'
     state, not one sequence. This is safe specifically because the
     override is an explicit, owner-authenticated act (never a daemon
     self-heal on restart): a fresh request signed under the new epoch is
     admissible again once the new epoch's ratchet reflects real time,
     while every envelope signed under a prior epoch remains permanently
     rejected at the epoch check regardless of its own `expires_at` or any
     epoch's ratchet value — the two checks are independent and both must
     pass, so initializing the new epoch's ratchet for forward progress
     never reopens the retired epoch's window, which the epoch check
     alone protects. In effect: a daemon-clock rollback is recovered from
     by **invalidating the past** (every prior epoch's envelopes stop
     being admittable, permanently) while **restoring forward progress**
     (the new epoch starts its own ratchet from real time so new,
     correctly-epoched requests are not denied as "rollback" forever) —
     never by mutating the retired epoch's own ratchet value, which stays
     exactly what it was. The override is logged as a decision card per
     A32/A23 — it is the owner's call, not a silent recovery path a
     crashed daemon takes on its own restart.
   - **The admission-order high-water mark is scoped per protected epoch,
     not a single global sequence spanning an override (closes SSA R7-2
     P2).** The per-fingerprint/global high-water mark from admitted
     `issued_at` values (above) was, through revision 7, one unscoped
     sequence that an override's epoch bump never touched. Concrete
     failure this left open: epoch 1 admits an envelope with
     `issued_at=1000`, advancing the mark to `1000`; the daemon's clock is
     later found to read a false future value and the owner issues an
     override, minting epoch 2 and re-anchoring the wall-clock ratchet to
     the real current reading of `940`; a fresh, correctly-signed epoch-2
     envelope issued at the real time `940` passes the epoch check and the
     re-anchored ratchet, but still fails the retained mark check (`940`
     is behind `1000`) — the override's promised immediate forward
     progress does not actually happen. This revision makes the mark a
     property **of the current protected epoch**, not of the daemon's
     entire history: each epoch owns its own mark, initialized to the
     daemon's current trusted wall-clock observation at the moment that
     epoch is minted (the same observation the override uses to re-anchor
     the ratchet, taken once and used for both) and advanced only by
     admissions under that same epoch thereafter. The prior epoch's mark
     is retained, never deleted or merged forward — its admission history
     stays available for audit — but it is never read by a check against
     the current epoch's admissions; an epoch-N check reads only epoch-N's
     mark. A correctly-epoched, post-override request is therefore
     admissible immediately once it clears the epoch and ratchet checks,
     with no dependency on where the retired epoch's mark happened to
     stop. No old-epoch envelope becomes newly eligible by this change:
     the epoch field check (above) rejects it before the mark is ever
     consulted, exactly as before. The mark and the ratchet (above) are
     both epoch-scoped by this revision — neither is "the daemon's one
     value for all time," and neither claims to decrease within an epoch;
     each new epoch starts its own mark and its own ratchet from a fresh
     initial value that may be lower than the retired epoch's final
     value, which is a property of starting a new independent domain, not
     of resetting one sequence backward.
   - **Test coverage required before qualification (closes SSA R7-2
     P2).** An envelope admitted under epoch N advances epoch N's mark to
     a high value; an owner-gated override mints epoch N+1; a
     correctly-epoched, correctly-timed fresh request under epoch N+1,
     with an `issued_at` below epoch N's retained mark, is submitted
     immediately after the override (must be **admitted** — the retained
     epoch-N mark must not be consulted for an epoch-(N+1) admission);
     the same scenario repeated with the post-override request's
     `issued_at` set below the new epoch's own freshly-initialized mark
     (not below the retired epoch's mark) (must be **denied** at the
     epoch-(N+1) **ordering-floor/mark** check — attributed correctly to
     the mark, not the ratchet, since the ratchet compares the daemon's
     own current wall-clock reading against its floor minus skew and
     never reads the envelope's `issued_at` at all; closes SSA R8-2 P2,
     correcting the prior draft's wrong attribution of this outcome to
     "the re-anchored ratchet"); a genuinely separate rollback control:
     the daemon's own current wall-clock reading, independent of any
     envelope, found behind the new epoch's ratchet floor minus max skew
     (must be **denied ALL** by the ratchet check, which never consults
     any envelope's `issued_at`); an epoch-N envelope presented after the
     override, with an `issued_at` that would have passed epoch N's own
     mark had epoch N still been current (must be **denied** at the
     epoch field check, before the mark is reached at all); and a daemon
     with no override history at all, where the first-ever admission
     under epoch 1 is checked against epoch 1's own freshly-initialized
     mark, not an undefined or carried-over value (must behave identically
     to every subsequent
     admission under that same epoch).
   - **Revalidation immediately before dispatch, under a shared generation
     fence (closes SHA R3-1 / SSA R3-1 — the core convergent finding).**
     Revision 3's "re-read current state immediately before dispatch" was
     necessary but not sufficient: a re-read and a separate revocation
     publish are still two operations, and a revocation can commit in the
     gap between the re-read and the Executor invocation. This revision
     replaces "re-read" with an explicit **fence**: the privileged sync
     step (item 1) publishes each new derived mapping under a monotonic
     **generation counter**, guarded by a single mutex/version-CAS shared
     with dispatch. Immediately before the Executor is invoked, the daemon
     acquires that same guard, re-checks `expires_at` against current wall
     clock and the signing fingerprint's permitted-verb mapping against the
     **current generation** (never the generation snapshotted at
     admission), and the Executor invocation is committed **while still
     holding that guard** (or atomically bound to the generation value it
     just observed via a CAS token the sync step's publish would
     invalidate) — there is no window between "read current generation" and
     "invoke Executor" in which a concurrent publish can land unobserved.
     A request that has expired, or whose signing key has been revoked or
     whose generation has advanced past the one it was authorized under, in
     that interval is denied; the audit record resolves to `denied`, never
     proceeds to `completed`.
   - **Dirty/invalid policy state under the same guard (closes SHA R4-1 P1 /
     SSA R4-1 P1 — the fence covers publication, not source divergence or a
     failed republish).** The generation fence above serializes dispatch
     against a *successful* publish; it says nothing about the interval
     between the root-owned source (`anchor-grants.allow` or
     `authorized_keys`) changing and the sync step either publishing a new
     generation or failing to. Concrete gap: a grant is removed from
     `anchor-grants.allow`, but the sync step's republish fails (malformed
     second line, interrupted write, any rejection case in item 1's sync
     rule) before the freshness ceiling is reached; the old generation
     remains "current," and a request queued before the removal still reads
     that unchanged, still-current generation at the pre-dispatch fence and
     dispatches. Revision 4's "serve last-known-good within the freshness
     ceiling" fallback was written for the case where the source has not
     changed and the sync step simply has not run yet — it was never meant
     to, but as written did, also cover "the source changed and the
     republish failed." Revision 5 added an explicit **dirty** flag to the
     same generation-fence state: the sync step, on observing that the
     source's mtime/content-hash differs from what the currently-published
     generation was derived from, sets `dirty=true` under the guard
     *before* attempting to validate and republish; a successful validate
     + publish clears `dirty` and advances the generation atomically in
     that same guarded operation; a failed validate (malformed, ambiguous,
     wrong mode/ownership) leaves `dirty=true`. Pre-dispatch revalidation
     checks `dirty` in addition to generation and expiry: **`dirty=true`
     denies unconditionally, regardless of freshness ceiling** — the
     last-known-good fallback applies only to "sync hasn't run since the
     last successful publish, and the source is unchanged since then,"
     never to "the source changed and the daemon cannot prove the new
     policy."
   - **Closing the observation-lag window: pre-dispatch freshness is a
     synchronous, live check, not a wait on the async sync step (closes
     SSA R5-1 P1).** The `dirty` flag above is set by the privileged sync
     step, which runs on its own schedule (triggered by a filesystem
     watch or a periodic tick) — independent of any one dispatch. That
     leaves exactly the window SSA names: `anchor-grants.allow` is edited
     (grant removed) at time `t0`; the sync step has not yet run at `t1`
     when a queued request reaches pre-dispatch revalidation; `dirty` is
     still `false` because nothing has *observed* the `t0` edit yet; the
     generation is unchanged and still "current"; the request dispatches
     on a source that, as of `t0`, no longer authorizes it. Waiting for
     the async observer to catch up does not close this window at any
     fixed polling interval — it only shrinks it. This revision closes it
     by removing the dependency on the async observer for the *dispatch*
     decision: pre-dispatch revalidation, under the same shared guard as
     the generation/dirty/ratchet checks and immediately before the
     Executor invocation, performs a **synchronous, in-guard invocation of
     the identical content-hash validate function the async sync step
     uses** — not a separate, lighter-weight check — against both
     `anchor-grants.allow` and the signing fingerprint's `authorized_keys`
     content, right then, and compares the resulting hash pair against the
     hashes the currently-published generation was derived from.
     Revision 6 compared `mtime`+`size` instead, which SSA correctly found
     insufficient on two counts (closes SSA R6-1 P1): first, mtime+size is
     not content identity — a same-length replacement with a restored
     mtime (the ADR's own temporary-file-check language demonstrates this
     exact pattern) produces an identical pair while the content differs;
     second, the shared guard that comparison ran under was held by the
     sync publisher and the Executor, not by whatever process writes
     `anchor-grants.allow` or an account's `authorized_keys` — so a raw
     edit landing between the stat read and the Executor invocation was
     never actually fenced, only usually-fast-enough. Running the *same
     validate call* the sync step uses — not a cheaper proxy for it —
     synchronously and in-guard fixes the spoofability half: a content
     hash cannot be spoofed by a same-length/same-mtime replacement.
     It does not, and cannot, fence a raw writer that never acquires this
     guard at all — a root process editing `anchor-grants.allow` directly,
     or an account editing its own `authorized_keys`, writes straight to
     the filesystem with no lock to wait on (closes SSA R7-1 P1, replacing
     revision 7's false "no process on the write side left unfenced" and
     "never act on a source state that has already changed" claims, which
     asserted a fence over writers this design never gives one to).
     **Proposed vs. active, stated precisely.** `anchor-grants.allow` and
     an account's `authorized_keys` content are, at every instant,
     **proposed** policy — whatever bytes currently sit on disk, writable
     at will by their respective owners, with no commitment semantics of
     their own. **Active** authority for dispatch is a distinct, narrower
     thing: the `(hash_pair, generation)` snapshot that the most recent
     successful validate call — async publish or synchronous pre-dispatch
     call, under the same shared guard — read and recorded. **Raw writers
     may edit either proposed source at any time, including while
     validation or dispatch holds the internal guard (closes SSA R8-1 P1
     — removing the prior "exactly two timings"/"cannot occur mid-read"
     claim, which restated the same impossible mutual-exclusion invariant
     R7-1 was supposed to remove: the guard was never held by any raw
     writer, so nothing serializes a raw write against it, and a write
     can land at any point relative to a validate call's read, including
     mid-read.)** The guard serializes policy validation/publication and
     dispatch; it does not serialize raw writes. Validation hashes and
     parses the same captured byte buffers for both inputs and commits
     their exact hash pair and generation durably under the guard, or
     fails closed if a stable valid capture cannot be obtained. An edit
     whose bytes are not included in that committed snapshot remains
     proposed, even if its write completed before the Executor invocation
     — a write landing concurrently with, or ahead of, a validate call's
     read is simply not guaranteed to be captured by that specific call;
     it is captured by whichever call's read actually observed it. Only a
     guarded active-policy commitment is an acknowledged anchor
     revocation. This revision makes
     **the validate call's snapshot**, never a raw file write by any
     actor, the sole source of active authority, consistently for both
     the root-owned grant source and the account-writable restriction
     source: a proposed edit becomes active authority only when some
     later validate call reads it and successfully commits a new
     generation from it; until then, the prior generation's snapshot
     remains active and governs dispatch, and a pending proposed edit is
     not an acknowledged revocation of anything. A mismatch between a
     pre-dispatch read and the currently-published generation's recorded
     hashes is treated identically to `dirty=true` — deny unconditionally
     — **regardless of whether the background sync step has run yet**;
     this is the committed-snapshot case changing underneath a queued
     request, not a claim about the raw writer's own fencing. The async
     sync step's `dirty` flag remains the signal that *drives
     republishing* (so the derived allowed-signers mapping used for
     signature verification stays current); the synchronous in-guard
     validate call is the independent, narrower guarantee that dispatch
     itself never acts on a *snapshot* older than the one its own
     pre-dispatch read just took, which is a claim about snapshot
     freshness, not about raw-write fencing. This is two SHA-256 reads of
     bounded, root-owned local files under an already-held guard; SSA is
     correct that no latency claim is made here without measurement — the
     qualification matrix below requires a timed result before either
     lane is reported "proven" rather than "candidate."
   - **Commitment acknowledgement: a raw write completing is not a signal
     that a proposed restriction has taken effect (closes SSA R7-1 P1,
     second half).** Because activation happens only at the next validate
     call, an operator or account that has just written a restriction
     into `anchor-grants.allow` or `authorized_keys` has not thereby
     learned anything about whether that restriction is enforced yet — the
     write syscall returning is a filesystem fact, not a policy fact. This
     ADR requires the daemon to expose a **commit sequence number**, bound
     atomically to the specific state it reports (closes SSA R8-1 P1,
     second half — a prior draft of this state conflated "a validate call
     observed this hash" with "this hash is active," which a denied or
     dirty validation attempt would have falsely satisfied). The exposed
     status transaction reports, together and atomically: (a) the
     **active published snapshot** — the hash pair and generation of the
     currently-dispatching policy, updated only by a validate call that
     successfully commits a new generation; (b) the current **dirty or
     invalid** state, if any, kept separate from (a) rather than merged
     into it; and (c) last-observed/proposed metadata for diagnostics,
     clearly labeled as neither of the above. A validation attempt that
     merely observed a hash — whether it committed, found a mismatch and
     denied, or failed closed — advances (b)/(c) but never substitutes for
     (a). An operator or account that needs to know a specific edit is
     active polls this exposed state (a local, unauthenticated-read,
     root-owned status file or socket query — no new network surface)
     until the **active published snapshot's** hash for the file they
     edited matches the hash of their edit; before that match, the edit is
     proposed, not committed, regardless of elapsed time, the freshness
     ceiling, or any denied/dirty validation observation that happened to
     see the new bytes. The daemon never claims a restriction is "in
     effect" based on write completion, mtime, a validation observation
     that did not commit, or any signal other than the active published
     snapshot's recorded hash match. A past hash that merely matches
     historically is not proof of current active authority — only the
     current active published snapshot is.
   - **Test coverage required before qualification (closes SSA R7-1
     P1).** Edit committed strictly before a validate call's guarded read
     (must be reflected in that call's hash and treated as active from
     that call onward); edit landing strictly after a validate call's
     guard releases, with no subsequent validate call yet run (must read
     as the *proposed*, not-yet-active state — the prior generation stays
     authoritative for any dispatch in the interim, and the edit is not
     treated as a revocation that already occurred); a validation attempt
     that fails (malformed, ambiguous, wrong mode/ownership) on an edited
     source (must leave the prior generation active and `dirty` or
     last-known-good per item 1's rule — the failed attempt must not be
     reported as a commitment of the edit, successful or otherwise); an
     operator polling the exposed commit-sequence/hash state after writing
     a restriction (must observe the pre-edit hash until the next
     successful validate call runs, then the post-edit hash — never a
     premature "committed" report); and the same four cases repeated for
     an account-writable `authorized_keys` restriction, not only the
     root-owned `anchor-grants.allow` source, since both sources are
     covered by the identical validate call and must behave identically
     under all four.
   - **Parent-directory and ownership hardening for policy state (closes
     SSA's "root-owned file mode alone does not protect" note).** Root
     ownership and mode `0600` on `anchor-grants.allow`, the derived
     allowed-signers mapping, and the persisted generation/dirty/ratchet
     state protect those files only if their **parent directories** are
     also non-account-writable — a writable parent lets any account holding
     write on it replace the file wholesale (unlink + recreate) regardless
     of the child file's own mode and ownership. This ADR requires every
     directory in the path to each of these files, up to and including its
     immediate parent, to be root-owned with no group/other write bit; the
     qualification matrix (§8) must include a test that attempts exactly
     this replacement-via-writable-parent and expects it to be refused by
     filesystem permissions before the sync step ever reads the result.
   - **The execution linearization point, defined.** The single instant the
     Executor is invoked under the fence above is the linearization point:
     before it, revocation or expiry denies outright (above), because the
     fence makes "the generation dispatch committed under" and "the
     generation revocation published under" mutually observable — one must
     happen-before the other, never interleaved. After that instant, the
     action is in flight and this ADR makes **no promise of retroactive
     non-execution** — already-started operations are resolved only by the
     post-action verification step (§item 4) into `completed`, `failed`, or
     `unknown-reconcile-required`; a revocation arriving after that instant
     is handled as a future reconciliation input, not an in-flight abort.
   - **Negative control, split by fence implementation (closes SHA R4-2 P2
     / SSA R4-3 P2 — one assertion cannot hold for both allowed fence
     types).** The single control inherited from revision 3 ("pause after
     the fence read, commit a revocation, resume, the paused request must
     deny") is correct for a CAS fence but impossible for a held-mutex
     fence: pausing *after acquiring* a mutex does not release it, so a
     competing revocation publish cannot commit at all while the paused
     request still holds the guard — the control as written demands that a
     correct mutex implementation either deadlock or violate its own
     mutual-exclusion guarantee. This revision replaces it with a pair of
     per-variant controls, each asserting an outcome that follows from which
     operation commits first, rather than mandating an ordering that
     implementation can never produce:
     - **Mutex variant.** (a) Revocation publish commits and releases the
       guard before the dispatching request acquires it → the request's
       acquire observes the new generation → **deny**. (b) The dispatching
       request acquires the guard first (and, per the fence definition
       above, holds it through Executor invocation) → revocation's publish
       attempt blocks on the same guard and cannot commit until the guard
       is released → the held request's action **commits first**;
       revocation, once it does commit, governs only subsequent requests
       and triggers §item 4's post-action reconciliation for the one that
       already ran. Both outcomes are correct; the test asserts the
       Executor call count and the revocation-publish ordering for each
       case, not one cross-case expected verdict.
     - **CAS variant.** (a) Revocation publish advances the generation
       before the dispatching request's atomic commit attempt → the CAS
       fails on a stale generation token → **deny**, no Executor
       invocation. (b) The dispatching request's CAS commits first → the
       Executor invocation is already bound to the generation value the CAS
       observed; a revocation publishing afterward cannot un-commit it →
       same post-action-reconciliation outcome as mutex-(b). The
       qualification matrix (§8) names which fence variant the
       implementation uses and tests only that variant's two orderings.
   - **Test coverage required before qualification**: two requests with the
     same `request_id` and *different* content submitted concurrently (one
     must win admission, the other must fail — never both executing);
     the same `request_id` resubmitted after a daemon restart (must resolve
     from the persisted replay record, not re-admit as new); a request with
     `node_id` for a different node (deny at Verifier); a request claiming a
     TTL longer than the server-bound maximum (deny at Verifier); a request
     that is queued behind another and whose `expires_at` passes while
     queued (deny at the pre-dispatch revalidation, not at admission); a key
     revoked while its request is queued between admission and dispatch
     (deny at the pre-dispatch revalidation); the two fence-variant negative
     controls above, exercised against whichever variant the implementation
     actually uses; a source edit (grant removed) followed by a sync
     failure (malformed replacement line) before the freshness ceiling,
     with a request queued before the edit — the queued request must deny
     at pre-dispatch revalidation on `dirty=true`, not dispatch on
     last-known-good; replacement of `anchor-grants.allow`, the derived
     mapping, or the generation/dirty/ratchet state via a writable parent
     directory (must be refused by filesystem permissions); all replay rows
     for a fingerprint pruned, then the daemon's wall clock rolled back,
     then an old (previously valid, now-expired-by-real-time) envelope
     resubmitted at the exact `issued_at` the pruned row held (must deny via
     the daemon-maintained wall-clock ratchet, not the now-absent replay row
     or the request-derived mark alone); the ratchet itself advancing with
     zero admissions in flight (periodic durable tick observed in the
     persisted store).

2. **Local privilege boundary (closes SSA-2 / SHA-1-second-half).** The
   anchor is a system-domain launchd daemon listening on a **local-only,
   filesystem-permissioned IPC socket** (not a TCP port) — reachable, per
   item 1's ingress boundary, by any local process running as a UID present
   in the derived allowed-signers mapping, kernel peer-credential checked
   (not network-reachable). In the ordinary case that process is the SSH
   session's forked signed-request submission step; item 1 states
   explicitly that this is the expected path, not an enforced one — the
   daemon cannot and does not distinguish "forked from SSH" from "invoked
   locally as the same account" at the socket layer, only the UID and the
   envelope's signature. Authorization is **per-principal, per-verb**: the
   daemon holds the item-1 derived mapping `(ssh_pubkey_fingerprint) →
   {local UID, verbs permitted}`, so a given signing key's compromise is
   bounded by that key's verb entry, never by "whatever that account's
   shell could do." **This explicitly narrows, and replaces, the prior
   "compromised-lane blast radius is bounded by the allowlist" claim**:
   that claim is true only for actions routed through this daemon. It does
   **not** reduce the residual risk already carried by an unrestricted SSH
   account on that host — a compromised SSH key still has whatever rights
   that account's shell already has, independent of this ADR. This ADR
   bounds the *anchor's own* attack surface; it makes no claim about the
   pre-existing SSH account's blast radius.

3. **Verb custody and safety (closes SHA-3).** Each allowlisted verb
   (restart-broker, remount-volume, re-arm-supervisor, fetch-known-good-config)
   is bound at build time to: an approved, absolute, root-owned executable
   path (never a caller-supplied path or `$PATH` lookup); an approved
   service/volume identity (a fixed launchd label or a fixed volume UUID,
   never a caller-supplied name or mount point); a bounded timeout and a
   fixed, idempotent retry policy (duplicate `request_id` within the replay
   window is a no-op, not a second execution); and a mandatory post-action
   verification step (re-check the service/volume state and record the
   observed outcome, not just "command exited 0"). `restart-broker`
   specifically must **acquire ADR-045/ADR-046's own ownership lock/lease**
   for the target broker — not merely inspect its state — before acting
   (closes SSA R2-3). Inspecting ownership and then invoking the verb are
   two steps with a race between them: another broker owner can acquire in
   the gap. The anchor instead calls ADR-045/046's existing authoritative
   acquire operation, holds that fence across the Executor's action and the
   post-action readback (item 4 below), and releases it only after the
   audit record resolves — it never checks, releases, and re-acquires. If
   ADR-045/046 exposes no compatible fenced-acquire for a given target (only
   a read-only status check), the anchor **denies** the request rather than
   proceeding on an inspect-only check; this ADR does not add a parallel
   ownership database, it calls the one ADR-045/046 already owns. A target
   marked owner-managed, or whose fence cannot be acquired because another
   owner already holds it, is denied the same way.
   **Test coverage required before qualification**: a competing non-anchor
   owner acquires the target's lock in the window between the anchor's
   custody inspection and its dispatch attempt — the anchor's fenced-acquire
   call must fail and the request must deny, not proceed on the now-stale
   inspected state.
   `fetch-known-good-config` fetches only from a fixed, owner-approved
   source location and verifies an integrity digest (not a version string)
   against a pinned allowlist before any write; it refuses a
   caller-writable path, a symlink target, or a URL/path supplied by the
   request. No verb in this table reads, writes, or rotates any credential,
   key, or FileVault/Keychain/SIP state — that remains a strict owner gate
   this ADR does not touch.

4. **Audit and failure semantics (closes SSA-3 / SHA-3-second-half).** A
   durable local record — identity, node, verb, target, request_id,
   decision — is written and fsynced **before** any verb executes. If that
   write cannot be durably persisted, the request is denied (fail closed);
   the anchor never executes an unaudited action. Requests on a given node
   are serialized (one in flight at a time); a crash or restart mid-verb
   leaves the audit record in an explicit `pending` state until the
   post-action verification step (§3) resolves it to `completed`,
   `failed`, or `unknown-reconcile-required` on next startup —
   `pending` is never silently promoted to `completed`. No secret, token,
   or payload body is ever written to the log — identity and decision only,
   matching ADR-075 §6.

5. **No weakening of existing controls.** The anchor **must not weaken**
   SSH, SIP, FileVault, Tailscale, or Keychain. No new bypass, no relaxed
   host-key checking, no disabling of an existing control to make the
   anchor easier to reach.

6. **Lane status is reported per-lane, never pooled, and labeled by
   evidence, not asserted as proven (closes SSA-4 / SHA-5).** M1 LAN SSH,
   M5 LAN SSH, and the Thunderbolt bridge rails are **previously observed
   transports** — each carries an evidence date from prior sessions, not a
   qualification of *this* recovery daemon running over them. They are not
   called "proven lanes" anywhere in this ADR or in the anchor's status
   reporting; "proven" is reserved for a lane that has passed the
   qualification matrix in §8. Per A35 (scope the check to the claim) and
   ADR-075 §8's point about anchor independence: two lanes sharing a
   router, a power strip, or a single issuer are not two independent lanes,
   and the status report to Pantheon/Ra must say so per-lane rather than
   rolling them into one "recovery: OK."
7. The anchor is **load-bearing infrastructure** once installed (A32): it
   must be recognized by pidfile, not by process name, in any future
   kill/renice/reaper path, the same way the Gemma broker is — a generic
   "largest RSS" or "unknown process" sweep must never treat it as
   expendable.

**Option A's failure envelope, stated explicitly (closes SHA-4):** Option A
recovers a node that is **booted, has a reachable network lane, and has
already passed FileVault pre-boot unlock** — it is authorized-service
recovery on a running machine, not unattended cold-boot or pre-unlock
recovery. It cannot recover: a dead `sshd` or dead network stack on the
target (there is no transport to reach the anchor), a machine asleep with
networking suspended, or a machine sitting at FileVault's pre-boot unlock
screen (postboot SSH does not exist yet at that stage). Those failure modes
require either a person at the console or Option B.

**Option B (documented, not built now):** a second trusted Mac with Apple
Configurator or DFU restore is the physical out-of-band floor. It requires a
person and a second machine on-site, so it is not "ownerless" — it is the
honest last resort when every lane in Option A is simultaneously unreachable
(e.g. the whole LAN is down, not just one path on it). Option B has two
distinct, non-interchangeable procedures that must not be collapsed:
- **Non-destructive firmware revive** (Apple Configurator "Revive") —
  attempts to restore firmware/bridgeOS on a Mac that is unresponsive but
  not data-corrupted; does not erase the data volume in the common case.
- **Destructive DFU restore** — a full restore that **erases the Mac**, per
  Apple's documented procedure: a second Mac running macOS 14+, both Macs
  on power, Configurator with internet access, and a compatible direct
  USB-C cable at the model-specific DFU port (source:
  https://support.apple.com/en-us/108900, read 2026-10-09).
Both are **attended** procedures — a person operates the second Mac — and
DFU restore requires its own, separate restore authorization decision; it is
never the default simply because a network lane happens to be unavailable.
This ADR does not prescribe DFU restore as a step; it names it as the last
resort when revive also fails.

## Alternatives Considered
1. **Physical OOB only (Option B as the sole path)** — rejected: requires a
   person present, which defeats the "ownerless" requirement for the common
   case (one service wedged, network otherwise fine).
2. **Reuse ADR-075's screen-sharing bridge for automated recovery** —
   rejected: that path authenticates an *operator* and grants a *full
   desktop*; mechanical recovery needs neither a human in the loop nor
   keyboard/mouse control, and inheriting that attack surface for a verb as
   small as "restart a daemon" is disproportionate.
3. **A new network listener with its own auth** — rejected: adds a new open
   port and a new credential type instead of riding the LAN SSH session the
   owner already trusts and already authorizes; more attack surface for no
   functional gain.

## Consequences
- **Positive**: mechanical recovery no longer depends on a cloud agent
  reaching in over whatever's open, or on the owner being physically present;
  the verb set is small, fixed, and audited, so an action routed through the
  anchor is bounded by the allowlist and the signed-request check (Decision
  items 1–2) rather than by ambient shell access. This bounds the
  **anchor's own** attack surface only — it is not a claim about the
  pre-existing SSH account's blast radius, which is unchanged by this ADR.
- **Negative**: the anchor is one more always-on, launchd-resident service
  per node that must survive reboots and be protected from being treated as
  an expendable process (A32) — it is new infrastructure to maintain, not a
  config change.
- **Risk**: allowlist creep is the primary risk (a "safe" verb table growing
  argv passthrough one convenience patch at a time); mitigated by requiring
  SHA+SSA review of any verb-table change, not just the initial design.
  Secondary risk: a lane-independence claim that is false in practice (e.g.
  two "separate" TB rails sharing a hub) reads as a clean board while one
  real lane exists — the per-lane reporting in Decision item 6 exists
  specifically to make that visible rather than averaged away.

## Data Flow (closes SHA-5's diagram ask)
```
caller (owner session or authorized automated agent)
  │  signs {node_id, verb, target, config_digest, request_id, issued_at,
  │  expires_at, epoch} under namespace "sirsi-recovery-anchor" with its
  │  existing SSH key, after reading the daemon's current protected epoch
  │  (typically from an existing SSH session; item 1 permits any local
  │   process running as the same mapped UID — ingress is UID+signature,
  │   not "arrived over SSH")
  ▼
local-only IPC socket, peer-credential checked (kernel-enforced UID only,
same host, never network-reachable)
  ▼
anchor daemon — Verifier
  ├─ wrong namespace / unenumerated key format → DENY, log, stop
  ├─ epoch ≠ daemon's current protected epoch → DENY, log, stop
  ├─ signature does not verify against derived allowed-signers mapping → DENY, log, stop
  ├─ node_id ≠ this node, or target/config_digest ≠ verb's approved identity → DENY, log, stop
  ├─ issued_at/expires_at outside server-bound TTL+skew → DENY, log, stop
  ▼ (signature valid, scoped to this node/verb, within TTL)
anchor daemon — Atomic admission
  ├─ UNIQUE(fingerprint, request_id) already reserved with a DIFFERENT
  │  content hash → DENY, log, stop
  ├─ same content hash already reserved for that key → return existing
  │  outcome (no-op)
  ├─ issued_at behind the fingerprint's/global persisted high-water mark
  │  FOR THE ENVELOPE'S OWN (already-verified) epoch → DENY, log, stop
  │  (orders admissions relative to each other within one epoch; a prior
  │  epoch's mark is never consulted for the current epoch)
  ├─ current wall clock behind the daemon-maintained wall-clock ratchet
  │  minus max skew → DENY ALL, log, stop (rollback defense, request-
  │  independent, checked here and at startup, survives replay-row pruning)
  ▼ (fresh reservation + `pending` audit write + high-water-mark +
     ratchet update, same transaction, node-serialized)
anchor daemon — Authorizer
  ├─ (pubkey_fingerprint, verb) not in current allowlist → DENY, mark audit `denied`
  ▼ (principal permitted for this verb)
anchor daemon — Custody fence acquire (verb-specific, via ADR-045/046)
  ├─ no compatible fenced-acquire, fence already held by another owner, or
  │  config source/digest unapproved, or path/symlink caller-writable
  │  → DENY, mark audit `denied`, release nothing (never acquired)
  ▼ (fence held)
anchor daemon — Pre-dispatch revalidation, under the shared generation fence
  ├─ epoch ≠ daemon's current protected epoch, rechecked here (not
  │  admission only, so an intervening override retires a queued request)
  │  → DENY, mark audit `denied`, release fence
  ├─ dirty=true (sync step observed source divergence not yet validated
  │  and published) → DENY unconditionally, mark audit `denied`, release
  │  fence — freshness-ceiling last-known-good fallback does NOT apply here
  ├─ synchronous, in-guard content-hash validate of anchor-grants.allow /
  │  the signing fingerprint's authorized_keys entry, run NOW using the
  │  same validate function the async sync step uses, differs from the
  │  hashes the current generation was derived from → DENY as dirty, same
  │  as above — does not wait for the async sync step to observe the
  │  change, and is not spoofable by a same-mtime/same-size replacement
  ├─ current wall clock behind the daemon-maintained wall-clock ratchet
  │  minus max skew, rechecked here (not admission/startup only) → DENY
  │  ALL, mark audit `denied`, release fence
  ├─ expires_at now passed, or signing key revoked/generation advanced since
  │  admission — checked against the CURRENT generation under the same
  │  mutex/CAS the privileged sync step publishes under, never a snapshot
  │  → DENY, mark audit `denied`, release fence
  ▼ (still valid under current generation — Executor invocation commits
     while still holding this same fence; this is the linearization point)
Executor — runs the ONE fixed, root-owned absolute executable bound to this
verb, fence still held
  ▼
Post-action verification — re-checks service/volume state
  ▼
Audit writer — resolves `pending` → `completed` | `failed` | `unknown-reconcile-required`
  ▼
Custody fence release (only now, after resolution)
  ▼
Status report to Pantheon/Ra — per-lane, per-node (never pooled; §Decision-6)
```
Every arrow left of "Executor" can terminate in DENY; only a request that
clears signature/namespace/key-format, node/target/TTL scope, atomic
admission, allowlist, and custody-fence acquisition, in that order, reaches
the single fixed executable for its verb — and the fence it acquired stays
held until the audit record resolves, never released-and-reacquired.

## Qualification Matrix (closes SSA-4 / SHA-5)
Before any lane or verb is reported as "proven" rather than "candidate,"
each row below must have a reviewed, dated test result on file:

| Test case | Expected result |
| :--- | :--- |
| Positive recovery (valid signed request, authorized verb, clear custody) | `completed`, post-action verification confirms state |
| Denied principal (valid signature, verb not in that principal's allowlist) | `denied`, no execution, audit entry written |
| Denied verb target (valid principal, `target`/`config_digest` ≠ verb's approved identity) | `denied` at **Verifier**, no execution |
| Denied verb path (valid principal/target, unapproved config source, caller-writable path, or symlink) | `denied` at **Custody**, no execution |
| Replayed request (same `request_id` resubmitted within window) | No-op per idempotency rule, not a second execution |
| Expired request (`expires_at` passed) | `denied` at Verifier, before allowlist check |
| Revoked authority, revocation observed at or before the pre-dispatch fence (principal's grant removed/narrowed before the Executor invocation commits) | `denied` at pre-dispatch revalidation; queued/not-yet-dispatched requests are **not grandfathered** |
| Revoked authority, revocation published strictly after the Executor invocation has committed under the generation fence | **No retroactive non-execution promise** — action resolves via post-action verification into `completed`/`failed`/`unknown-reconcile-required`; revocation is a reconciliation input, not an abort |
| Audit store unavailable (disk full / fsync failure) | `denied` fail-closed, no execution, attempt logged where possible |
| Protected/owned workload (ADR-045/046 marks target owner-managed) | `denied` at custody check |
| Concurrent requests to same node | Serialized; second request waits or is rejected, never interleaved |
| Interrupted recovery (daemon restarts mid-verb) | Audit resolves from `pending` to `unknown-reconcile-required`, never silently `completed` |
| Per-lane independence (e.g. two TB rails on one hub) | Status report shows shared dependency, not two independent "OK" lanes |
| Dead `sshd`/network on target | Lane reports `unreachable`, distinct from `denied` — never conflated |
| Presenting UID does not equal the signing fingerprint's mapped UID (the presenting UID may itself be mapped under a *different* fingerprint, or absent from the mapping entirely — both deny the same way) | `denied` — the presenting UID is not the signing fingerprint's own mapped UID; holding a grant under any other fingerprint does not substitute |
| Presenting UID holds *some* valid anchor grant under a different fingerprint, but differs from the mapped UID of the fingerprint that actually signed this envelope | `denied` — holding any grant is not sufficient; the presenting UID must equal the signing fingerprint's own mapped UID |
| Two keys enrolled on one UID | Both succeed independently, each bound to its own mapped permitted-verb set |
| Local same-UID invocation with no SSH session involved | Succeeds identically to an SSH-forked invocation |
| Key in `authorized_keys` with `from=`/`command=` restriction, no `override: true` on its `anchor-grants.allow` line | Absent from derived mapping; `denied` as unknown key |
| Key added to `authorized_keys` by the account itself, no matching `anchor-grants.allow` entry | Absent from derived mapping; `denied` as unknown key — self-enrollment via `authorized_keys` alone is impossible |
| Privileged sync step given a malformed/ambiguous `anchor-grants.allow` or `authorized_keys` input, with the underlying source otherwise unchanged since the last successful publish | Refuses to publish; serves last-known-good within freshness ceiling, then fails closed past the ceiling — distinct from the `dirty` row below, which applies once the source has actually changed |
| Two requests, same `request_id`, different content, submitted concurrently | One wins admission under `UNIQUE(fingerprint, request_id)`; the other fails — never both execute |
| Same `request_id` resubmitted after daemon restart | Resolves from persisted replay record, not re-admitted as new |
| Request `node_id` names a different node | `denied` at Verifier |
| Request claims a TTL longer than the server-bound maximum | `denied` at Verifier |
| Queued request whose `expires_at` passes before dispatch | `denied` at pre-dispatch revalidation, not at admission |
| Signing key revoked while its request is queued between admission and dispatch | `denied` at pre-dispatch revalidation |
| Negative control (mutex variant), case (a): revocation publish commits and releases the guard before the dispatching request acquires it | Request's acquire observes new generation; `denied`, no Executor invocation |
| Negative control (mutex variant), case (b): dispatching request acquires the guard first and holds it through Executor invocation | Revocation publish blocks until guard release; the dispatched action's Executor invocation **commits first** (not the same as `completed` — post-action verification still resolves it to `completed`/`failed`/`unknown-reconcile-required`); revocation governs only subsequent requests (post-action reconciliation) |
| Negative control (CAS variant), case (a): revocation advances the generation before the dispatching request's atomic commit | CAS fails on stale generation token; `denied`, no Executor invocation |
| Negative control (CAS variant), case (b): dispatching request's CAS commits before a subsequent revocation publish | Executor invocation proceeds bound to the observed generation; revocation cannot un-commit it (post-action reconciliation) |
| Source edit (grant removed) followed by sync failure (malformed replacement) before the freshness ceiling, with a request queued before the edit | Queued request `denied` at pre-dispatch revalidation on `dirty=true`, never dispatched on last-known-good |
| Source unchanged, sync simply has not run since last successful publish, freshness ceiling not yet exceeded | Last-known-good mapping served (not `dirty`) |
| Replacement of `anchor-grants.allow`, the derived mapping, or the generation/dirty/ratchet state via a writable parent directory | Refused by filesystem permissions before the sync step ever reads the result |
| All replay rows for a fingerprint pruned, daemon clock rolled back, previously-valid-but-now-expired envelope resubmitted at the exact `issued_at` the pruned row held | `denied` via the daemon-maintained wall-clock ratchet (checked at admission, not startup only), not re-admitted as new |
| Wall-clock ratchet advances with zero admissions in flight over a full tick interval | Periodic durable tick observed in the persisted store |
| Replay row pruned before `max permitted TTL + max permitted skew` has elapsed past its `expires_at` | Refused by the pruning routine itself — not an operator-tunable schedule |
| Competing non-anchor owner acquires target lock between custody inspection and dispatch | Fenced-acquire fails; request `denied`, not proceeded on stale state |
| `anchor-grants.allow` edited (grant removed) at `t0`; a request already admitted and queued reaches pre-dispatch revalidation at `t1 > t0`, before the async sync step has run at all | `denied` via the synchronous in-guard content-hash validate call at pre-dispatch, not dependent on the sync step having observed `t0` yet |
| `anchor-grants.allow` replaced with a same-length file, mtime restored to match the previously-published generation's recorded value, content actually changed | `denied` — the content-hash comparison catches the change; an mtime+size-only comparison (revision 6, superseded) would not |
| Signing fingerprint's `authorized_keys` entry edited by the account itself (restriction added or removed) between admission and pre-dispatch | `denied`-or-admitted consistently with the content-hash check — the account-writable source is covered by the same synchronous validate call as the root-owned source, not a narrower or separately-timed check |
| Envelope's `epoch` field does not equal the daemon's current protected epoch at verification | `denied` at Verifier, before any other check — including a never-before-seen envelope, which gets no implicit admission-epoch inference |
| Envelope admitted under epoch N; an owner-gated override mints epoch N+1 before the request reaches pre-dispatch | `denied` at pre-dispatch's epoch recheck, not only re-checked at admission |
| Owner-gated override mints a new epoch; a correctly-epoched (N+1) fresh request is submitted immediately after | Admitted — the override transaction re-anchors the wall-clock ratchet to real time in the same transaction, so the new epoch does not itself leave every request denied as rollback |
| Max TTL 60s, max skew 5s; envelope issued at wall-clock 940 with a 1s TTL (`expires_at` 941); ratchet stalled at 940 (periodic tick missed a cycle); pruning runs at 1006 (`expires_at` + maxTTL + skew) and deletes the row; daemon crashes and restarts at wall-clock 940, resubmitting the identical envelope | `denied` via the ratchet — the floor was advanced to the trusted wall-clock observation taken at prune time (1006), not to the deleted row's own `expires_at` (941), so 940 fails `940 < 1006 - skew` even though the replay row and its request-derived mark are both gone (closes SSA R6-3 P1 countermodel) |
| Fingerprint's first-ever admission on this daemon, submitted while the daemon's current wall-clock ratchet is behind a rolled-back clock | `denied` via the global ratchet — no new-principal exemption; the ratchet applies identically regardless of admission history |
| Request admitted while ratchet check passes, then a rollback is detected (ratchet check would now fail) before the request reaches dispatch | `denied` at pre-dispatch revalidation's ratchet recheck, not only at admission/startup |
| Pruning tick computes eligible rows, then crashes/restarts before committing their deletion | On resume, eligibility is re-derived from the persisted ratchet floor, never from the pre-crash in-memory list; no row is deleted without the ratchet already covering it |
| Owner-gated override clears a rollback fail-closed state | A new protected epoch is minted, and its own ratchet is initialized from the daemon's trusted current wall-clock reading, in the same transaction; every envelope signed under a prior epoch is rejected at the Verifier on its `epoch` field, regardless of its own `expires_at`; the retired epoch's own ratchet value is preserved unchanged for audit — it is never mutated — while the new epoch's ratchet may be numerically lower than it, since the two are independent epochs' state, not one value reset backward (closes SSA R8-2 P2) |
| Epoch N admits a request, advancing epoch N's high-water mark to a high value; override mints epoch N+1; a correctly-epoched, correctly-timed fresh request under epoch N+1 with `issued_at` below epoch N's mark is submitted immediately after | `admitted` — epoch N's retained mark is never consulted for an epoch-(N+1) admission (closes SSA R7-2 P2) |
| Post-override epoch-(N+1) request with `issued_at` below the new epoch's own freshly-initialized mark | `denied` at the epoch-(N+1) ordering-floor/mark check — not the ratchet, which never reads an envelope's `issued_at` (closes SSA R8-2 P2, correcting the prior row's wrong attribution) |
| Daemon's own current wall-clock reading (independent of any envelope) found behind the new epoch's ratchet floor minus max skew | `denied ALL` by the ratchet check — the genuine rollback control, distinct from the ordering-floor/mark row above |
| Epoch-N envelope presented after an override to epoch N+1, with an `issued_at` that would have passed epoch N's own mark had epoch N still been current | `denied` at the epoch field check, before the mark is ever reached |
| A raw edit to `anchor-grants.allow` or `authorized_keys` lands while a dispatch's own mandatory pre-dispatch validate call still holds the guard, strictly before that call's Executor invocation | That dispatch proceeds on whichever bytes its own validate call actually captured and committed — not on a stale snapshot, since every dispatch performs its own synchronous in-guard validate call (item 1a); the edit is not acknowledged as a revocation unless this call's own read captured it (closes SSA R8-2 P2, replacing the prior, impossible "dispatch with no subsequent validate call" row) |
| A raw edit lands after one dispatch's validate call has already committed its snapshot and released the guard; a **later**, separate dispatch request then reaches pre-dispatch revalidation | The later dispatch performs its own mandatory synchronous validate call and denies as `dirty`/mismatch if the edit changed the content hash since the previously-published generation — it never reuses the first dispatch's now-stale snapshot |
| A validate call attempt fails (malformed, ambiguous, wrong mode/ownership) against an edited source | Prior generation stays active and governs any dispatch in the interim; retaining those old snapshot bytes as active is permitted, but dispatching while `dirty`/invalid is not — state is `dirty` or last-known-good per item 1's rule (last-known-good only where the source is proven unchanged since the last successful publish, never where it has changed and the republish failed), applying identically to the root-owned grant source and the account-writable restriction source; the failed attempt is never reported as committing the edit |
| Operator writes a restriction, then polls the daemon's exposed commit-sequence/hash state before any validate call has run against it | State reports the active published snapshot's pre-edit hash, with dirty/invalid state reported separately; `committed` is reported only after a validate call successfully commits a new active generation whose hash matches the operator's write — a validation observation that merely saw the new bytes without committing (including one that denied as dirty) never advances the reported active hash |

A lane or verb with an unresolved row stays labeled "candidate," not
"proven," in every status surface (Pantheon dashboard, Ra report, this ADR).

## Scoped Value (closes SSA's publication/commercialization ask)
This section is platform groundwork, not a launchable-feature claim.
- **User/pain**: the owner (sole operator across the fleet) today recovers a
  wedged service either by hand or via an ad hoc cloud-agent path that
  ADR-045/046 already identified as the wrong inversion for the local-LLM
  broker. The pain is operator time and an unreviewed recovery surface.
- **Workflow**: an authorized caller (owner session or an already-authorized
  automated agent) signs a bounded recovery request over an existing SSH
  session; the anchor verifies, authorizes, audits, and executes exactly one
  allowlisted verb, then reports per-lane status.
- **Value**: bounded, audited mechanical recovery without a new always-open
  network surface and without physical presence for the common case.
- **Trust**: no new credential type, no weakened existing control (§Decision
  item 5), fail-closed audit (§Decision item 4), owner-gated verb-table
  changes (Consequences — Risk).
- **Operational owner**: Ra (this ADR's author) owns implementation and the
  SHA+SSA re-review of any verb-table change; the owner retains the gate on
  minting/enrolling any new SSH key.
- **Done evidence**: every row of the Qualification Matrix above has a
  dated, reviewed result on file, plus a passing SHA+SSA review of the
  implementation against this revised design. No "proven lane" or
  "recovery: OK" claim is made before that evidence exists.

## Review History
- Revision 1 (head `e22478e1`): SSA CHANGES_REQUESTED (signing/admission
  contract undefined, SSH blast-radius claim unscoped, audit/failure
  semantics absent, availability claim ahead of evidence) and SHA
  CHANGES_REQUESTED (same four gaps, plus verb custody/ADR-045/046
  reconciliation and the Option B attended-procedure distinction).
- Revision 2 (head `c5680351`): responded to all P1/P2 items from both
  revision-1 reviews — see inline "(closes SSA-N / SHA-N)" markers above.
  SSA reviewed this exact head and returned CHANGES_REQUESTED on three
  remaining items (R2-1 trust mapping/SSH-restriction bypass, R2-2
  execution-time admission/replay atomicity, R2-3 custody-fence atomicity)
  plus two editorial notes (the "(built here)" wording, signing-key
  availability qualified per caller/device tuple). SHA's parallel review of
  this same head returned BLOCKED, no verdict — not because the commit was
  unpushed, but because source was inaccessible from SHA's reviewing worker
  (DNS/`FETCH_HEAD` failure in that worker; corrected here per SHA's
  revision-4 review, which proved the remote object was present throughout
  and traced the failure to that worker's own resolution/fetch path, not to
  remote absence — A37's "a record exists only on origin" does not apply
  here, since the record was on origin the whole time). No revision-2
  acceptance is inferred from this correction.
- Revision 3 (head `1087bf15`): responded to all three of SSA's revision-2
  P1/P2 items and both editorial notes — see inline "(closes SSA R2-N)"
  markers (now superseded, see below). Both SHA and SSA independently
  reviewed this exact head and both returned CHANGES_REQUESTED: SHA found
  R3-1 (P1, revocation not serialized with dispatch — a re-read and a
  separate revocation publish remain two operations) and R3-2 (P2,
  qualification-matrix rows contradicting the stated linearization-point
  and Verifier/Custody-stage rules), plus an implementation note on the
  reservation key. SSA found R3-1 (P1, the trust-mapping source itself —
  account-writable `authorized_keys` — was never root-protected, so the
  "sync on every change" claim did not establish synchronous revocation),
  R3-2 (P1, the clock-rollback defense did not survive replay-record
  pruning), and R3-3 (P2, the same qualification-matrix contradictions SHA
  found, plus a missing two-UID denial case and a Verifier/Custody
  misattribution), plus implementation elaborations on field encoding and
  the first-use/bearer-risk scoping of the "unreplayable" claim.
- Revision 4 (head `f2d03042`): responds to all P1/P2 items from both of
  revision 3's reviews — see inline "(closes SHA R3-N / SSA R3-N)" markers
  above. Specifically: introduced `anchor-grants.allow` as the sole,
  root-owned grant source (`authorized_keys` narrows only, never grants);
  replaced "re-read before dispatch" with an explicit generation-fenced
  mutex/CAS shared between policy publication and dispatch commitment;
  added a durable, pruning-independent high-water mark for clock-rollback
  detection with fail-closed startup behavior; corrected the reservation
  key to `UNIQUE(fingerprint, request_id)` with content hash as a checked
  value; pinned envelope field types and duplicate/unknown-field rejection;
  scoped the "unreplayable" claim to state the residual first-use/bearer
  risk on an unreserved `request_id` rather than asserting it away; and
  corrected three qualification-matrix rows (verb-target vs. verb-path
  staging, the grandfather/linearization contradiction split into
  pre-fence vs. post-fence rows, and a new two-UID denial row). Both SHA and
  SSA independently reviewed this exact head and both again returned
  CHANGES_REQUESTED: SHA found R4-1 (P1, the new high-water mark was itself
  a request-derived, replayable value — equal-mark replay after pruning
  still passed) and R4-2 (P2, the single negative control was impossible
  under the mutex fence variant), plus the SHA-revision2-attribution
  history-note correction. SSA found R4-1 (P1, the generation fence
  serializes successful publication, not a dirty/failed re-sync, leaving a
  source-edited-but-unpublished revocation unenforced), R4-2 (P1, the same
  equal-mark replay-after-pruning gap SHA found, independently derived),
  and R4-3 (P2, the same mutex/CAS negative-control contradiction SHA
  found), plus implementation elaborations on parent-directory/ownership
  hardening and the `authorized_keys` self-restoration scoping.
- Revision 5 (head `7449476b`): responds to all P1/P2 items from both of
  revision 4's reviews — see inline "(closes SHA R4-N / SSA R4-N)" markers
  above. Specifically: replaced the request-derived high-water mark's
  rollback check with a daemon-maintained wall-clock ratchet, persisted
  independently of any individual request and checked at every admission
  (not startup only), plus a restored minimum replay-retention floor tying
  pruning eligibility to `TTL + skew` past each row's `expires_at`; added an
  explicit `dirty` policy state under the same generation guard that
  pre-dispatch revalidation denies against unconditionally, closing the gap
  where a failed or interrupted re-sync left a source-edited revocation
  unenforced behind the freshness-ceiling fallback; split the single fence
  negative control into mutex-variant and CAS-variant cases, each asserting
  the outcome that actually follows from which operation commits first;
  and folded in the remaining editorial corrections (SHA's revision-2
  attribution history note, the generic different-UID matrix row wording,
  the `authorized_keys` self-restoration scope, and parent-directory/
  ownership hardening for the policy-state files). Remains **Proposed**
  pending a fresh SHA+SSA pass on this exact text; no merge, installation,
  or implementation is authorized by this revision.
- Revision 6 (head `5b4e7de3`): responds to SSA's
  CHANGES_REQUESTED on revision 5 (exact head `7449476b`) — see inline
  "(closes SSA R5-N)" markers above. SHA's parallel revision-5 review
  returned BLOCKED on source access (not a verdict on the text); a
  filesystem git-bundle handoff was supplied per that item and SHA's review
  of this revision is still pending. SSA found R5-1 (P1, the `dirty` flag
  depends on the async sync step having *observed* a source change — a
  request reaching pre-dispatch revalidation before that observation runs
  still reads `dirty=false` and dispatches on a source already changed);
  R5-2 (P1, three related gaps in the wall-clock ratchet: a new-principal
  exemption from the global ratchet check, a rollback check run at
  admission/startup but not rechecked at pre-dispatch, and unspecified
  pruning/override interaction with the ratchet); and R5-3 (P2, qualification
  wording for the last-known-good fallback not yet aligned with the
  revision-5 dirty rule in two places, plus two narrower matrix-wording
  fixes). This revision: replaces reliance on the async `dirty` observation
  for the *dispatch* decision with a synchronous stat (mtime+size) of the
  root-owned source files taken at pre-dispatch time itself, under the same
  guard, immediately before the Executor invocation — closing the
  observation-lag window by removing the wait on an independent process's
  schedule from the dispatch critical path; removes the new-principal
  ratchet exemption (the ratchet is global, not per-fingerprint); adds the
  identical ratchet recheck to pre-dispatch revalidation; requires pruning
  to persist its retirement floor in the same transaction as the row
  deletion it depends on; defines an owner-gated rollback override as
  minting a new protected epoch that invalidates all prior-epoch envelopes,
  rather than resetting the ratchet backward; and aligns the last-known-good
  fallback wording across §1's prose, its test-coverage list, and the
  qualification matrix, plus the two narrower matrix-wording corrections
  (UID-mismatch scope, "commits first" vs. `completed`). Remains
  **Proposed** pending a fresh SHA+SSA pass on this exact text; no merge,
  installation, or implementation is authorized by this revision.
- Revision 7 (head `68dc688c`): responds to SSA's
  CHANGES_REQUESTED on revision 6 (exact head `5b4e7de3`) — see inline
  "(closes SSA R6-N)" markers above. SHA's parallel revision-6 review
  returned BLOCKED — not on source content, but on bundle delivery to its
  worker host (a different physical machine from the one cutting the
  bundle); ra has no filesystem or network path onto that host, so this
  blocker is escalated to the owner rather than re-attempted with another
  bundle. SSA found R6-1 (P1, the synchronous pre-dispatch stat compared
  `mtime`+`size`, not content, and ran under a guard that fenced the sync
  publisher and Executor but not an account or root editing the source
  files directly — no distinct controlled commitment was defined); R6-2
  (P1, the seven-field envelope carried no signed epoch, so "reject an
  envelope whose admission epoch is older than current" had nothing
  authenticated to check, and minting an override epoch did not specify
  how new post-override requests resume admission without reopening the
  retired window); R6-3 (P1, the pruning transaction advanced the
  retirement floor to the deleted row's own `expires_at` rather than a
  wall-clock observation taken at prune time, leaving a crash/restart
  window where both the ratchet and the replay-row check pass); and R6-4
  (P2, the §1 active fallback prose — not the already-aligned test-list or
  matrix — still lacked the unchanged-source qualifier, plus a stale
  revision-5 "(this text)" self-label). This revision: replaces the
  mtime+size stat with a synchronous, in-guard invocation of the same
  content-hash validate function the async sync step uses, covering both
  the root-owned grant source and the account-writable restriction source
  identically, making that call itself the commit boundary; adds `epoch`
  as an eighth, signed envelope field checked at both admission and
  pre-dispatch against the daemon's actual current epoch, and defines an
  override as minting the next epoch **and** re-anchoring the wall-clock
  ratchet to real time in the same guarded transaction, so epoch-bound
  fresh requests resume while every prior-epoch envelope stays permanently
  rejected; changes the pruning floor formula to
  `max(current floor, trusted wall-clock observation at prune time)`
  instead of the deleted row's own `expires_at`, with a recorded
  countermodel; and adds the missing qualifier plus the history-label
  correction. Remains **Proposed** pending a fresh SHA+SSA pass on this
  exact text; no merge, installation, or implementation is authorized by
  this revision.
- Revision 8 (head `9054b085`): responds to SSA's
  CHANGES_REQUESTED on revision 7 (exact head `68dc688c`) — see inline
  "(closes SSA R7-N)" markers above. SSA found R7-1 (P1, the raw-edit
  commitment boundary asserted both that the synchronous validate call is
  authoritative and that "no write-side process is left unfenced," which
  is false for a raw writer — a root process editing `anchor-grants.allow`
  directly, or an account editing its own `authorized_keys` — that never
  acquires the guard the validate call runs under; a validated snapshot
  and the live file content were conflated as the same commitment) and
  R7-2 (P2, the rollback override's new-epoch forward progress was
  contradicted by the retained global admission-order high-water mark,
  which could deny a fresh, correctly-epoched, post-override request
  whose `issued_at` fell below a mark set before the override). This
  revision: redefines `anchor-grants.allow` and `authorized_keys` as
  always-**proposed** policy, with only the most recent validate call's
  content-hash-and-generation snapshot, read under the shared guard,
  serving as **active** authority for dispatch — a raw edit landing after
  that snapshot was read is the next proposed state, not an
  already-occurred revocation, and is committed or rejected only by a
  later validate call; drops the false "no write-side process left
  unfenced" and "never act on a source state that has already changed"
  claims; adds an exposed commit-sequence/hash state so an operator or
  account can observe when a specific edit actually became active, rather
  than inferring commitment from write completion; and scopes the
  admission-order high-water mark into the protected-epoch domain, so an
  owner-gated override initializes the new epoch's own mark from the
  daemon's current wall-clock observation while the retired epoch's mark
  is retained historically and never consulted for a current-epoch
  admission. Remains **Proposed** pending a fresh SHA+SSA pass on this
  exact text; no merge, installation, or implementation is authorized by
  this revision.
- Revision 9 (this text, head after `9054b085`): responds to SSA's
  CHANGES_REQUESTED on revision 8 (exact head `9054b085`) — see inline
  "(closes SSA R8-N)" markers above. SSA found R8-1 (P1, the raw-edit
  section still asserted an impossible mutual-exclusion invariant over
  writers that never hold the guard, restating rather than removing R7-1's
  defect; the acknowledgement query conflated a validation observation
  with an active commitment) and R8-2 (P2, "the ratchet is never reset
  backward" contradicted the override's own re-anchor from a false-future
  reading to real time; a test/matrix row wrongly attributed an
  ordering-floor denial to the ratchet; one matrix row described an
  impossible dispatch-with-no-validate-call schedule). This revision:
  removes the mutual-exclusion claim — raw writers may write at any time,
  including mid-validate-read, since the guard never serializes against
  them; redefines the exposed acknowledgement state to report the active
  published snapshot separately from dirty/invalid/observed state, so a
  non-committing validation observation never advances the reported
  active hash; makes the ratchet epoch-scoped like the mark, so an
  override's new epoch starts its own ratchet from a fresh observation
  (which may be numerically lower than the retired epoch's unmutated
  ratchet) without claiming one value is reset backward; re-attributes
  the `issued_at`-based positive control to the ordering-floor/mark check
  and adds a genuine, envelope-independent ratchet rollback control; and
  replaces the impossible matrix row with the actual rule that every
  dispatch performs its own mandatory synchronous validate call against
  the current source. Remains **Proposed** pending a fresh SHA+SSA pass
  on this exact text; no merge, installation, or implementation is
  authorized by this revision.

## References
- Ledger: `ra/rs-41-ownerless-recovery-signed-lan-anchor`; owner direction SHA
  20260915-012036.
- `docs/continuations/ra-router-stall-gate-spool-20260915-6509a1af.md` —
  prior session's framing of Option A/B and the lane list.
- `docs/ADR-075-DESKTOP-RECOVERY-ISSUER-INGRESS.md` — sibling interactive
  recovery path; §6 (audit scope) and §8 (anchor independence) reused here.
- `docs/ADR-045` / `docs/ADR-046` — existing local-LLM broker ownership and
  recovery-inversion precedent; `restart-broker` custody check (Decision
  item 3) must defer to these, not override them.
- Apple, "DFU restore a Mac" — https://support.apple.com/en-us/108900 (read
  2026-10-09); source for Option B's destructive-restore procedure.
- OpenBSD, `ssh-keygen(1)` (ALLOWED SIGNERS, `-Y sign`/`-Y verify`, `-n`
  namespace) — https://man.openbsd.org/ssh-keygen; `sshd(8)`
  (`authorized_keys` `from=`/`command=` restrictions) —
  https://man.openbsd.org/sshd (both read 2026-10-09). Source for item 1's
  allowed-signers/authorized_keys distinction, namespace binding, and the
  restriction-inheritance rule in the derived trust mapping — cited by SSA
  in the revision-2 review this revision responds to.
- PANTHEON_RULES.md A1 (Safety First), A3 (fixed auditable command set), A32
  (load-bearing recognition by pidfile), A35 (scope the check to the claim).
