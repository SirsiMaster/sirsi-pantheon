# ADR-055 — SNE Licensing, Anubis/Ra Product Seam, and Disclosure Posture

**Status:** Accepted  
**Date:** 2026-08-03  
**Deciders:** Owner (Cylton Collymore); analysis by claude-nexus; codex-inference bound to enforce in review  
**Cross-repo canon:** sirsi-inference/docs/adr/ADR-002-sne-licensing-disclosure-posture.md (RATIFIED 2026-08-03; this ADR is the Pantheon-lane binding of that decision)

---

## Context

The SNE engine (Sirsi Nexus Engine) is the inference serving layer that
Pantheon products — Anubis and Ra — sit on top of. This ADR records the
decisions that govern how the engine license, product tiers, and IP posture
map to Pantheon's open/commercial split. It is written for the Pantheon repo
(sirsi-pantheon) as the operational binding of the cross-repo canon.

---

## Decisions

### 1. SNE is permanently closed-source

The engine source never crosses into this repo. The open-source-to-compete
argument was considered and rejected: our measured edge is the serving
architecture; publishing it donates it to the fastest-moving copier in the
same commit and cannot be undone. The Pantheon side of that boundary is:
zero engine source in `sirsi-pantheon`. The contract is the only crossing.

**Enforcement:** Any PR that adds engine source, internal kernel code, or
serving-architecture internals to this repo is a critical defect. Ma'at
gate (Rule A28) and code review must reject it.

### 2. Anubis is open source; Ra is commercial

- **Pantheon Anubis** (`sirsi-pantheon` this repo) — Apache 2.0, free for
  anyone. Ships with the interactive profile grant that lets individuals use
  SNE (or any compliant engine) without a Ra license.
- **Pantheon Ra** — commercial product; fleet management, enterprise SLA,
  deterministic audit mode at scale. Requires a Ra license grant.

### 3. `--profile interactive|fleet` is the license seam

The same flag that selects behaviour is the license boundary:

| Flag | Profile | License | User |
|------|---------|---------|------|
| `--profile interactive` | single-user / enthusiast | free (Anubis grant) | individual |
| `--profile fleet` | multi-node, fleet-managed | Ra commercial license | enterprise |

No DRM. Enforcement is contractual for fleet; the technical and commercial
boundaries are intentionally the same flag so the seam is simple and auditable.

### 4. Engine-swappability keeps the open-source claim honest

Anubis connects to SNE through an OpenAI-compatible API contract. It must
genuinely work against any compliant engine (mlx_lm, oMLX, or any future
compatible server). SNE holds the default seat on reproducible, measured
merit — not lock-in. If Anubis ever hard-codes SNE internals, this ADR is
violated.

### 5. Public verification boundary

What is public (docs/REPRODUCTION.md):
- The parameter and environment record for every published number
- The client-side benchmark harness (measures any OpenAI-compatible server;
  contains no engine code)
- Comparator setup instructions
- Upstream issue reproductions already public in upstream threads

What stays sealed:
- Engine kernels, serving architecture internals, the proposed upstream patch
- A provisional patent disclosure on the deterministic-serving method
  precedes any public artifact revealing its mechanics (task sne-36 in
  sirsi-inference)

### 6. A33 governs every outward claim (Rule A33)

Every number or capability claim on Pantheon surfaces (website, README,
release notes, deck) must follow the A33 voice: "potential defects" /
"potential improvements"; numbers scoped to the measurement environment;
reproduction pack linked. See docs/CLAIMS-TABLE-A33.md for the
owner-blessed claims table — that table is the only authorised source for
outward numbers. Prior claims-package language is superseded.

---

## Consequences

- **Anubis-side seam design** (task sne-37 in sirsi-inference): versioned
  contract spec, public releases shell repo (EULA + notarised binaries only),
  the two license grants. Routes to claude-pantheon when the fleet resumes.
- **Deck lane**: use only the A33-voice blessed claims table
  (docs/CLAIMS-TABLE-A33.md); nothing else ships on outward surfaces.
- **Distribution**: open Anubis is the free distribution channel — gets the
  engine in front of every Apple silicon enthusiast; Ra adds fleet management
  on top.
- **ADR-INDEX updated**: ADR-055 added; next available becomes ADR-055.

---

## Open items (inherited from sirsi-inference ADR-002)

- Seam ADR (sne-37): versioned contract spec, public shell repo, two grants.
- Owner blessing for the deferred Apple upstream filing.
- Counsel review of the provisional before conversion (optional, owner's call).

---

Refs: ANUBIS_RULES.md §2.2 (Rule A1 — safety, no engine source crosses),
ANUBIS_RULES.md (Rule A33 — humble claims law);
sirsi-inference/docs/adr/ADR-002-sne-licensing-disclosure-posture.md;
docs/CLAIMS-TABLE-A33.md; docs/REPRODUCTION.md
Changelog: v0.9.1 — ADR-055 SNE licensing and Anubis/Ra seam canon landed (renumbered from ADR-051; collision with #449)
