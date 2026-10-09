# ADR-077: Ownerless Recovery — Signed LAN Anchor

## Status
**Proposed, revision 2** — 2026-10-09. Design only; no code, no key material,
no new `authorized_keys` entries. Routed for SHA (hardware) + SSA (software)
review before any implementation, per the owner directive that created this
task (SHA 20260915-012036, ledger `rs-41-ownerless-recovery-signed-lan-anchor`).
Revision 2 responds to SSA's and SHA's CHANGES_REQUESTED verdicts on revision
1 (exact head `e22478e1`) — see "Review History" below. Both reviews named
the same four gaps
independently: an undefined signing/admission contract, an overstated SSH
blast-radius claim, unbound verb custody, and an availability claim ahead of
its evidence. This revision closes all four; it remains proposed, not
accepted, pending a fresh SHA+SSA pass on this exact text.
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

**Option A (built here):**

1. **What "signed" means (closes SSA-1 / SHA-1).** SSH authenticates a
   *transport and account*; it does not by itself authenticate a *request*.
   This ADR defines "signed" as a separate property layered on top of the
   existing SSH session, not a synonym for it:
   - Every recovery request is a bounded envelope —
     `{node_id, verb, target, config_digest, request_id, issued_at, expires_at}`
     — serialized and signed with the SAME key material the owner's SSH
     session already authenticates with (the client's existing SSH keypair,
     signature produced via `ssh-keygen -Y sign` / agent signature, verified
     server-side with `ssh-keygen -Y verify` against the same
     `authorized_keys` entry already trusted for that account). **No new
     signing credential, no new enrollment, no new key type** — the signer
     is the principal already authorized to open that SSH session.
   - The anchor daemon is the **verifier**: it checks the signature against
     the connecting account's known public key, checks `expires_at` against
     wall clock, and checks `request_id` against a local replay cache that
     survives restart (persisted alongside the audit log — item 4 below). A
     request that fails any of those three checks is denied and logged; it
     never reaches the verb dispatcher.
   - This gives the design a real, falsifiable meaning for "signed" — a
     forged or replayed request is rejected even by someone who has
     captured a live SSH session's output, because the signature is bound
     to `request_id` + `expires_at` + the specific verb/target/digest, not
     to the transport. If a future revision drops this envelope and relies
     on SSH authentication alone, it must say so explicitly and drop
     "signed" from the title.
   - This does **not** change how an automated caller (a cloud agent, a
     supervisor) invokes the anchor: it uses the account's existing
     SSH key and existing `ssh-agent` (or an on-disk key it is already
     authorized to use) to produce the request signature — no interactive
     unlock, no owner login credential borrowed, no new secret issued to
     the caller.

2. **Local privilege boundary (closes SSA-2 / SHA-1-second-half).** The
   anchor is a system-domain launchd daemon listening on a **local-only,
   filesystem-permissioned IPC socket** (not a TCP port), reachable only
   from the same host's already-authenticated SSH session (the SSH session
   forks/execs the signed-request submission step as the authenticated
   user; the daemon trusts the kernel's peer-credential check on that
   socket, not the network). Authorization is **per-principal, per-verb**:
   the daemon holds a static allowlist mapping `(ssh_pubkey_fingerprint) →
   {verbs permitted}`, so a given SSH account's compromise is bounded by
   that account's verb entry, never by "whatever that account's shell could
   do." **This explicitly narrows, and replaces, the prior "compromised-lane
   blast radius is bounded by the allowlist" claim**: that claim is true
   only for actions routed through this daemon. It does **not** reduce the
   residual risk already carried by an unrestricted SSH account on that
   host — a compromised SSH key still has whatever rights that account's
   shell already has, independent of this ADR. This ADR bounds the
   *anchor's own* attack surface; it makes no claim about the pre-existing
   SSH account's blast radius.

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
   specifically must check ADR-045/ADR-046's existing ownership state for
   the target broker before acting — if that broker is marked
   owner-managed or mid-operation by ADR-045/046's own bookkeeping, the
   anchor denies the request rather than silently overriding it.
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
  │  with its existing, already-authorized SSH key
  ▼
EXISTING authenticated LAN SSH session to target node
  │  (no new port, no new auth mechanism)
  ▼
local-only IPC socket, peer-credential checked (kernel-enforced, same host only)
  ▼
anchor daemon — Verifier
  ├─ signature invalid / expired / request_id replayed → DENY, log, stop
  ▼ (signature valid, fresh, not replayed)
anchor daemon — Authorizer
  ├─ (pubkey_fingerprint, verb) not in allowlist → DENY, log, stop
  ▼ (principal permitted for this verb)
anchor daemon — Audit writer
  ├─ durable write fails (fsync error / disk full) → DENY, log attempt, stop
  ▼ (audit record durably persisted as `pending`)
anchor daemon — Custody check (verb-specific)
  ├─ target owned/mid-operation per ADR-045/046, or config source/digest
  │  unapproved, or path/symlink caller-writable → DENY, mark audit `denied`
  ▼ (custody clear)
Executor — runs the ONE fixed, root-owned absolute executable bound to this verb
  ▼
Post-action verification — re-checks service/volume state
  ▼
Audit writer — resolves `pending` → `completed` | `failed` | `unknown-reconcile-required`
  ▼
Status report to Pantheon/Ra — per-lane, per-node (never pooled; §Decision-6)
```
Every arrow left of "Executor" can terminate in DENY; only a request that
clears signature, allowlist, audit-durability, and custody checks, in that
order, reaches the single fixed executable for its verb.

## Qualification Matrix (closes SSA-4 / SHA-5)
Before any lane or verb is reported as "proven" rather than "candidate,"
each row below must have a reviewed, dated test result on file:

| Test case | Expected result |
| :--- | :--- |
| Positive recovery (valid signed request, authorized verb, clear custody) | `completed`, post-action verification confirms state |
| Denied principal (valid signature, verb not in that principal's allowlist) | `denied`, no execution, audit entry written |
| Denied verb target (valid principal, unapproved target/path/digest) | `denied` at custody check, no execution |
| Replayed request (same `request_id` resubmitted within window) | No-op per idempotency rule, not a second execution |
| Expired request (`expires_at` passed) | `denied` at Verifier, before allowlist check |
| Revoked authority (principal's `authorized_keys` entry removed mid-window) | `denied`, existing in-flight request is NOT grandfathered |
| Audit store unavailable (disk full / fsync failure) | `denied` fail-closed, no execution, attempt logged where possible |
| Protected/owned workload (ADR-045/046 marks target owner-managed) | `denied` at custody check |
| Concurrent requests to same node | Serialized; second request waits or is rejected, never interleaved |
| Interrupted recovery (daemon restarts mid-verb) | Audit resolves from `pending` to `unknown-reconcile-required`, never silently `completed` |
| Per-lane independence (e.g. two TB rails on one hub) | Status report shows shared dependency, not two independent "OK" lanes |
| Dead `sshd`/network on target | Lane reports `unreachable`, distinct from `denied` — never conflated |

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
- Revision 2 (this text): responds to all P1/P2 items from both reviews —
  see inline "(closes SSA-N / SHA-N)" markers above. Remains **Proposed**
  pending a fresh SHA+SSA pass on this exact text; no merge, installation,
  or implementation is authorized by this revision.

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
- PANTHEON_RULES.md A1 (Safety First), A3 (fixed auditable command set), A32
  (load-bearing recognition by pidfile), A35 (scope the check to the claim).
