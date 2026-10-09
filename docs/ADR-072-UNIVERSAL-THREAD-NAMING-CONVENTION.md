# ADR-072 — Universal Thread Naming Convention, Router-Enforced

**Status:** Proposed 2026-09-28 (Ra design; owner-directed). SSA round 1
(2026-10-09, thr-80b0f440844c2424): **changes requested** on C2/C4/C5 plus phase
order — see the revised C2/C4/C5 and §6 "Phase order" below. Re-review + owner
bind still pending.
**Deciders:** owner (directive 2026-09-28), Ra (router architect)
**Custodian:** 𓁢 the Router (registry authority)

## Context

Router identity is a mess, and it costs hours every day. In one session alone the
fabric carried, for a single logical lane, all of: bare `claude-finalwishes`,
`claude-finalwishes-m1`, `claude-finalwishes-m5`, `claude-finalwishes-helper`, and
`claude-fw` — plus `mercury` vs `mercury-m5` vs `claude-io`, and `codex-inference` vs
`codex-apollo`. Mail stranded on retired ids that nothing could pull; sends failed
with "identity not fully declared"; the same name meant different things on M1 and
M5; a shared checkout on the wrong branch resolved identity wrongly.

Root causes:
1. **No grammar.** Names were free-form, so a lane, a machine seat, and a task all
   collided in one string with no structure the router could parse or enforce.
2. **Lanes self-named.** A session picked its own id; the router accepted whatever
   string arrived, so `helper`, `-m5`, `-cylton` all coexisted for one lane.
3. **Name conflated with thread-id and machine.** There was no stable hierarchy
   binding a human name to the durable thread-id and the machine it runs on, so a
   rename or a machine move produced a new, unrelated id and stranded the old one.
4. **Identity resolved off a shared working tree** (the branch a shared checkout
   happened to be on), not off an authoritative registry.

Owner directive (2026-09-28): *"I need 1 universal naming convention for every
thread… `<Agent name>-<project name>-<machine name>-[optional task name]`… make the
router demand the threads present in this way… thread id, machine id and task id be
maintained behind that named convention. 1 name per thread id, but a thread id can
be renamed and a name can be reused on a new thread if the thread is retired. The
router has to know and understand the hierarchy at all times and enforce it."*

## Decision

Every router thread has exactly one **name** in a single grammar, the router is the
sole **name authority** (it hands out conformant names; lanes never self-name into
non-conformance), and the router maintains and enforces the **hierarchy behind the
name** at all times.

### 1. Name grammar

```
<agent>-<project>-<machine>[-<task>]
```

- Lowercase, hyphen-delimited. **Grammar (SSA #1, resolved):** the first three
  slots — agent, project, machine — are each `[a-z0-9]+` (no internal hyphens; the
  hyphen is their delimiter). The **task is the remainder after the third hyphen**
  and MAY contain hyphens: `[a-z0-9]+(-[a-z0-9]+)*`. The validator splits on the
  first three hyphens only, so `claude-finalwishes-m1-fw-r02` parses as
  agent=`claude`, project=`finalwishes`, machine=`m1`, task=`fw-r02`, and
  `mercury-mercury-m5-mercury-releases` → task=`mercury-releases`. This one encoding is
  applied identically in the grammar, examples, migration records, and validators.
- **agent** — the agent/lane identity family: `claude`, `codex`, `ra`, `mercury`,
  `gemma`, `sirsi` (governance/admin), … A fixed, router-known set.
- **project** — the product or domain the thread serves. **Every** thread carries
  one, including infrastructure lanes (owner decision 2026-09-28): `router`,
  `finalwishes`, `pantheon`, `mercury`, `photon`, `apollo`, `nexus`, `deck`,
  `home` (fabric/overseer, e.g. claude-home), `governance` (SSA), `maat`, …
- **machine** — the host, **gleaned by the router from the machine's designated
  name** (owner decision 2026-09-28): the hostname prefix before the first `.`,
  lowercased. `M1.local` → `m1`, `M5.local` → `m5`; future machines self-name on
  launch and their prefix becomes the slot. The name slot is bound to the durable
  **machine-id** (ADR-067) behind it, and the router enforces they agree.
- **task** — optional. A specific workstream when one lane runs parallel tasks
  (`fw-r02`, `mercury-releases`). Absent for a lane's single default thread.

Examples: `ra-router-m1`, `claude-home-m1`, `sirsi-governance-m5`,
`codex-finalwishes-m5`, `claude-finalwishes-m1-fw-r02`, `mercury-photon-m5`.

### 2. The hierarchy behind the name (router-maintained)

```
name  ─1:1─>  thread-id (durable thr-…)  ──>  machine-id (credentialed, ADR-067)
                                          └─>  task-id (workstream, optional)
```

- **name → thread-id** is 1:1 for **live** threads: exactly one live thread per name,
  exactly one name per live thread.
- **thread-id** is the durable key (never changes on rename); the name is a mutable
  label on it.
- **machine-id** is the ADR-067 credentialed identity; the name's `machine` slot is
  the friendly alias and MUST resolve to it.
- **task-id** binds the optional task slot to the thread's current workstream.

The router stores this mapping and is the only writer of it.

### 3. Router as name authority (enforcement model — owner: "hand out a conformant name")

Registration does not accept a free-form name. A thread presents its **components**
`{agent, project, task?}`; the router **gleans machine** from the host, **validates**
the agent/project against its known sets, **constructs** the canonical name, **binds**
it to the thread-id and the credentialed machine-id, and **returns** the assigned
name. A lane cannot register a non-conformant name because it never supplies the
name — the router builds it. This replaces free-string `sirsi thread register <id>`
with a component-based register that hands back the canonical id.

The router **refuses**:
- an agent or project outside its known sets (extending the set is a registry change,
  not a self-service field);
- a machine slot that does not match the gleaned hostname / credentialed machine-id
  (no cross-host spoofing — extends ADR-067);
- a second live thread claiming a name already bound to a live thread (1:1 invariant);
- a rename target that is any of the above.

### 4. Lifecycle: rename and name reuse

- **Rename** changes the name on the **same thread-id** (e.g. add/remove a task slot,
  or a project re-scope). The router reassigns the label, preserves the thread-id and
  its mail/ledger, and updates the mapping atomically. `sirsi thread rename` (new
  verb) is the only supported path; mail is never stranded because the thread-id is
  unchanged.
- **Retirement** (`sirsi thread close`) frees the name. A retired name may be **reused
  on a NEW thread-id** later. The router allows reuse only after the prior holder is
  retired (never two live holders).
- **No orphaning.** Because names map to durable thread-ids and rename preserves the
  thread-id, the manual "migrate mail off a retired id" cleanups this ADR is born
  from stop happening. Where a lane genuinely retires and its successor differs, the
  supported reassign verb (`ra/reassign-verb`) moves any residual mail; the router
  never leaves mail addressed to a name with no live thread unsurfaced.

### 5. Migration (existing fabric → convention)

The router publishes a migration map; each live thread renames once (thread-id
preserved). Illustrative:

| current id | → canonical name |
|---|---|
| `ra` | `ra-router-m1` |
| `claude-home` (both machines) | `claude-home-m1` + `claude-home-m5` (agent=claude, project=home; dev-root seats) |
| `sirsi-software-admin` | `sirsi-governance-m5` |
| `claude-finalwishes-m1` | `claude-finalwishes-m1` (already close; formalized) |
| `claude-finalwishes-m5` | `claude-finalwishes-m5` |
| `codex-apollo` | `codex-apollo-m5` |
| `mercury` / `mercury-m5` | `mercury-mercury-m1` / `mercury-mercury-m5` |
| `codex-pantheon` | `codex-pantheon-m1` |

Retired ids (`codex-inference`, bare `claude-finalwishes`, `claude-fw`,
`claude-finalwishes-helper`, `manual-pantheon`, `cylton-mercury`) are already drained
and stay retired; their names become reusable under the grammar.

## 6. Implementation constraints (SSA review conditions 2–6, 2026-09-28)

**C2 — Versioned, origin-pinned registry schema.** The allowed agent values,
project values, and machine aliases live in ONE versioned schema on origin at a
canonical path (`contracts/naming/registry-schema-vN.json`), carrying an explicit
`schema_version` and a content hash computed over the schema's own bytes with
`schema_hash` itself excluded from the hashed payload (sha256 over the
canonicalized JSON minus the `schema_hash` field — never a hash that includes
itself, which proves nothing). The hash alone grants no authority: it is
meaningful only alongside a **promotion receipt** — a signed/attributable
record, written by the router service at promotion time, binding
`{schema_version, schema_hash, promoted_by, promoted_at}` to the origin commit
that introduced it. A client-supplied pin (a hash a lane happens to hold) is
never trusted on its own; the router always re-derives trust from the
service's own promotion receipt, not from what the caller presents. Promotion
(adding an agent/project value or a machine alias) is an owner/SSA-bound change
on origin that **atomically** activates the new `(schema_version, schema_hash,
machine bindings)` triple together — never bumps the version without the hash,
or the hash without the machine bindings. The router **rejects** an
unsupported (unknown) schema version outright and **rejects** an
authenticated-but-stale version presented where a newer one has been promoted
(unauthorized downgrade), unless the caller is in the explicit offline
carve-out below.
  - **Offline carve-out:** a host with no reachable router service MAY operate
    against the **last promotion receipt it verified while last online**,
    under this explicit policy only — never a hash it invented or a
    working-tree file it edited. Missing or never-verified pin **fails
    closed** (registration refused, not silently allowed through).
  - **Working-tree divergence is never authority** (A37): a schema file edited
    in a checkout, with no matching promotion receipt, is detected as a hash
    mismatch and refused — the router's refusal is a **local operator
    diagnostic** ("your checkout disagrees with the last promoted schema"),
    distinct from the router **service's** rejection of a remote registration
    request against an unpromoted/stale schema. The service itself never
    depends on any client checkout to decide trust — only on its own
    promotion receipts.

**C3 — Machine proof from the authenticated session, not hostname text.** The
`machine` alias is not trusted from any client-supplied string. The router derives
the host from the **authenticated router session/service** and the **credentialed
machine-id** (ADR-067), then verifies the canonical alias maps to that machine-id.
Hostname text alone carries no authority; a session on m5 cannot register a `-m1`
name. The alias↔machine-id binding is part of the schema (C2). Any remote
service call the router itself makes during registration (schema promotion
lookup, machine-id verification) carries the router's own authenticated,
credentialed host context — never the service host's bare hostname and never
a hostname string supplied by the registering caller.

**C4 — Atomic mapping, crash-safe rename, recovery.** The mapping
`name → thread-id → machine-id → task-id` is written in a **single transaction**.
Invariants are enforced as store constraints, not application checks: a **partial
unique index on live rows** gives exactly one live name per thread and one live
thread per name. The `(machine-id, session)` constraint governs **ownership of a
live name claim**, not "one thread per session" — a session legitimately runs
several threads (parallel tasks), so the constraint is scoped to
`(machine-id, session, name)` uniqueness, never a blanket single-thread-per-session
rule; any tighter restriction is a separate, explicit lifecycle decision this
ADR does not make.

**Idempotency is a replay guard, not a concurrency guard — the two are separate
mechanisms and C4 requires both:**
- **Serialization (concurrency):** a rename reads the current mapping row with
  its **revision** (an integer or row version) and writes with a
  **compare-and-swap** on that revision inside the transaction. Two renames
  racing on the same thread are serialized by the store (row lock / CAS
  failure on the loser); the loser gets an explicit **stale-revision conflict**
  error, never a silent overwrite and never two successful "renamed" receipts
  for one thread's single rename.
- **Idempotency (replay):** each rename request carries an idempotency key.
  The key is recorded bound to `{authenticated caller, thread-id, operation,
  request-digest}` — a digest of the actual requested change, not just the key
  string. A retried request with the **same key and same digest** returns the
  original receipt as a no-op. A request with the **same key but a different
  digest** (a key reused for a different payload) is **rejected**, never
  applied as a new rename and never treated as a replay of the old one.
- **Receipt/mapping atomicity:** the idempotency receipt and the mapping update
  commit in the **same transaction**. If the process crashes after commit but
  before the reply reaches the caller, the retried request finds the committed
  receipt and returns it unchanged — the caller cannot distinguish a slow
  success from a crash-after-commit, by design.

**Migration** writes a **rollback/resume receipt** (idempotency key + prior
state) so an interrupted rename/migration resumes or rolls back cleanly; a
crash mid-rename never leaves a thread nameless or a name dangling. Collisions
are **rejected**, never silently overwritten.

**C5 — Alias draining during flag-then-refuse (no silent stranding), with
generation-fenced reuse.** Every bound name carries one of three states —
**canonical** (the live holder's current name), **active alias** (a prior
name of the *same still-live* thread-id, during its migration window), or
**tombstone** (a prior name of a now-*retired* thread-id, retained for
redirect, not for delivery) — and the router enforces a **single collision
policy** across all three: a name can be promoted from tombstone back to
canonical (reuse) only when no canonical or active-alias row currently holds
it. A retiring thread's name does not go straight to "free" — it becomes a
tombstone first, carrying the retired thread's **durable thread-id and
generation number** (incremented each time the name is reused on a new
thread), so a redirect can always name "which" dead thread the mail was meant
for, not just a label.

Reuse does **not** delete the tombstone; it **supersedes** it with a new
generation while keeping the superseded generation's identity (thread-id +
generation) in the record, so the structured response C5 promises stays
answerable after reuse, not just during the migration window:
- **Active alias → canonical (still live):** mail to the old name is
  delivered to the live thread-id — `{status: renamed, to: <new-name>}`.
- **Tombstone, not yet reused:** `{status: retired, successor?: <name>}`.
- **Tombstone, reused by a new holder (new generation):** mail addressed to
  the name **by thread-id/generation** (a sender who resolved an expected
  thread-id before sending) that no longer matches the current generation is
  **rejected as stale** — `{status: stale, expected_generation: N, current:
  M}` — never silently redirected into the new, unrelated holder's inbox. A
  **legacy name-only send** (no resolved generation attached, e.g. an old
  script) is **required to resolve an expected thread/generation before
  delivery is attempted**, or the send is rejected as ambiguous; the router
  never guesses which generation a bare name means once more than one has
  existed.
- The router **never auto-reroutes** a retired thread's queued-but-undelivered
  mail to whatever new holder later reuses its name — queued mail for a
  retired thread is surfaced to the retired thread's successor only via the
  explicit `ra/reassign-verb` path, never implicitly by name collision.

Required test coverage (folds into C6): rename A→B (A becomes active alias);
attempted reuse of A **while** A is still an active alias of the live thread
(must be refused — collision policy, not a race to win); retirement of A
(A becomes tombstone) and reuse of A after the window (new generation,
superseded tombstone kept); stale sender delivery (an old resolved
generation reference against a reused name is rejected, not redirected);
redirect delivery (unresolved legacy name-only send is rejected as
ambiguous, never guessed). Alias/tombstone rows are not time-bounded by a
calendar window alone — an active alias is scoped to "while the original
thread-id is still live"; a tombstone persists until explicitly superseded by
reuse, not deleted on a timer.

**C6 — Adversarial + divergence fixtures (required before the impl bind).** The
implementation ships fixtures that fail without the guard and pass with it (A35),
run against the actual store/service boundary with production constraints
enabled — each with a causal negative control that reaches the guarded
operation and fails without the guard, followed by a restored pass:
cross-host spoofing (a `-m1` name from an m5 session — refused via C3),
stale/working-tree registry divergence (schema-hash mismatch — refused via C2),
unsupported/downgraded schema version and revoked host credential (refused via
C2/C3), duplicate live claim (two sessions, one name — refused via C4's partial
unique index), rename race (concurrent renames of one thread — serialized by
C4's revision CAS, not just the idempotency key), same-key/same-payload retry
(no-op replay), same-key/different-payload retry (rejected), crash midway
through rename/migration at points both before and after commit
(resume/rollback via C4's receipt, same receipt returned post-crash),
atomic schema promotion racing a concurrent registration, duplicate
canonical/alias/tombstone collision (refused via C5's single collision
policy), retired-name reuse only after retirement with a new generation
(tombstone superseded, not deleted), stale generation delivery and ambiguous
legacy name-only send (both rejected per C5, never guessed or cross-delivered),
task-component binding rejection (an unauthorized/non-existent task — C2
phase-order note below), and migration retry/rollback that loses no mail,
lease, or history. Preserve exact source/schema/store versions, command
outputs, and failure evidence for each fixture; no fixture execution is
claimed by this ADR text itself — fixtures ship with the P2 implementation.

**Phase order (SSA round 1, resolved):** P2 — not P3/P5 — MUST land C2's
origin-pinned schema/promotion-receipt check and C4's crash-safe,
revision-guarded mapping **before registration is allowed to mutate state at
all**. P3 may expose the `rename`/reuse *commands* and P5 may finish
integration/cutover, but neither phase defers P2's trust (C2/C3) or storage
(C4) invariants — those are preconditions of the first successful
registration, not later hardening. The optional `task` component, when
present, MUST bind to an existing, authorized, durable task/workstream id
known to the router (not an arbitrary string the caller supplies) — the
namespec grammar check (P1) validates the *shape* of the task slot only and
grants no binding authority by itself.

## Neith's Triad (A22)

### Data Flow Architecture

```mermaid
flowchart TD
  L[Lane / session] -->|register {agent, project, task?}| R[Router name authority]
  H[Host designated name\nM1.local] -->|glean prefix -> m1| R
  MID[machine-id\ncredentialed ADR-067] -->|verify == machine slot| R
  R -->|construct <agent>-<project>-<machine>[-<task>]| NAME[canonical name]
  R -->|bind 1:1| MAP[(name -> thread-id -> machine-id -> task-id)]
  NAME -->|returned| L
  L2[Sender] -->|send --to <name>| R2[Router dispatch]
  R2 -->|resolve name -> live thread-id| MAP
  MAP -->|deliver| INBOX[thread inbox]
  R2 -->|name has no live thread| REFUSE[refuse + surface\nnever strand]
  RENAME[sirsi thread rename] -->|same thread-id, new label| MAP
  CLOSE[sirsi thread close] -->|free name for reuse| MAP
```

### Recommended Implementation Order

1. **P1 — grammar + validator (no enforcement):** a `namespec` package that parses,
   validates, and constructs `<agent>-<project>-<machine>[-<task>]`; the known
   agent/project sets; `machine` gleaned from hostname; unit tests both directions.
2. **P2 — router hands out names:** component-based `register`/`adopt` that returns
   the canonical name and writes the name→thread-id→machine-id→task-id mapping; the
   machine-slot == credentialed-machine-id check (extends ADR-067). **Precondition,
   not follow-up:** C2 (origin-pinned schema + promotion receipt) and C4 (CAS'd,
   crash-safe mapping transaction) land in this phase — the first registration P2
   ships already enforces both; they are not deferred to P3/P5.
3. **P3 — rename + reuse lifecycle:** `sirsi thread rename` (same thread-id, atomic
   relabel); retirement frees the name; the 1:1-live invariant enforced in the store.
4. **P4 — doctor + migration:** `sirsi router doctor` flags non-conforming live
   names and name/machine mismatches; publish the migration map; each lane renames
   once; then flip register to refuse non-conformant components.
5. **P5 — resolve identity from origin, not the working tree** (folds in
   `ra/identity-from-origin`): the known sets + name authority read from the router
   service / origin, killing the shared-checkout fragility that co-caused this ADR.

### Key Decision Points

| Question | Options | Recommendation |
|---|---|---|
| Who authors the name? | (a) lane self-names, router validates; (b) router constructs from components | **(b)** — owner directive; a lane cannot self-name into non-conformance |
| Do infra lanes get a project slot? | (a) bare singletons; (b) project slot for all | **(b)** — owner decision; `ra-router-m1`, `claude-home-m1`, `sirsi-governance-m5` |
| Machine slot source | (a) owner allowlist; (b) gleaned from host designated name, bound to machine-id | **(b)** — owner decision; `M1.local`→`m1`, verified against ADR-067 machine-id |
| Enforcement rollout | (a) hard-reject day one; (b) hand out conformant names + doctor-flag, then refuse post-migration | **(b)** — owner: mandate registration to a handed-out conformant name; migrate, then refuse |
| Rename semantics | (a) new id (strands old); (b) relabel same thread-id | **(b)** — thread-id durable; mail never strands (the whole point) |

## Consequences

- **Positive:** one parseable name per thread; the router understands and enforces the
  agent/project/machine/task hierarchy; renames and machine moves stop stranding mail;
  name reuse is safe and explicit; the manual "drain a retired id" work this ADR is
  born from disappears; identity stops depending on a shared checkout's branch.
- **Cost:** a one-time fabric-wide rename (each thread once); the register/adopt API
  changes from free-string to components; every lane's startup updates to the
  component-based register.
- **Risk:** during migration, mixed conformant/non-conformant names coexist — handled
  by P4's flag-then-refuse window, never a hard cutover that de-registers live lanes.

## References
ADR-062 (router service), ADR-067 (credentialed machine-id — the machine binding),
A22 (Neith's Triad), A27 (heartbeat/registration), A31 (Rule of Ra), A33 (thread
census), A37 (record on origin); tasks `ra/reassign-verb`, `ra/identity-from-origin`.
Owner directive 2026-09-28.
