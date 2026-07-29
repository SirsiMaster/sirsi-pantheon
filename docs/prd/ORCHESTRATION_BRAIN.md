# PRD / Design — Sirsi Orchestration Brain (tiered, pluggable, user-navigable)

**Status:** PROPOSED — co-authored plan (claude-home ↔ claude-pantheon), 2026-06-30
**Owner repo:** sirsi-pantheon (canon + implementation) · consumed by SirsiNexus (visibility/management)
**Governs:** PANTHEON_RULES A26 (router workstream), A27 (heartbeat), A28 (CI gate). Proposes new **A29**.

---

## 0. Vision (what a public user gets)

A user downloads **Pantheon** and it *already works with zero AI configured* — a deterministic
router + dispatch loop runs the realm. Then they can dial intelligence UP, one tier at a time, and
**see, modify, troubleshoot, and swap** every LLM choice from inside Pantheon (CLI + menubar) and,
at the enterprise tier, inside **SirsiNexus**. No black box. The brain is a config they own.

> The model is the cheap, swappable part. The architecture is the asset. We ship the architecture.

---

## 1. The three tiers

| Tier | Job | Default impl | Needs LLM? | Cost |
|---|---|---|---|---|
| **Tier 0 — Dispatch** | watch inbox, route by rules, heartbeat, close acks | event-driven router (Router v2: SQLite + fsnotify/socket + MCP) | **No** | free |
| **Tier 1 — Triage** | classify ambiguous items, emit action grammar, escalate | rules-first; optional local LLM for the NL remainder | optional | zero-token if local |
| **Tier 2 — Execution** | build / review / bind / deploy / validate (tool-using) | agentic Claude or codex session, on-demand | yes (agentic) | per-session |

**Loop ≠ brain:** the eternal monitor is Tier-0 (bash/launchd today → Router v2 daemon). A model is
*invoked by* the loop, never *is* the loop. Tier-0 must remain fully functional with **no LLM at all**.

### Default routing policy — the Model Tiering Doctrine (PANTHEON_RULES A30, owner directive 2026-07-13)

The law in one line: **generation is cheap to get wrong; judgment is expensive to get wrong — push
generation down-tier, keep judgment up-tier.** Independence in review comes from a fresh context with
no stake in the code, **not** from a different brand of model. What is tiered is *cognitive difficulty*,
not vendor. This is the default work→tier mapping the brain routes on; canonical law lives in
`~/Development/AGENTS.md`.

| Work | Default tier | Why |
|---|---|---|
| Queue triage, first-draft code for a well-specified decomposed task, summarization, log-reading, boilerplate, NL query over local state | **Tier 0** (local, zero-token) | High-volume, low-stakes. **Output is a SCREEN or DRAFT, never a verdict.** |
| Routing, nudging, ACK-closes, board publishing, grinding a decomposed list (dep bumps, doc updates, test fixes) | **Tier 1** (cloud, standard effort) | Routine agentic work; most scheduled/loop runs. An empty run needs almost nothing. |
| **Binding verdict (source-deep review before merge — ALWAYS)**, architecture decision, security review, ESCALATE-classed ambiguity, debugging that resisted a first pass | **Tier 2** (frontier, high effort) | The irreversible, exceptional-thinking work. Spend where a mistake ships to main. |

Operating rules (the brain enforces these; they are not left to discipline):

1. **Builders decompose; the cheapest competent tier types.** A thread's job on well-specified work is
   spec → hand generation to Tier 0 → review the output — not typing code itself at frontier prices.
2. **The bind is always frontier.** A slightly-off draft is caught at bind; a slightly-off bind ships a
   bug to main. Spend where the failure is irreversible.
3. **Screens never become verdicts.** No Tier-0 classification may stand as a binding
   security/review/architecture decision — verification stays up-tier.
4. **Read only what escalates.** Cloud models do not read whole queues/logs/repos when a Tier-0 screen
   can classify first (the 2M-token incident is the cautionary tale).
5. **Route by difficulty, not by habit.** If a task's tier is unclear, start one tier lower and escalate
   on failure — escalation is cheap, standing overspend is not.

Brand invariant (A25/Brand-Over-Model-Name): user-facing surfaces never expose model identity
("Ask Sirsi", never a vendor/model name); the on-device privacy promise stands independent of which
local model serves Tier 0.

### Tier-0 Registry + Wake invariant (owner directive 2026-06-30) — "the router can ALWAYS see and wake every registered thread"

Registration MUST imply wakeability. A thread that is "registered but unwakeable" is a broken
contract (it's the zombie state we have today: `claude-*` registered yet asleep, items rotting).
Enforce this invariant in Tier-0:

1. **Registration binds to a persistent wake-channel.** `sirsi thread register` MUST record *how* to
   wake the thread and verify that channel is live: **Claude session → a persistent `/loop` watcher
   process** (separate from the interactive turn; pulls inbox, queues work, can resume/notify);
   **worker/headless → a LaunchAgent pull-loop** (`wake-install`); **codex → its heartbeat automation**.
   No live wake-channel ⇒ registration is **incomplete** (rejected or immediately flagged), never silently accepted.
2. **Lifecycle is shared: no live channel ⇒ not registered.** The router continuously verifies each
   registered thread's channel (node-status is the truth source). A registered thread whose channel
   died is **auto-re-armed** when re-armable (relaunch the LaunchAgent / restart the watcher); when
   NOT re-armable (owner hard-closed the session), it is **de-registered + surfaced to the owner** —
   never left as a registered zombie. This makes A27 (Heartbeat Loop Mandate) *enforced*, not advisory.
3. **A wake verb.** `sirsi router wake <thread|--all>` triggers the recorded channel: pull-now for a
   live watcher/worker; re-arm for a dead-but-re-armable channel; honest **"needs-owner"** for a
   closed interactive session (the one case local code genuinely cannot resurrect — stated plainly,
   not faked). With persistent watchers in place, "wake all" becomes real for every thread whose
   channel is a resident process.

The honest boundary: local code cannot resurrect a fully-closed Claude *process*. The invariant is
made true by binding registration to a **resident watcher** that outlives any single turn — so the
router always has a live handle to pull/queue/notify, and "registered" can never mean "invisible."
This registry IS Tier-0's core; the brain (Tier-1/2) sits on top of a substrate that always sees its threads.

## 2. The LLM spectrum (the slider the user navigates)

- **Level 0 — Deterministic (default, ships on):** no LLM. Rules dispatch, route, heartbeat, auto-close acks, auto-escalate ambiguity. Pantheon is useful out of the box with zero AI, zero keys, zero cost.
- **Level 1 — + Local triage:** a local model (MLX/Ollama: gemma, Qwen, etc.) screens the ambiguous remainder. Zero-token. Recommended default once a model is present.
- **Level 2 — + Agentic execution:** local agent / codex / Claude session handles ESCALATE work end-to-end.
- **Level 3 — + Hosted:** opt-in API keys (Haiku/Sonnet/Opus, or any OpenAI-compatible: GLM/Kimi/DeepSeek) per role. Only path that adds per-token cost; never required.

Per-**role** pluggability (triage vs execution vs build-draft can each use a different provider).
Config lives in `~/.sirsi/orchestrator.conf` (the existing `sirsi-brain.sh` contract, formalized).

## 3. User-facing surfaces ("visible + modifiable + troubleshootable")

- **`sirsi brain` CLI** (the control plane):
  - `status` — current tier/level, model per role, loop liveness, RAM headroom.
  - `route <role>` — show the Local LLM router's resolved backend without loading a model.
  - `use <role> <provider>` — swap a role's model (e.g. `use triage local:qwen2.5-7b-instruct`); `use <role> none` drops to deterministic.
  - `test [item]` — dry-run a triage/route decision and show the reasoning (no side effects).
  - `doctor` — troubleshoot: is the model loaded? RAM ok? loop alive? config valid? auth present (Level 3)? Emits plain-English fixes.
  - `levels` — show the spectrum + what each unlocks + the cost.
- **Menubar / TUI brain panel:** shows tier + model per role; one-click swap; live loop/RAM status. Plain-English per [[feedback_plain_english_gui]].
- **SirsiNexus visibility:** Nexus consumes Pantheon's brain status (local-models-through-pantheon pattern) so enterprise users see + manage the orchestration tier across endpoints; modify per-fleet defaults.

## 4. Canon to establish (so it's seamless + governed)

1. **PANTHEON_RULES A29 — "Orchestration Brain: Tiered & Pluggable"**: defines the 3 tiers; mandates the **deterministic Tier-0 floor** (must run with zero LLM); the **per-role pluggable** contract; the **user-visibility + swap + doctor** requirement; first-run ships at **Level 0**.
2. **ADR-0XX** (Neith Triad per A22): the decision, rejected alternatives (single-model brain; always-on Claude; gemma-only), data-flow + implementation-order + decision matrix.
3. **User guide** (A8): `docs/user-guides/orchestration-brain.md` — plain-English: what the tiers are, how to swap, how to troubleshoot, what each level costs.

## 5. Build decomposition (phases; each independently shippable + bound)

- **P1 — Tier-0 dispatch:** Router v2 (SQLite store off-git, fsnotify/socket wake, MCP-as-interface). See [[docs/prd/ROUTER_V2_DURABLE_DISPATCH.md]]. Foundation.
- **P2 — Control plane:** formalize `~/.sirsi/orchestrator.conf` + ship `sirsi brain {status,use,test,doctor,levels}`. Pluggable per-role.
- **P3 — Tier-1 triage:** rules-first classifier wired into dispatch; optional local-model fallback for NL ambiguity; emits the ROUTE/CLOSE/ACK/ESCALATE grammar. (Reuses `sirsi-conductor.sh` grammar.)
- **P4 — Surfaces:** menubar/TUI brain panel + Nexus brain-status consumption.
- **P5 — Canon + docs:** A29 + ADR + user guide; first-run defaults to Level 0.
- **P6 — Validate/deploy:** E2E across Level 0→3, model-swap matrix tested, ship in the next Pantheon release; Nexus consumes it.

## 6. /goal (completion condition)

Met when ALL hold:
1. A fresh public Pantheon install runs the realm at **Level 0 (deterministic, no LLM)** out of the box — dispatch/route/heartbeat/ack all work with zero AI configured.
2. `sirsi brain use triage local:<model>` adds Tier-1 with **zero tokens**; `sirsi brain use <role> none` reverts to deterministic — both without restart/rewrite.
3. `sirsi brain doctor` diagnoses a broken/missing model + a dead loop + bad auth, in plain English, and tells the user how to fix it.
4. The active tier + per-role model are **visible and swappable in the menubar**, and **visible in SirsiNexus**.
5. Canon landed: **A29** + an ADR (Neith Triad) + `docs/user-guides/orchestration-brain.md`.
6. Full green: build/vet/test/Ma'at gate; tiers E2E-tested; shipped in a Pantheon release.

## 7. Neith's Triad (A22) — seeds for the ADR

**Data flow:** `inbox event → Tier-0 rules → {handled | ambiguous} → Local LLM Router resolves role backend → Tier-1 triage (rules→optional LLM) → {ROUTE/CLOSE/ACK | ESCALATE} → Tier-2 agentic execution → result back to router`. Fallback: any model-absent/error path degrades to deterministic (escalate or queue), never drops an item.

**Implementation order:** P1 (dispatch) → P2 (control plane) → P3 (triage) → P4 (surfaces) → P5 (canon/docs) → P6 (validate/deploy). P1 is the minimum viable; P2-P3 deliver the swap UX; P4-P6 make it public-grade.

**Key decisions:**

| Question | Options | Recommendation |
|---|---|---|
| What is the eternal loop? | a model / a Claude session / a daemon | **daemon (Tier-0), no LLM** — models are invoked, not resident loops |
| Default for public download | needs an LLM / works with none | **Level 0 deterministic, ships on** — zero friction, zero cost |
| Triage model | gemma-4 (reasoner) / format-instruct (Qwen-class) / rules-only | **rules-first + Local LLM router**; Gemma is one resident backend, not the slot |
| Cost model | per-token default / local default | **local/free default**; hosted (Level 3) strictly opt-in |
| Where users manage it | code only / config + CLI + GUI + Nexus | **all of CLI `sirsi brain` + menubar + Nexus** — visible + modifiable |

---

### Collaboration note
claude-pantheon owns pantheon canon + build; claude-home orchestrates + binds; gemma drafts where useful. This doc is the seed — claude-pantheon validates/amends, then we execute P1→P6 through the router relay (A26).
