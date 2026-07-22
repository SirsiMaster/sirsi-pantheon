# Agent Roles Contract (claude / codex / gemma / bind)

The mechanical contract for the multi-agent spec. One page; every claim here is
enforceable by a tool, a gate, or a routed item — nothing rests on convention.
Drafted by gemma (Tier 0), reviewed and bound by claude-pantheon (Tier 2).

The law in one line (Rule A30): **generation is cheap to get wrong; judgment is
expensive to get wrong — push generation down-tier, keep judgment up-tier.**

## 1. What gemma receives and returns

Gemma is a text-in/text-out worker. It has no tools, no file system, no network.

* **Input:** all required source must be embedded in the task body via
  `--instructions @file` (or inline). Gemma resolves no paths; an un-embedded
  build task is malformed and produces a deliverable built on nothing.
* **Output:** complete files, each preceded by `=== FILE: <path> ===` when
  there is more than one. A referenced-but-not-embedded file yields a trailing
  `MISSING-SOURCE: <path>` marker so the router can re-scope.
* **Status:** every gemma output is a DRAFT (Tier 0). Per Rule A30 a screen or
  draft never stands as a verdict. Gemma never refuses — it produces its best
  attempt and flags what a real agent must confirm.

## 2. What codex is authoritative on

Codex is the independent source-deep reviewer and per-lane SME.

* **Method:** review means `gh pr diff` plus reading the source at the current
  head — never review-by-commit-message, never review of a stale revision.
* **Closure:** a codex verdict is returned in the requester's item Result
  section and the item closed; it is not a new inbound.

## 3. What bind mechanically is

Per ADR-041, `binding-hold` is a mechanical gate, not a conversational agreement.

* **Pass condition:** an APPROVED GitHub review from a login ≠ author on the
  CURRENT head SHA. A push after approval drops the approval and re-fails the
  hold.
* **Key placement IS the enforcement:** the `sirsi-bind` GitHub App key lives
  ONLY on the conduit host, never in GitHub Secrets — a key reachable from a
  workflow would let any PR self-approve, restoring the circularity the gate
  exists to break.
* **Why mechanical:** relay conversation "rounds" leave no mechanical trace. A
  conversation cannot be diffed against a merge — which is why nothing short of
  a recorded review can tell an honest merge from a self-merge.

## 4. Tier routing (Rule A30)

| Work | Tier | Who |
| :--- | :--- | :--- |
| Drafting, decomposed builds, triage, summarization | Tier 0 (local, zero tokens) | gemma |
| Routine agentic work — routing, ACK-closes, grinding task lists | Tier 1 (cloud, standard) | claude threads |
| Binding verdicts, architecture, security review, resistant debugging | Tier 2 (frontier, high effort) | claude / codex, always for the bind |

Screens never become verdicts. When the tier is unclear, start one lower and
escalate on failure.

## 5. Relay mechanics (Rule A26)

A routed request gets its reply routed BACK to the requester as a fresh inbound
item — close-with-Result alone is audit trail and wakes no one. An agent that
closes without routing the reply has not finished the relay.
