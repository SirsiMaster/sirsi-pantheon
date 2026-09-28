# ADR-072 — Universal Thread Naming Convention, Router-Enforced

**Status:** Proposed 2026-09-28 (Ra design; owner-directed; SSA review + owner bind pending)
**Deciders:** owner (directive 2026-09-28), Ra (router architect)
**Custodian:** 𓁢 the Router (registry authority)

## Context

Router identity is a mess, and it costs hours every day. In one session alone the
fabric carried, for a single logical lane, all of: bare `claude-finalwishes`,
`claude-finalwishes-m1`, `claude-finalwishes-m5`, `claude-finalwishes-helper`, and
`claude-fw` — plus `hermes` vs `hermes-m5` vs `claude-io`, and `codex-inference` vs
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

- Lowercase, hyphen-delimited, each slot `[a-z0-9]+` (no internal hyphens inside a
  slot; the hyphen is the delimiter).
- **agent** — the agent/lane identity family: `claude`, `codex`, `ra`, `hermes`,
  `gemma`, `sirsi` (governance/admin), … A fixed, router-known set.
- **project** — the product or domain the thread serves. **Every** thread carries
  one, including infrastructure lanes (owner decision 2026-09-28): `router`,
  `finalwishes`, `pantheon`, `hermes`, `photon`, `apollo`, `nexus`, `deck`,
  `home` (fabric/overseer, e.g. claude-home), `governance` (SSA), `maat`, …
- **machine** — the host, **gleaned by the router from the machine's designated
  name** (owner decision 2026-09-28): the hostname prefix before the first `.`,
  lowercased. `M1.local` → `m1`, `M5.local` → `m5`; future machines self-name on
  launch and their prefix becomes the slot. The name slot is bound to the durable
  **machine-id** (ADR-067) behind it, and the router enforces they agree.
- **task** — optional. A specific workstream when one lane runs parallel tasks
  (`fw-r02`, `hermes-releases`). Absent for a lane's single default thread.

Examples: `ra-router-m1`, `claude-home-m1`, `sirsi-governance-m5`,
`codex-finalwishes-m5`, `claude-finalwishes-m1-fw-r02`, `hermes-photon-m5`.

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
| `hermes` / `hermes-m5` | `hermes-hermes-m1` / `hermes-hermes-m5` |
| `codex-pantheon` | `codex-pantheon-m1` |

Retired ids (`codex-inference`, bare `claude-finalwishes`, `claude-fw`,
`claude-finalwishes-helper`, `manual-pantheon`, `cylton-hermes`) are already drained
and stay retired; their names become reusable under the grammar.

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
   machine-slot == credentialed-machine-id check (extends ADR-067).
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
