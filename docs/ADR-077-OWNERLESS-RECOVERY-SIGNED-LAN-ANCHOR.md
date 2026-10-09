# ADR-077: Ownerless Recovery — Signed LAN Anchor

## Status
**Proposed, revision 4** — 2026-10-09. Design only; no code, no key material,
no new `authorized_keys` entries. Routed for SHA (hardware) + SSA (software)
review before any implementation, per the owner directive that created this
task (SHA 20260915-012036, ledger `rs-41-ownerless-recovery-signed-lan-anchor`).
Revision 2 responded to SSA's and SHA's CHANGES_REQUESTED verdicts on
revision 1 (exact head `e22478e1`); revision 3 responded to SSA's
CHANGES_REQUESTED verdict on revision 2 (exact head `c5680351`). Both SHA and
SSA independently reviewed revision 3 (exact head `1087bf15`) and both
returned CHANGES_REQUESTED — see "Review History" below. The two reviews
converged on the same underlying gap stated two ways: the authority a
revocation changes and the authority a dispatch reads are not the same
protected, versioned thing, so "revoke before invoke denies" was asserted
without a mechanism that makes it true. This revision (a) makes the grant
source root-owned and distinct from account-writable `authorized_keys`,
(b) replaces the "re-read before dispatch" prose with an explicit generation
fence shared between policy publication and dispatch commitment,
(c) makes the clock-rollback replay defense survive record pruning via a
durable high-water mark, (d) fixes the reservation key to
`UNIQUE(fingerprint, request_id)` with content hash as a checked value, and
(e) corrects three qualification-matrix rows that contradicted the
linearization-point rule and the Verifier/Custody stage split stated
elsewhere in this document. It remains proposed, not accepted, pending a
fresh SHA+SSA pass on this exact text.
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
     `{node_id, verb, target, config_digest, request_id, issued_at, expires_at}`
     with field types fixed, not left to the implementation to infer:
     `node_id` (string, this node's pinned identity), `verb` (string, one of
     the enumerated allowlist names — §item 3), `target` (string, the
     verb's approved service label or volume UUID), `config_digest` (string,
     lowercase-hex `sha256`, exactly 64 characters), `request_id` (string,
     UUIDv4 canonical form), `issued_at`/`expires_at` (integer, Unix seconds
     UTC — never a float, never a string, never a different unit). The
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
       or if the previous successful sync is older than a bounded
       **freshness ceiling**, the daemon serves the last-known-good mapping
       if still within ceiling, or **fails closed** (denies all requests)
       once the ceiling is exceeded — a stale or broken policy input must
       never silently keep granting.
     - A key present in `authorized_keys` with a `from=`/`command=`
       restriction and no `override: true` on its `anchor-grants.allow`
       line remains **absent from the derived mapping**: the restriction is
       inherited as exclusion, never as a narrowed-but-present grant.
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
     malformed or ambiguous input (must refuse to publish, serve
     last-known-good within the freshness ceiling, then fail closed); the
     freshness ceiling exceeded with no successful sync (must deny all
     requests, not serve a stale mapping indefinitely).

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
   - **Clock-rollback defense survives record pruning (closes SSA R3-2
     P1).** "A request whose `issued_at` predates the oldest retained
     record" is not a durable watermark by itself: once completed replay
     rows age out and are pruned, an empty per-fingerprint set has no oldest
     record, and a rolled-back clock can make a previously-expired, pruned
     envelope satisfy TTL/skew again. This ADR instead requires a **durable
     high-water mark**, persisted independently of individual replay rows
     (per fingerprint, and a global value) — updated to `max(current value,
     this request's issued_at)` in the **same transaction** as each
     admission, so it survives pruning of the rows it was derived from. At
     daemon startup, the current wall clock is checked against the
     persisted global high-water mark minus the max permitted skew; if wall
     clock is behind that mark, the daemon treats this as a detected clock
     rollback and **fails closed** — denies all admissions — until the
     condition clears (clock resynchronized past the mark, or an explicit
     owner-gated override). A fingerprint with no prior high-water mark
     (new principal, never-pruned empty state) has no rollback floor yet and
     is evaluated on TTL/skew alone, same as today.
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
     (deny at the pre-dispatch revalidation); **a deterministic negative
     control that pauses the daemon after the pre-dispatch fence read,
     commits a revocation of the signing key's generation, then resumes**
     — the paused request must deny, never complete, proving the fence
     (not timing luck) is what closes the gap; all replay rows for a
     fingerprint pruned, then the daemon's wall clock rolled back, then an
     old (previously valid, now-expired-by-real-time) envelope resubmitted
     (must deny via the persisted high-water mark, not re-admit).

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
  │  signs {node_id, verb, target, config_digest, request_id, issued_at, expires_at}
  │  under namespace "sirsi-recovery-anchor" with its existing SSH key
  │  (typically from an existing SSH session; item 1 permits any local
  │   process running as the same mapped UID — ingress is UID+signature,
  │   not "arrived over SSH")
  ▼
local-only IPC socket, peer-credential checked (kernel-enforced UID only,
same host, never network-reachable)
  ▼
anchor daemon — Verifier
  ├─ wrong namespace / unenumerated key format → DENY, log, stop
  ├─ signature does not verify against derived allowed-signers mapping → DENY, log, stop
  ├─ node_id ≠ this node, or target/config_digest ≠ verb's approved identity → DENY, log, stop
  ├─ issued_at/expires_at outside server-bound TTL+skew → DENY, log, stop
  ▼ (signature valid, scoped to this node/verb, within TTL)
anchor daemon — Atomic admission
  ├─ UNIQUE(fingerprint, request_id) already reserved with a DIFFERENT
  │  content hash → DENY, log, stop
  ├─ same content hash already reserved for that key → return existing
  │  outcome (no-op)
  ├─ issued_at behind the fingerprint's/global persisted high-water mark →
  │  DENY, log, stop (clock-rollback defense, survives replay-row pruning)
  ▼ (fresh reservation + `pending` audit write + high-water-mark update,
     same transaction, node-serialized)
anchor daemon — Authorizer
  ├─ (pubkey_fingerprint, verb) not in current allowlist → DENY, mark audit `denied`
  ▼ (principal permitted for this verb)
anchor daemon — Custody fence acquire (verb-specific, via ADR-045/046)
  ├─ no compatible fenced-acquire, fence already held by another owner, or
  │  config source/digest unapproved, or path/symlink caller-writable
  │  → DENY, mark audit `denied`, release nothing (never acquired)
  ▼ (fence held)
anchor daemon — Pre-dispatch revalidation, under the shared generation fence
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
| Different UID presents a validly-signed envelope | `denied` — UID not present in the derived mapping |
| Presenting UID holds *some* valid anchor grant under a different fingerprint, but differs from the mapped UID of the fingerprint that actually signed this envelope | `denied` — holding any grant is not sufficient; the presenting UID must equal the signing fingerprint's own mapped UID |
| Two keys enrolled on one UID | Both succeed independently, each bound to its own mapped permitted-verb set |
| Local same-UID invocation with no SSH session involved | Succeeds identically to an SSH-forked invocation |
| Key in `authorized_keys` with `from=`/`command=` restriction, no `override: true` on its `anchor-grants.allow` line | Absent from derived mapping; `denied` as unknown key |
| Key added to `authorized_keys` by the account itself, no matching `anchor-grants.allow` entry | Absent from derived mapping; `denied` as unknown key — self-enrollment via `authorized_keys` alone is impossible |
| Privileged sync step given a malformed/ambiguous `anchor-grants.allow` or `authorized_keys` input | Refuses to publish; serves last-known-good within freshness ceiling, then fails closed past the ceiling |
| Two requests, same `request_id`, different content, submitted concurrently | One wins admission under `UNIQUE(fingerprint, request_id)`; the other fails — never both execute |
| Same `request_id` resubmitted after daemon restart | Resolves from persisted replay record, not re-admitted as new |
| Request `node_id` names a different node | `denied` at Verifier |
| Request claims a TTL longer than the server-bound maximum | `denied` at Verifier |
| Queued request whose `expires_at` passes before dispatch | `denied` at pre-dispatch revalidation, not at admission |
| Signing key revoked while its request is queued between admission and dispatch | `denied` at pre-dispatch revalidation |
| Negative control: daemon paused immediately after the pre-dispatch fence read; revocation committed under the generation fence; daemon resumed | Paused request `denied` — the fence, not timing, must be what closes the gap |
| All replay rows for a fingerprint pruned, daemon clock rolled back, previously-valid-but-now-expired envelope resubmitted | `denied` via the persisted high-water mark, not re-admitted as new |
| Competing non-anchor owner acquires target lock between custody inspection and dispatch | Fenced-acquire fails; request `denied`, not proceeded on stale state |

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
  this same head returned BLOCKED, no verdict: the exact commit had not yet
  been pushed to `origin`, so SHA's independent fetch could not reach it
  (A37 — a record exists only on origin). That gap is closed by this
  revision's push; it does not by itself constitute SHA acceptance of
  anything.
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
- Revision 4 (this text): responds to all P1/P2 items from both of
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
  pre-fence vs. post-fence rows, and a new two-UID denial row). Remains
  **Proposed** pending a fresh SHA+SSA pass on this exact text; no merge,
  installation, or implementation is authorized by this revision.

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
