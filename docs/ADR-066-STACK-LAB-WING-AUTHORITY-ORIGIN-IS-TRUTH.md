# ADR-066: Stack Lab Wing Authority — Origin Is Truth, One Registry

## Status

**Proposed** — 2026-09-16. Ra decision under the most-legs rule (owner 2026-09-03) on the
owner's directive of 2026-09-16: *"fix the replication route once and for all and canonize
through Ra."* Bind stays with the owner / SSA. Design and check mechanism proposed by
claude-home (router items `20260916-091404`, `20260916-113151`); registry built by
claude-inference (`SirsiMaster/sirsi-stacklab`).

## Context

On 2026-09-16 the fabric's Stack Lab wing records were found in four states at once:

1. **Stranded.** `stacklab.wing.io-connect` was committed on the M5 (`bc4ae5a`, branch
   `fix/h1-2-stream-pool-rail`) and never pushed. The router wing already declared it as a
   peer, so the roster pointed at a record reachable from nowhere. No alarm fired.
   Recovered by claude-home as sirsi-io-connect PR #197.
2. **Never authored.** `sne-engine`, `hardware-estate`, `pantheon.pt-wing-001` had scaffolding
   in the apple-stack-lab control plane (SirsiNexusApp fleet-mirror) but no record at all —
   on either Mac.
3. **Undeclared.** `nexus-experience`, `finalwishes`, `apple-accelerator-routes` existed as
   ideas, not as peers in any roster.
4. **Loose.** The SNE wing's own Stack Lab record — 15,003+ files of receipts, recipes and
   Ma'at incidents — lived as unversioned files in a non-git directory on the M5.

claude-inference then built `SirsiMaster/sirsi-stacklab` (private): `wings/` (eight records,
six of them schema-valid **drafts**), `canon/` (byte copies of contracts v1/v2 with
SHA256SUMS), `history/` (the SNE record under version control). That is the right fix for
state 4, but left unreconciled it is a **third** place a wing may live — beside a lane's own
`contracts/stacklab/` and the `sirsi-inference` fixture branch — and a new fragmentation.

The defect class is the same one ADR-065 names for lanes: **the thing that decides whether a
record exists lives where nobody checks it.** A wing that exists only on one host, one
branch, or one operator's memory does not exist for the fabric.

## Decision

**1. Origin is truth.** A canonical Stack Lab wing record exists for the fabric **only** when
it is on `origin/main` of its owning lane's repository at
`contracts/stacklab/<lane>-wing-v1.json`. A record that lives only as a local commit, an
unpushed commit, an unmerged branch, a control-plane copy, or a host-local file is
**stranded** and does not exist. (The router's own record is `sirsi-pantheon`
`docs/router-service/stacklab/router-wing-ra-v1.json`, byte-pinned to the SNE fixture per
`WING.md`; that pinning rule is unchanged.)

**2. One registry.** `SirsiMaster/sirsi-stacklab` is the **authoritative universal registry**.
It does not author wings. For each lane it **pins** the lane's origin record by SHA-256
(`wings/pinned/`) and mirrors canon and history. A file under `wings/` that is not a pin of an
origin record is a **draft**: schema-valid, useful, and **not canonical**. A draft becomes
canonical only when its owning lane lands the record on its own `origin/main` (Decision 1)
and the registry pin resolves to that content hash.

**3. The roster is bidirectional.** The router wing's `allowed_peer_wings` is the
authoritative roster of fabric wings. Every declared peer MUST resolve to an origin record
(Decision 1) **and** a matching registry pin (Decision 2). No canonical wing record may exist
without being a declared peer. Changing the roster is a change to the SNE fixture and the
vendored copy together, never a hand edit of one.

**4. Stranding is a checked, loud condition.** `sirsi stacklab doctor` (or a pass inside
`sirsi router doctor`) evaluates, for every declared peer:

| finding | condition |
|---|---|
| `stranded/unbuilt` | declared peer with no schema-valid record on its owning repo's `origin/main` |
| `unpushed/stranded` | a local wing record (working tree or non-main branch) with no origin record — the io-connect class |
| `unpinned` | origin record exists but the registry has no pin, or the pin hash differs |
| `undeclared` | a registry pin or origin record for a wing that is not in the roster |
| `invalid` | any record that fails `contracts/stacklab/v2/wing.schema.json` |

It runs in CI; `--fix` opens the recovery PR; a pre-push guard in any repo refuses to leave a
`contracts/stacklab/*wing*.json` change behind on an unmerged branch. **claude-home builds
this under Ra's review and bind (GO given on `20260916-091404`).**

**5. The fleet-mirror is secondary.** The file-based M5↔M1 mirror MUST additionally carry every
lane's wing, but Decisions 1–4 do not depend on it. Routed to SHA on return (2026-09-19).

**6. State at adoption and the ratification list.**

| wing | owner | state 2026-09-16 | to become canonical |
|---|---|---|---|
| `m1-ra` | ra | canonical (origin + pinned, `f0345682…`) | — |
| `io-connect` | claude-io | origin record in PR #197 (OPEN, CLEAN); pinned | merge #197 |
| `sne-engine` | codex-inference | draft | land on `sirsi-inference` `origin/main`; pin |
| `hardware-estate` | SHA (acting claude-io) | draft | land on `SirsiNexusApp` `origin/main`; pin |
| `pantheon.pt-wing-001` | pantheon | draft | land on owning repo `origin/main`; pin |
| `apple-accelerator-routes` | codex-inference / SHA | draft, **not in roster** | as above + roster (rs-45) |
| `nexus-experience` | nexus | draft, **not in roster** | as above + roster (rs-45) |
| `finalwishes` | finalwishes | draft, **not in roster** | as above + roster (rs-45) |

**rs-45**: extend the roster to the three undeclared wings through the SNE fixture refresh.
Until each row lands, the doctor reports it `stranded/unbuilt` — that is the intended,
honest state, not a failure of the check.

## Alternatives Considered

- **Per-lane repos only, no registry.** Rejected: the SNE record had no repo home at all, and
  nothing then indexes the fabric's wings in one place.
- **Registry as the single author.** Rejected: it moves each lane's record away from the lane
  that produces its evidence, and re-creates the control-plane-copy drift of state 2.
- **Fleet-mirror as the fix.** Rejected: a file mirror carries whatever is on the source host,
  including stranded and draft records, with no notion of origin.

## Consequences

- A wing declared before it is built shows up red in every doctor run until its lane lands
  it. That is the point.
- Lanes gain one obligation: push the record to `origin/main` of their own repo. The pre-push
  guard makes forgetting impossible to miss.
- The registry gains a pin per wing and a refresh step when a lane's record changes.
- Risk: two repos disagree about a wing (origin vs pin). The doctor names it `unpinned`; the
  origin record wins by Decision 1.

## References

- Router items: `20260916-075010` (establish + link), `20260916-085341` (io-connect
  recovered, PR #197), `20260916-091404` (canonize origin-is-truth), `20260916-113151`
  (universal registry exists; name the authority)
- `SirsiMaster/sirsi-stacklab` (registry), `sirsi-io-connect` PR #197
- `docs/router-service/stacklab/WING.md`, `contracts/stacklab/v2/wing.schema.json`
- ADR-065 (same defect class, for lanes); PANTHEON_RULES A37; A35 (scope the check)
