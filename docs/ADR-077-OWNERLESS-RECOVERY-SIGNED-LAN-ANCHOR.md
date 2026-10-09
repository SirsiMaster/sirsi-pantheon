# ADR-077: Ownerless Recovery — Signed LAN Anchor

## Status
**Proposed** — 2026-10-09. Design only; no code, no key material, no new
`authorized_keys` entries. Routed for SHA (hardware) + SSA (software) review
before any implementation, per the owner directive that created this task
(SHA 20260915-012036, ledger `rs-41-ownerless-recovery-signed-lan-anchor`).
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
1. A system-domain service identity (a launchd daemon, not a user login
   session) listens behind **EXISTING authenticated LAN SSH** on each node.
   No new port, no new auth mechanism, no new credential type: the anchor
   rides the SSH session the owner's already-authorized key already opens.
   This is explicitly not permission to mint or broaden credentials — minting
   a new key or enrolling a new `authorized_keys` entry stays an owner gate
   (already tracked: `signing-escrow-loose-keys-m5`-adjacent M5 SSH key
   enrollment, SHA 212138).
2. The anchor exposes an **allowlisted, fixed table of recovery verbs** —
   e.g. restart-broker, remount-volume, re-arm-supervisor, fetch-known-good-config
   — never a general shell and never argv passthrough. Growing the allowlist
   into "run this string" is the same hazard Rule A3 names for `sirsi-agent`
   on untrusted targets, restated here for the owner's own nodes: a fixed,
   auditable command set, nothing else.
3. **Every admission is recorded** — which lane, which verb, when — to a
   durable local audit log (same posture as ADR-075 §6: identity and decision
   only, never capability or payload contents).
4. The anchor **must not weaken** SSH, SIP, FileVault, Tailscale, or Keychain.
   No new bypass, no relaxed host-key checking, no disabling of an existing
   control to make the anchor easier to reach.
5. **Lanes are reported separately, never pooled.** The proven lanes are M1
   LAN SSH, M5 LAN SSH, and three Thunderbolt bridge rails. Per A35 (scope the
   check to the claim) and ADR-075 §8's same point about anchor independence:
   two of these sharing a router, a power strip, or a single issuer are not
   two independent lanes, and the anchor's status report to Pantheon/Ra must
   say so per-lane rather than rolling them into one "recovery: OK".
6. The anchor is **load-bearing infrastructure** once installed (A32): it
   must be recognized by pidfile, not by process name, in any future
   kill/renice/reaper path, the same way the Gemma broker is — a generic
   "largest RSS" or "unknown process" sweep must never treat it as
   expendable.

**Option B (documented, not built now):** a second trusted Mac with Apple
Configurator or DFU restore is the physical out-of-band floor. It requires a
person and a second machine on-site, so it is not "ownerless" — it is the
honest last resort when every lane in Option A is simultaneously unreachable
(e.g. the whole LAN is down, not just one path on it).

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
  the verb set is small, fixed, and audited, so the blast radius of a
  compromised lane is bounded by the allowlist, not by "whatever SSH can do."
- **Negative**: the anchor is one more always-on, launchd-resident service
  per node that must survive reboots and be protected from being treated as
  an expendable process (A32) — it is new infrastructure to maintain, not a
  config change.
- **Risk**: allowlist creep is the primary risk (a "safe" verb table growing
  argv passthrough one convenience patch at a time); mitigated by requiring
  SHA+SSA review of any verb-table change, not just the initial design.
  Secondary risk: a lane-independence claim that is false in practice (e.g.
  two "separate" TB rails sharing a hub) reads as a clean board while one
  real lane exists — the per-lane reporting in Decision §5 exists specifically
  to make that visible rather than averaged away.

## References
- Ledger: `ra/rs-41-ownerless-recovery-signed-lan-anchor`; owner direction SHA
  20260915-012036.
- `docs/continuations/ra-router-stall-gate-spool-20260915-6509a1af.md` —
  prior session's framing of Option A/B and the lane list.
- `docs/ADR-075-DESKTOP-RECOVERY-ISSUER-INGRESS.md` — sibling interactive
  recovery path; §6 (audit scope) and §8 (anchor independence) reused here.
- PANTHEON_RULES.md A1 (Safety First), A3 (fixed auditable command set), A32
  (load-bearing recognition by pidfile), A35 (scope the check to the claim).
