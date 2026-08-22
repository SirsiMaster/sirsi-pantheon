# 𓃣 Anubis Engineering Journal
# Running commentary and insights — a documentary of the build process.
# Each entry is timestamped with context and reasoning.
# This is the "why" behind every decision.

## 2026-08-21 - M1 post-repair serving continuity

- Sent one bounded 32-token OpenAI-compatible request after ownership repair.
- Received coherent HTTP 200 response in 1.008 seconds with 24 visible tokens.
- Runtime/manifest hashes and plain NVFP4/no-assistant execution remained exact.
- Recorded 53.8028 visible and 71.7371 engine tok/s only as single-request
  continuity telemetry; no durability, isolation, thermal, or cross-device
  performance claim is admitted from this sample.

## 2026-08-21 - Transactional M1 SNE ownership repair

- Extended `sirsi sne ownership` with explicit `repair --confirm`.
- Repair stages SHA-pinned backup and staged receipt, disables and moves only
  recognized legacy labels, verifies one canonical owner, writes an accepted
  receipt, and rolls back enablement/plists if any step fails.
- Focused tests cover accepted retirement, disable-failure rollback, and missing
  confirmation.
- Real M1 repair accepted and retired `ai.sirsi.pantheon-sne-e2b`.
- Post-repair diagnostic reports canonical=1, legacy=0, loaded=1.
- Canonical supervisor PID and exact plain E2B NVFP4 model/manifest remained
  unchanged, proving ownership cleanup did not perturb serving.

## 2026-08-21 - Machine-readable SNE ownership gate

- Added read-only `sirsi sne ownership` and JSON schema
  `pantheon.sne-ownership.v1`.
- The diagnostic classifies the one canonical supervisor, legacy Pantheon-SNE
  labels, executable targets, and loaded state; ownership drift exits nonzero
  with actionable transactional-repair guidance.
- Unit gates initially caught missing recovery guidance when LaunchAgents did
  not exist; repaired the early return and reran green.
- Built a transient current CLI, ran it on the M1, and removed it. Real result:
  one loaded canonical owner plus one stopped legacy owner, correctly rejected
  as `ownership-drift`.

## 2026-08-21 - M1 developer-state drift and readiness invariant

- Read-only Tailscale/SSH audit proved the M1 is not a clean host: no installed
  Pantheon app or CLI, but an ad-hoc dashboard candidate, signed SNE package,
  multiple research runtimes/models, and two SNE LaunchAgents are present.
- The old dashboard candidate projected incompatible states at once: ready,
  unqualified support, stopped lifecycle, and a 64-position cache.
- Added a current-source post-reduction invariant requiring the active identity
  to be signed-support-matrix `release-supported` before top-level readiness.
- Unsupported active processes remain stoppable but are blocked from Nexus and
  OpenAI-compatible readiness.
- Focused gates and the complete dashboard suite pass.
- Harness lesson repeated and recorded: never use `path` as a zsh variable;
  zsh binds it to `PATH`, causing commands to disappear mid-script. Remote
  release checks now use `endpoint` plus absolute tool paths.

## 2026-08-21 - Optional menubar authenticated restart

- Added `Restart & Resume Safely…` to the Pantheon Command Deck.
- The action opens a visible Terminal with
  `host restart --authenticated --confirm`; Apple still prompts for the
  FileVault credential and cancellation remains available.
- Added a regression contract preventing either consent flag from drifting.
- `go test ./cmd/sirsi-menubar -count=1` passes.
- The complete `go test ./cmd/sirsi -count=1` suite passes outside the sandbox
  in 32.911 seconds; the sandbox-only loopback bind failure was not a product
  defect.
- Current locked-session distribution audit exposes zero signing identities,
  an inaccessible notary keychain profile, and no local DMG. Re-audit after
  unlock before concluding credentials or certificates are absent.

## 2026-08-21 - FileVault-aware planned restart contract

- Determined that the observed post-reboot login behavior was expected:
  automatic login is off and launchd cannot cross FileVault's pre-boot gate.
- Added `sirsi host restart --authenticated --confirm` as a narrow, explicit
  wrapper around Apple's interactive authenticated-restart facility.
- The command checks FileVault active state and hardware support, rejects
  ordinary or unconfirmed restart requests, and never handles password bytes.
- Added four focused tests covering consent, exact Apple command sequence,
  fail-closed preflight behavior, and delay validation; all pass.
- Canonized planned restart, process recovery, and unplanned cold boot as three
  distinct lifecycle cases in the product contract.

## 2026-08-21 - One-copy Homebrew CLI exposure

- Confirmed against the installed Homebrew 6 Cask implementation and fixtures
  that a `binary` artifact may target an executable inside an installed app.
- Updated release cask generation to install `Pantheon.app` once and expose the
  bundled `sirsi` CLI from `Pantheon.app/Contents/MacOS/sirsi`.
- Preserved explicit user consent for caretaker registration; Homebrew install
  does not silently enable login restoration.
- M5 SNE API4096 watcher remains healthy and fail-closed while the graphical
  Metal session is locked; it will resume the preserved tuple after unlock.

## 2026-08-21 - Exact SNE-to-Nexus user handoff

- Pantheon already owned install, signed admission, start/stop, readiness,
  diagnostics, recovery, catalog update/rollback, and model removal, but its SNE
  page ended at READY and required the user to know the separate CLI command.
- Added one shared `BuildNexusCapabilityURL` contract for dashboard and CLI.
  It permits only HTTPS `sirsi.ai`, rejects userinfo/alternate hosts, and carries
  the local SNE capability in the browser fragment rather than a network query.
- Added same-origin/capability-protected `POST /api/sne/nexus/open`. It opens
  Nexus only when Pantheon verifies an exact active signed release-supported
  tuple and returns no secret material.
- Added an accessible `[Open Nexus Local AI]` ready-state control. Candidate,
  unqualified, stopped, identity-drifted, and missing-capability states fail
  closed. Focused `internal/dashboard` and `cmd/sirsi` suites pass.
- Receiver audit confirmed Nexus already stores the fragment capability only in
  `sessionStorage` and immediately removes it from browser history. Corrected
  Pantheon/CLI launch from the site root to the canonical `/local-ai` workspace;
  both focused suites now assert and pass the exact route.
- Privacy follow-up: the dashboard no longer returns raw browser-opener errors.
  Failure uses stable public text, preventing platform diagnostics from ever
  echoing launch arguments or fragment capability material. Dashboard suite passes.

---

## Entry 029 — 2026-04-01 15:47 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"b3eafb76-9e33-4114-9bf6-345bb2dd653b","transcript_path":"/Users/thekryptodragon/.claude/projects/-Users-thekryptodragon/b3eafb76-9e33-4114-9bf6-345bb2dd653b.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","trigger":"manual","custom_instructions":""}

---

## Entry 030 — 2026-04-02 16:50 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- Session: Seshat v2.0 adapters built, 22 plugins installed, screenshots MCP, Sirsi Orchestrator, GitHub CI cleanup (225+ runs), NexusApp workflow fix, Go 1.24 compat, 78G iCloud migration for M5 transfer. All repos clean and pushed.

---

## Entry 031 — 2026-04-04 18:17 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- Session: ProtectGlyph, Stele Universal Event Bus, SIRSI_MASTER_PLAN, Deity Registry (Rule A25). Shipped v0.10.0. All deities inscribe to Stele. Ma'at owns all quality gates across all repos. Pre-push hooks corrected. Case studies written. Full lifecycle LoE assessed for all 4 repos. Next session: KV cache optimizations, token usage improvements, agentic harness enhancements, then full-throttle dev on FinalWishes Sprint 5-6 and Assiduous Sprint 11-13.

---

## Entry 032 — 2026-04-04 18:21 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"1b4b4861-83fa-412d-a688-c199b6f4e775","transcript_path":"/Users/thekryptodragon/.claude/projects/-Users-thekryptodragon/1b4b4861-83fa-412d-a688-c199b6f4e775.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","trigger":"manual","custom_instructions":""}

---

## Entry 033 — 2026-04-06 02:11 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"e3a963d3-b25b-4a85-a05c-c69aecd0145f","transcript_path":"/Users/thekryptodragon/.claude/projects/-Users-thekryptodragon/e3a963d3-b25b-4a85-a05c-c69aecd0145f.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","trigger":"manual","custom_instructions":""}

---

## Entry 034 — 2026-04-18 20:11 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"73458060-7593-4916-9c32-3885e6708be2","transcript_path":"/Users/thekryptodragon/.claude/projects/-Users-thekryptodragon-Development-sirsi-pantheon/73458060-7593-4916-9c32-3885e6708be2.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","trigger":"auto","custom_instructions":null}

---

## Entry 035 — 2026-05-19 18:35 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- -
- Router snapshot:
- active topics: assiduous-v110-completion, finalwishes-v010-illinois-probate, ra-horus-router-hypervisor-canon, claude-cli-auth-for-router-dispatch
- completed topics: 25
- last Codex read: 2026-05-19T10:50:37-04:00
- last Claude read: 2026-05-19T20:00:00Z
- pending:
- claude-assiduous: 20260519-codex-assiduous-google-calendar-implementation
- claude-finalwishes: 20260518-codex-finalwishes-v010-illinois-probate-review, 20260519-codex-finalwishes-v010-goal-review
- codex: 20260519-claude-pantheon-horus-wake-complete
- codex-assiduous: 20260519-claude-assiduous-codex-batch2
- codex-pantheon: 20260519-claude-pantheon-horus-wake-complete
- dispatch ledger: 968 bytes, updated 2026-05-19 11:42:04

---

## 2026-05-21/22 — Router Collapse + Caffeinate Contract + Verification

**Session goal:** strip overengineered push-model router infrastructure; ship a pull-model that works for any agent identity, with native FSEvents wake and thread keep-alive.

### Commits shipped (sirsi-pantheon, all on origin/main)

- `be2f2b7` `fix(router)` — dispatch.sh handles BOTH legacy `state.json:pending[]` AND pull-model `items/*.md` queues
- `76a43cc` `feat(hooks)` — caffeinate claude threads (auto-register + background heartbeat loop anchored to claude PID)
- `8c3e359` `docs(agents)` — add Caffeinate Contract (universal 4-step pattern) to sirsi-pantheon AGENTS.md
- `22ec913` `feat(router)` — `sirsi router ack <agent> <id>` migration helper (authored by codex-pantheon, committed by claude-pantheon)
- `84f79ca` `docs(agents)` — add §Lean #11 (wake mechanisms should not own delivery semantics — codex-pantheon adoption)
- `446880d` and 5 sibling commits — Lean Engineering Doctrine appended to AGENTS.md in all 6 repos (assiduous, FinalWishes, homebrew-tools, porch-and-alley, sirsi-pantheon, SirsiNexusApp)
- Earlier (same arc): `d3a396f` pull-model router (`send/pull/show/close/status`), `1cc3347` deleted 10 legacy push-model verbs (~969 LOC removed), `7af0687` hook surfaces pull-model items

### Architecture state on disk

- **Router CLI:** 6 verbs total — `status`, `send`, `pull`, `show`, `close`, `ack`. Down from 12 push-model verbs at session start. `routercmd.go` 1051 → 198 → 365 lines net (after ack addition).
- **Storage:** `internal/work` package, file-per-item under `.agents/idea-router/items/<ts>-<from>-<to>-<slug>.md` with YAML frontmatter (`from`, `to`, `status`, `opened`, `closed`, `title`).
- **Wake:** launchd `com.sirsi.idea-router.plist` with `WatchPaths` on `state.json`, `items/`, `proposals/`. ThrottleInterval=10. Fires `.agents/idea-router/dispatch.sh` on any change. Dispatch reads both queues, spawns `claude --print` per agent, then `sirsi router ack <agent> <ids>` to drain legacy pending. Zero idle process.
- **Thread keep-alive:** `.claude/hooks/router_inbox_check.py` on SessionStart + UserPromptSubmit. Auto-registers if no fresh active thread, immediate heartbeat, spawns detached bash loop (`while kill -0 <claude_pid>; do sirsi thread heartbeat; sleep 60; done`). Dedup via `/tmp/sirsi-caffeinate-<thread_id>.pid`.

### Verification gaps surfaced (not theoretical — real)

1. **Adoption ≠ notification.** Shipped notice to 5 sibling claude-* agents about new `ack` verb but never verified adoption. Sent 5 follow-up adoption-ack-requests with explicit reply contract (`close --result "adopted"` or variant). Adoption is now async-pending; closes organically as repos get worked.
2. **8 items in `items/` have empty `to:` field** — direct file writes bypassing `sirsi router send` (which requires `--to`). Senders unknown. Per AGENTS.md §Lean #10 (atomicity at FS boundary), all writes should flow through CLI.
3. **Orphan CTR threads accumulating.** `sirsi thread list` shows 2 stale claude-pantheon threads from earlier dispatcher spawns whose caffeinators died with host processes. CTR doesn't auto-close on dead-PID. Recommendation: `sirsi thread reaper` that marks dead-PID threads closed on read paths.
4. **dispatch.sh "agents fired: 1" with no observable claude output.** Child output buffered until exit. Recommend `--output-format stream-json --verbose` per PANTHEON_RULES.md §2.21 (Ra Scope Autonomy).
5. **Caffeinate Contract verified only on claude-pantheon side.** Codex-side implementation pending; sent question whether Codex.app's automation API allows long-lived background processes.

### Doctrine codified

- `~/Development/AGENTS.md` §Lean Engineering Doctrine — 11 numbered principles, with §Lean #11 attributed to codex-pantheon
- `~/Development/AGENTS.md` §Thread Registration Law §Caffeinate Contract — 4-step universal pattern
- Same propagated to all 6 repo `AGENTS.md` files
- `~/.claude/projects/-Users-thekryptodragon/memory/feedback_lean_ethos.md` — user's "LEAN AF, direct comms, smallest packages" ethos as a memory entry for future sessions
- `~/.claude/projects/-Users-thekryptodragon/memory/MEMORY.md` — indexed with the LEAN ethos pointer

### CTR state at session end

- Sent + bridged 2 router items to codex-pantheon (ack-verb request, then verification-insights)
- Sent + bridged 5 router items to sibling claude-* agents (ack-adoption requests)
- claude-pantheon's session thread (`thr-a441bbff379e62a9`) closed explicitly + caffeinator (PID 95339) killed
- 2 orphan claude-pantheon threads remain in CTR (other concurrent sessions, not mine)
- `pending[claude-pantheon]` and `pending[codex-pantheon]` both drained to 0 at session end

### Lessons for next session

- **Question polling before tuning intervals.** Per AGENTS.md §Lean #1 — applied here to replace 1s polling daemon with FSEvents.
- **Verify before claiming.** Earlier in session I declared "FSEvents wake live and proven" when it was only proving the OLD legacy queue. Codex caught the binary mismatch. Lesson: smoke-test against the NEW model's items, not just the OLD pending[].
- **Notification ≠ adoption.** Sending a router item is not confirmation it was acted on. Use explicit ack-request items with reply contracts for verification.
- **Multi-agent collaboration loop works.** Claude → Codex → Claude → Codex round-trip on the binary mismatch + ack verb was peer-to-peer with no human in the loop. Each agent acted from its vantage point.

---

## 2026-05-26 — Understand-Anything Plugin Installed + Knowledge Graph Indexed

**Session goal:** install the Understand-Anything Claude Code plugin, index sirsi-pantheon's full polyglot codebase into a semantic knowledge graph, and unify the resulting artifact with Thoth's memory model.

### Plugin install

- `pnpm` 11.3.0 installed via Homebrew (Node 25.6.1 already present, ≥22 requirement met).
- Plugin marketplace added: `Lum1104/Understand-Anything` → installed `understand-anything` v2.7.5 at `~/.claude/plugins/cache/understand-anything/understand-anything/2.7.5/`.
- Workspace built with `--config.dangerouslyAllowAllBuilds=true` (pnpm 11 default-denies postinstall scripts; tree-sitter parsers + esbuild need them). 12 tree-sitter language parsers compiled.

### Indexing the repo

Ran `/understand` over the full project (`--scope everything`, 894 git-tracked files + 13 untracked = 907 scanned). Skill pipeline ran all 7 phases:

| Phase | Output |
|-------|--------|
| 0 — Pre-flight | Plugin root resolved, core built, repo at `22ec913` |
| 0.5 — Ignore config | `.understand-anything/.understandignore` generated; nothing excluded (full polyglot pass) |
| 1 — SCAN | 907 files, 486 code, 19 languages, 2,277 internal import edges resolved by static analysis |
| 1.5 — BATCH | 56 semantic batches via Louvain community detection (sizes 3–32 files) |
| 2 — ANALYZE | 56 `file-analyzer` subagent dispatches in parallel (background, sliding 5–10 concurrent). Total 3,354 raw nodes + 6,935 raw edges produced |
| 3 — ASSEMBLE REVIEW | Merge step + path-convention `tested_by` linker. 14 duplicates collapsed; 17 `step:` → `pipeline:` prefix normalizations applied. 0 dangling edges. |
| 4 — ARCHITECTURE | 9 architectural layers, all 924 file-level nodes assigned exactly once |
| 5 — TOUR | 14-step pedagogical tour starting at `cmd/sirsi/main.go`, walking through the deity hierarchy |
| 6 — REVIEW | Inline deterministic validator: 0 issues, 252 orphan warnings (markdown docs + configs with no edges — expected) |
| 7 — SAVE | `knowledge-graph.json` (2.9 MB), `meta.json`, and `fingerprints.json` (907 baseline hashes) written under `.understand-anything/` |

### Final graph

- **3,340 nodes** — 496 file, 2,108 function, 308 class, 353 document, 51 config, 23 pipeline, 1 service.
- **6,947 edges** — 2,433 contains, 2,279 imports, 1,816 exports, 128 tested_by, 126 related, 70 depends_on, 48 calls, 22 documents, 19 configures, 4 deploys, 2 triggers.
- **9 layers** — cli-entrypoints, core-services, mobile-bindings, editor-extensions, agent-workqueue, documentation, infrastructure-cicd, configuration, testing.
- **14-step tour** — README → cmd/sirsi/main.go → deity binaries → Jackal rules → Isis/Guard → Thoth/Ma'at → MCP server → Horus dashboard → mobile bindings → VS Code extensions → idea router → build → CI.

### Three-tool clarification (Seba / Thoth / Understand-Anything)

User flagged a naming/role overlap: Seba already holds **architectural mapping sovereignty** per the deity registry. Resolved as a clean three-way split:

- **Thoth** = memory + intent + plans (the *why* and *what next* — this file)
- **Seba** = architectural map (the canonical *layer/topology* — deity-owned, lives in `internal/seba/`)
- **Understand-Anything** = semantic verification (the auto-derived *what exists* + *what imports what* — lives in `.understand-anything/`)

Three artifacts, three jobs, no overlap. Understand is the verifier, Seba is the architect, Thoth is the historian.

### Bidirectional sync codified

- Added `## Knowledge Graph (Understand-Anything)` section to `memory.yaml` with artifact pointer, current stats, query commands, and a `sync_protocol` block.
- This journal entry serves as the first delta record. Future `/understand` runs should append a similar entry summarizing what changed (new packages appeared, layer assignments shifted, edge counts moved).
- Added rule to global `~/CLAUDE.md` instructing future sessions to maintain the bidirectional sync automatically.

### Verification gaps and notes

- Swift and Kotlin nodes are file-level only — tree-sitter Swift/Kotlin grammars are not bundled in the plugin's structural extractor, so per-function/per-class extraction is missing for iOS and Android code. The graph still captures their file relationships and architectural-layer assignment, but function-level call graphs for those languages are not in scope until upstream adds those parsers.
- 252 orphan nodes (markdown docs and standalone configs with no incoming or outgoing edges) — these are document-class nodes that the file-analyzers couldn't link to other artifacts. Expected for marketing pages, ADRs, and pure-narrative case studies.
- The graph was built from `HEAD` (`22ec913`); uncommitted changes in `.agents/idea-router/items/` and `state.json` are NOT reflected. Re-run `/understand` after committing those to refresh.

### Lessons

- **One global pnpm install can unlock dozens of cached plugins.** The `--config.dangerouslyAllowAllBuilds` flag is the right hammer for native-binary plugins; cleaner than per-package `pnpm approve-builds`.
- **The 5-concurrent guideline in `/understand`'s phase 2 is an artificial floor, not a ceiling.** With background dispatches and notification-driven progression, running 10–12 concurrent worked fine here. The bottleneck was per-batch LLM latency, not parallelism.
- **The polyglot ratio matters for graph density.** With 367 Go files producing 2,108 function nodes and 281 markdown files producing zero function nodes, the call/import graph is heavily Go-weighted. That matches reality (Go is the core) but means architectural-layer queries dominated by markdown look sparse on edges. Acknowledge in onboarding docs.


## 2026-05-26 — "Does It Work" Audit + 3 Silent-Failure Fixes

**Session goal:** verify the architecture shipped 2026-05-21/22 actually works end-to-end. User asked one question: "does it work?". Probe found three real failures, all silent. All three fixed in this turn.

### The four-day gap (May 22 → May 26)

After last session, dispatch.log shows the system was QUIET from 2026-05-22T16:12 to 2026-05-26T11:57. No items routed. No threads heartbeated by the daemon. No errors. The user opened a session today and asked the right question.

### Probes + findings

Sent a real router item, watched dispatch.sh respond. FSEvents fired correctly. dispatch.sh reported `agents fired: 0` even though `sirsi router pull claude-pantheon` clearly returned the item. Root cause: launchd plist had no `WorkingDirectory`, so cwd=`/`, so `router.FindRepoRoot()` walked up from `/` and found nothing → pull returned empty → awk extracted no ids → dispatch silently no-op'd.

### Commits shipped (all on origin/main)

- `f5cd429` `fix(router)` — dispatch.sh `cd $REPO_ROOT` upfront so FindRepoRoot resolves regardless of how the script is invoked. Self-contained beats relying on plist hygiene.
- `75e68fe` `feat(thread)` — `reapDeadPIDThreads()` in cmd/sirsi/threadcmd.go. Auto-reaps orphan CTR threads whose PIDs no longer exist on this host (syscall.Kill(pid, 0) == ESRCH → mark closed). Hooked into `sirsi thread list` so the read IS the event. No daemon, no polling, no new verb. Per AGENTS.md §Lean #1 + #4. Verified by sweeping 2 real orphans (`thr-4990a8df4cbd1468`, `thr-f582c02ec658042a`) from the 2026-05-21/22 session.
- `2111423` `fix(router)` — dispatch.sh fails loud on `sirsi router pull` errors. Captures exit code + stderr. Distinguishes "queue empty" from "pull broken" in dispatch.log. Same pattern applicable to any future failure that would otherwise hide as `agents fired: 0`.

### Architectural lesson

**"The loud failure is the gift" only holds if "no work" and "missed work" look different.** dispatch.sh logged identically for both states — 4 days of silent failure. The third commit (`2111423`) is the *generalization* of the first (`f5cd429`): not just fix the bug, fix the mechanism that hid the bug. Recommend Codex's `ctr-thread-wake` automation adopt the same separation — its stay-quiet prompt should explicitly NOT stay quiet on read failures.

### Verification methodology validated

The user's question "does it work" was the single highest-leverage prompt of the session. Three bugs fell out of one probe. Lesson for future sessions: don't trust "tests pass" or "FSEvents fired" as proof of end-to-end function. Send a real item, watch what happens, audit the log. Reality > telemetry summaries.

### Still pending (async by design, not broken)

- 5 sibling adoption-acks (`claude-finalwishes`, `claude-assiduous`, `claude-nexus`, `claude-porch-and-alley`, `claude-homebrew-tools`) still `open` after 4 days. Those repos haven't had a claude session opened. Architecture is sound; cross-repo adoption observability is bottlenecked on session activity.
- 8 items in `items/` with empty `to:` field — direct file writes bypassing `sirsi router send` validation. Operator-error from senders, not router bug.
- One open ask to Codex: should plist hygiene fall under Lane A (codex-owned router delivery / queue health), or is workstation config out of router scope? The 4-day silent failure proves we should formalize plist ownership in the lock table.

### CTR state at session end

- 0 active claude-pantheon threads (all 5 historical sessions now closed, 1 reaped by the new reaper)
- Probe items all closed
- pending[claude-pantheon] = 0; pending[codex-pantheon] = 1 (the audit insights I just sent)
- launchd `com.sirsi.idea-router` loaded, idle, waiting for FSEvents

### Lessons indexed for next session

1. Always cd to known location before running sirsi from a script.
2. Folded periodic cleanup (reaper) into read-paths is the right shape — no daemon needed.
3. Silent failure separation is a generalizable lean pattern: anywhere a primary check can return "empty" for either legitimate or broken reasons, distinguish them in the log.
4. Real probes catch what telemetry misses. "Does it work" is the user's most valuable prompt.

---

## 2026-05-31 — Agent Work Safety Governor

**Context:** During a Sirsi Nexus assessment, an agent analysis path ballooned to roughly 135 GB of application memory and crashed the working environment. The user correctly called out that this is the exact failure class Pantheon is supposed to prevent.

**Decision:** Unified existing Pantheon safety primitives behind the `sirsi agent` surface:

- `internal/agentguard/` added as the composable governor.
- `sirsi agent preflight [command...]` checks system/resource state and command policy before work.
- `sirsi agent safe-run -- <command...>` executes only after preflight, with timeout and RTK output budgets.
- `docs/AGENT_WORK_SAFETY.md` records the crash lesson and the first policy set.

**What was reused:** `internal/yield` for CPU pressure, `internal/guard` Doctor for RAM/swap/Jetsam/process facts, `internal/rtk` for output filtering, and the existing `sirsi agent` command namespace. Vault/Horus remain adjacent primitives for future large-output storage and structural code inspection.

**Initial policy:** block unbounded `$HOME`/`~/Development` scans, direct `.codex/sessions/*.jsonl` reads via `cat`/`rg`/`grep`/Python, and Python-based repo/transcript-wide analysis without explicit budgets.

**Verification:**

- `go test ./internal/agentguard`
- `go test ./cmd/sirsi`
- `go build ./cmd/sirsi`
- Live smoke: `sirsi --json agent preflight -- cat .../.codex/sessions/...jsonl` returns `verdict: block`.
- Live smoke: `sirsi --json agent safe-run --force ...` runs with output budget while still reporting current Jetsam blockers.

**Lesson:** Pantheon already had the organs: Guard, Yield, RTK, Vault, Horus. The missing piece was the front door that forces agent work through those organs before the machine is under pressure.

---

## 2026-05-31 — Router Thread/Item Relationship Index

**Context:** After the crash recovery, the router needed a clear answer to which Claude thread owns which router item. `sirsi thread list` showed no active registered threads; `threads.json` contained only closed/reaped `claude-pantheon` sessions.

**Decision:** Added `.agents/idea-router/THREAD_ITEM_INDEX.md` as the canonical relationship index. It records:

- current open item ownership by `to` agent
- `thread_unassigned` for every open Claude repo-agent item until a live thread registers
- historical Pantheon thread/lane provenance for Lane B, Lane C, ADR-020, and LEAN AF coordinator sessions
- the rule that thread ownership is established only by an active registered thread heartbeating `current_item=<item-id>`

**Router cleanup:** Reconciled `state.json.pending` to match open item frontmatter for `claude-assiduous`, `claude-finalwishes`, `claude-homebrew-tools`, `claude-nexus`, and `claude-porch-and-alley`; corrected `pending_for_user` to the actual open Development-root decision.

**Verification:** `sirsi router status` reports 20 open / 46 closed, with no `codex-pantheon` inbox items and no blank-recipient bucket.

---

## 2026-05-31 — Codex Router Reviews Closed + Dispatch Race Fixed

**Context:** Terminal showed multiple live Claude windows while CTR still showed no registered threads. Claude then routed three `codex-pantheon` items: dispatch race, ADR-020 canon-correction v2, and `sirsi thread discover` Phase 1 review.

**Decision / Work Completed:**

- Patched `.agents/idea-router/dispatch.sh` with per-agent lock directories under `.agents/idea-router/locks/` so WatchPaths bursts cannot spawn sibling workers for the same inbox.
- Removed the Python legacy-pending reader from `dispatch.sh`; it now uses `jq` when available and otherwise relies on pull-model item frontmatter.
- Approved ADR-020 canon-correction v2 and made one follow-up line edit in `docs/ADR-020-INTERACTIVE-SURFACE-REOPENED.md` so it no longer says the changelog "needs" correction after the correction landed.
- Approved `sirsi thread discover` Phase 1. Phase 2 hook scope is approved with the constraint that hooks call `sirsi thread discover --self` and do not enumerate process tables.

**Router Artifacts Closed:**

- `20260531-codex-pantheon-dispatch-concurrency-guard-review.md`
- `20260531-codex-pantheon-adr020-canon-v2-approval.md`
- `20260531-codex-pantheon-thread-discover-phase1-approval.md`

**Verification:**

- `bash -n .agents/idea-router/dispatch.sh`
- `bash -n .agents/idea-router/sweep.sh`
- `git diff --check -- .agents/idea-router/dispatch.sh docs/ADR-020-INTERACTIVE-SURFACE-REOPENED.md`
- `go test ./internal/router ./cmd/sirsi`
- `go test ./internal/agentguard ./internal/router`
- `go build ./cmd/sirsi`
- `sirsi router pull codex-pantheon` => no open items
- `sirsi router status` => 20 open / 50 closed

---

## 2026-05-31 — Process Scout Registry

**Context:** User clarified the desired bar: every IDE, terminal, agent, PID, and process should be known to Pantheon automatically. If a process cannot register as a router thread, Pantheon should still scout the machine and know it exists.

**Decision:** Added a read-only process awareness registry separate from CTR thread ownership:

- `internal/router/processes.go` and tests define `ProcessRegistry`, `ProcessRecord`, role classification, and reconciliation preserving `first_seen` while marking missing PIDs `gone`.
- `sirsi thread scout` records the visible process table into `.agents/idea-router/processes.json`.
- `.agents/idea-router/sweep.sh` now refreshes both `sirsi thread discover --json` and `sirsi thread scout --json` automatically.
- Removed the old Python parser from `sweep.sh`; watcher validation uses `jq`.

**Important boundary:** `threads.json` remains for agent sessions that can own router work. `processes.json` is the broader host awareness map for every visible PID. Pantheon observes broadly, but process control remains gated through Guard/Throttle/Slay and explicit safety rules.

**Live smoke:** Escalated host run of `sirsi thread scout --limit 12` saw 831 visible processes on `Mac.lan`: 18 agent, 2 IDE, 30 terminal, 30 system, 751 process. It captured the live Claude/Codex PIDs that the screenshot showed.

## 2026-05-31 — Runtime Restore After OOM + ADR-021 (Deities ≠ Single-Repo)

**Context:** User's Mac crashed from application-memory exhaustion; Pantheon (which they expected running) was gone. No LaunchAgent/login-item ever made it auto-start, so every reboot killed it.

**Restore:** Rebuilt v0.22.0-beta from source (`make build`, `build-menubar`, `bundle`). Found `guard.StartBridge` is embedded in the menubar (`cmd/sirsi-menubar/main.go:388`) — so menubar and the Sekhmet RAM watchdog are ONE process, not two (no separate `guard --watch`; that flag no longer exists in v0.22). Installed fresh `sirsi`+`sirsi-menubar` to all PATH copies (checksums unified), loaded `ai.sirsi.pantheon` LaunchAgent (RunAtLoad+KeepAlive → reboot-persistent), registered `sirsi mcp` user-scope (✓ Connected). Caught a silent regression: first agent launch ran the stale May-11 brew binary because login-shell PATH put /opt/homebrew/bin ahead of ~/.local/bin — fixed by unifying all copies. Menubar verified live via screenshot (🟢 RAM 11%).

**Router:** Registered this thread `thr-7452fa9c16e656c9` (claude-pantheon, lane pantheon-runtime-restore) — had never registered. The two TUI items in the claude-pantheon inbox were MISROUTED (codex filed a MISROUTE NOTICE); closed the notice (the valid item), left the TUI correction for the intended thread.

**ADR-021 (proposed):** The menubar's `osiris assess failed` traced to `stats.go:84` `RepoDir: "."` resolving to launchd cwd `/`. User rejected the shallow "pin a repo" fix: *"Sirsi/Pantheon components are NOT restricted to repo management… recognize we have a design problem."* ADR-021 names the principle — workstation-scoped deities source scope from the CTR registry (`sirsi thread discover`, committed `10a97b7` same day), never cwd; Osiris becomes a workstation-wide risk aggregator; non-git degrades to benign. Routed to codex-pantheon for review; no code before acceptance. Committed `dd36ccf` (ADR + INDEX + CHANGELOG).

## 2026-05-31 — `sirsi thread discover` + codex round-trip (CTR auto-registration, Phase 1)

**Why:** "How many threads registered since reboot?" → zero. Root cause: registration was manual-only, so a reboot reaps every PID and nothing re-enrolls. Compounding it — the live sessions were all launched from `$HOME` (`cwd=/Users/thekryptodragon`, no `CLAUDE_PROJECT_DIR`), so they have no repo identity to register under. That is a real constraint, not a bug: a session in `~` is not a repo agent.

**Design (agreed with codex-pantheon via router, items `…195033` / `…210057`):** two complementary pieces — a SessionStart hook (push at birth, Phase 2) and `sirsi thread discover` (pull/reconcile, Phase 1). Codex confirmed it has no project-local SessionStart equivalent, so `discover` is its only registration path; it accepted the anchor-pid lifecycle (externally-registered threads bind to the discovered PID, reaped by the existing watcher when it exits) and `discover --self` as the shared hook entry point.

**Built (commit `10a97b7`, pushed):** pure `ReconcileDiscovery` in `internal/router/discover.go` — surface-scoped longest-ancestor cwd match; `unmappable` (home) and `ambiguous` (the genuine `codex-homebrew` vs `codex-homebrew-tools` cwd collision) are reported, never guessed (Rule A23). 9 unit tests, no real processes (Rule A16). CLI + bounded enumeration (`pgrep -x`/`lsof`, `--print`/`-p` worker filter to avoid a self-registration loop, `--self`, stable snake_case `--json`) in `cmd/sirsi/threaddiscover.go`. Live: `discovered=6 registered=0 unmappable=5 skip=1` — proved the premise and the already-registered skip path (an externally-registered repo thread was correctly skipped).

**Codex verdict — APPROVED.** Directives: keep the `--print` filter (note a future stricter interactive-session signal); wire `discover` into the sweep report-only (**Phase 1.5**); Phase 2 hook approved (must call `discover --self`, never broad process scans); live-delivery into a running session stays **Phase 3, spike-gated** (the local mechanism is Claude Code remote-control, not the claude.ai `RemoteTrigger` cloud API).

**Deployed + Phase 1.5:** built from the working tree and installed to `~/.local/bin/sirsi` (the install therefore also carries the parallel scout-lane's uncommitted `thread scout`). The hourly `sweep.sh` — already wired by the scout/runtime-restore lane to call `discover` + `scout` — runs PASS. ADR-021 (`dd36ccf`, parallel lane) consumes this primitive: workstation-scoped deities (Osiris et al.) source their repo set from CTR discovery, never `cwd`.

**Open / next:** Phase 2 (SessionStart hook → `discover --self`) approved, not yet wired. Phase 3 spike pending. Coupling to flag: the installed binary includes the scout lane's uncommitted code — that lane should commit `threadscout.go` + `sweep.sh` + friends and own a clean rebuild-install.

## Entry 036 — 2026-06-01 12:28 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"bafb166f-7d28-44f4-872f-6c2c49b47752","transcript_path":"/Users/thekryptodragon/.claude/projects/-Users-thekryptodragon/bafb166f-7d28-44f4-872f-6c2c49b47752.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","trigger":"manual","custom_instructions":null}
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-01T16:24:36Z
- last Claude read: 2026-06-01T16:12:19Z
- pending: none
- dispatch ledger: 2658 bytes, updated 2026-05-21 17:30:56

---

## 2026-06-02 — ADR-026 Horus ops-dashboard (proposed) + R4 capability inventory

**Lane:** claude-home, Horus ops-view content lane. Boundary ratified 2026-06-01 (items `235419`/`235652`): I own the ops-view content + read contract; claude-pantheon owns the surface chrome (CLI/TUI/menubar/macapp). Horus renders INTO their surfaces, not beside them.

**Finding — the gap is exposure, not computation.** `router.CollectNodeStatus()` (`internal/router/nodestatus.go`) already aggregates the entire operator read-model into one `NodeStatus`: registered agents + wake-health, router queue (pending-by-agent / topics / last reads), work-queue dispatch failures, live+stale threads carrying `os_state` OS-truth liveness (ADR-022), daemon health + binary-drift (ADR-023), and claude/codex CLI auth. It is complete and trapped in Go — **not** in the frozen dashboard contract (matrix row *Router ack → MISSING, no `/api/router/*`*), **no** CLI verb (Rule A27 canon references `router node-status`, which does not exist), **no** surface render (menubar hosts the dashboard server but paints none of it; TUI scaffold has no ops pane).

**ADR-026 decision:** promote `NodeStatus` to a frozen additive contract; serve it at typed `GET /api/node-status` (+ `?view=summary` → `OpsSummary` for the menubar); add `sirsi router node-status [--json]` (makes the A27-referenced verb real, --json shape == HTTP body); define menubar/TUI as read-only *projections* of the one read-model (no re-aggregation — the frozen-action-contract principle applied to reads). Read-only endpoint: zero destructive surface, nothing to confirm-gate.

**Challenged the framing (Rule A23):** the resume said "GET /api/horus," but `/api/horus/*` is already the code-graph namespace (`scan/query/report` → `SymbolGraph`/`WorkstationReport`). Reusing it conflates two Horus meanings, so the ops-view is `/api/node-status` (one name → one meaning). Recommended `/api/node-status`; flagged for claude-pantheon to override if their surface ladder needs `/api/horus`.

**R4 inventory:** `docs/HORUS_OPS_READMODEL_R4_INVENTORY.md` — the human-readable form of `watcherspec.go` ("the R4 capability inventory in code"): Part 1 the per-surface watcher-capability matrix (how each surface stays alive), Part 2 the ops read-model source + exposure ledger (what the operator can see vs. what's still trapped). R-mapping confirmed from ADR-025 status line: R1/R2/R4/R5 = ADR-024, R3 = ADR-025.

**Canon:** ADR-INDEX (+ADR-026 Proposed, 24→25, next 027), CHANGELOG (Unreleased/Added). Routed to claude-pantheon for review (item `20260602-021743`, type=review). Design-phase only — no code in either lane until codex + claude-pantheon bless the contract shape; then I implement steps 1-3 (contract+endpoint+verb), they implement 4-5 (surface render).

**Drift caught live:** the `sirsi` on PATH is v0.21.0 (no `router send --type` flag) while the repo is v0.22.0-beta — the exact ADR-023 binary-drift class. Flagged in the review item for the Decision-5 stale-Homebrew rebuild on claude-pantheon's ADR-024 follow-up plate; not self-fixed (its lane).

## 2026-06-02 — ADR-025 completed: Thoth-gated exit + suspend/resume/reconcile (R3)

**Context:** Core ADR-025 (`414142f`) had landed the `suspended` status and `SuspendThread`/`ResumeThread` lib primitives with 6 tests. This session finished R3 of the always-on supervisor: the CLI verbs, the SessionStart reconciliation gate, the exit hooks, and the 3 remaining acceptance tests.

**Built:**
- **`ReconcileExits`** (`internal/router/threads.go`) — the authoritative SessionStart gate (ADR-025 §4). Pure, host- and agent-scoped, with an injected `RetroSyncFn` (Rule A16). Stale-active → heal in place to suspended after retro sync; reaped (terminal, never revived per ADR-022) → mint a suspended successor carrying `reaped_from` if memory is recoverable, else a visible `UNRECOVERABLE` warning. Idempotent via `hasSuccessorFor` + a 24h `ReconcileReapedLookback` so SessionStart never re-mints or re-warns forever.
- **CLI verbs** (`cmd/sirsi/threadsuspend.go`): `thread suspend` (`--self`/`--thread`, `thoth sync` first for a fresh `thoth_ref`, snapshots owned open items from `state.json` Pending + resume prompt, kills the fs-watcher), `thread resume` (restores owned items, prints resume prompt, returns the ADR-024 `WatcherFor` spec to re-arm), `thread reconcile` (`--agent`, reaps dead PIDs first, then `ReconcileExits`; `SIRSI_SUPERVISOR=0` skips the managed action). `bestEffortThothSync` returns (commit-ref, ok) where ok = capture succeeded — the honest signal gating successor-vs-warn.
- **Hooks** (user-scope `~/.claude/settings.json`, ADR-024 §4 default-on): new `SessionEnd` → `thoth sync` + `thread suspend --self` (best-effort, visible error — SessionEnd cannot block); `SessionStart` 3rd entry → `thread reconcile --agent <id>` (the guaranteed gate). Both gated by `SIRSI_SUPERVISOR=0`.

**Tests:** +3 reconcile acceptance tests (stale-in-place, reaped-successor-then-warn + idempotency, host/agent scoping). 9 ADR-025 tests total, `go test -race ./internal/router` green; `go build ./...` exit 0. Verbs smoke-tested register→suspend→resume→close on a throwaway thread with the freshly-built binary.

**Deviation flagged for codex:** re-`register` matching a suspended record currently mints a *fresh* thread (codified by the shipped core test `TestRegisterThread_BypassesSuspendedFastPath`) rather than auto-adopting via the resume transition as ADR-025 §1 describes. Explicit `sirsi thread resume` is the supported resume path. Left as-is to avoid changing codex-reviewed core behavior; routed to codex-pantheon for a ruling.

**Operational note:** the PATH-installed `sirsi` is killed with exit 137 on a trivial `thread heartbeat` — the freshly-built binary works. Confirms ADR-023 binary drift; the user-scope hooks call PATH `sirsi`, so they only become functional after the rebuild+install follow-up.

## 2026-06-02 — ADR-024 Amendment 1 implemented: worker-lifecycle gate + (pid,start_time) reap-key

**Context:** claude-home CLAIM 024522 assigned claude-pantheon (sole writer) the ADR-024 amendment for CTR registration-hygiene findings (2) worker-lifecycle and (3) reap-key. claude-home APPROVED the design (025217). User directed "keep going until you finish," so implemented the approved design (doer→reviewer; routed implementation to codex). Finding (1)/menubar excluded per ruling 023813.

**(3) Reap-key — the systemic bug.** Bare-PID liveness can't tell a recycled/re-registered PID from the original. `internal/router/liveness.go`: `PIDStateOf(pid, startedAt)` (composite identity) + new `PIDRecycled` state (distinct from `gone` for diagnostics; `DeadByOSTruth` includes it) + injectable `pidStartFn`/`defaultPIDStart` (`ps -o lstart=`, mutex-guarded A21). `""` startedAt → bare-PID fallback (zero regression). `Thread.StartTime` captured at register via `PIDStartTimeOf`; `ReapDeadThreads` + `RegisterThread` fast-path key on the composite. Adopted claude-home note (b): one canonical `PIDStateOf(pid, startedAt)`, not a separate `PIDStateOfWithStart`. **No-false-reap guarantee** tested (the regression that reaped live sessions this session).

**(2) Worker gate.** `cmd/sirsi`: injectable `oneShotProbe` + pure `ephemeralWorkerSkip`; `register` refuses one-shot `--print`/`-p` workers (no-op, not error). Selective-gate test (claude-home note a) proves interactive surfaces still register under the same path.

**Tests:** `internal/router/adr024_amend_test.go` (PIDStateOf composite matrix, recycled-reaped, live-survives, composite fast-path) + `cmd/sirsi/adr024_amend_test.go` (selective gate). `go test -race ./internal/router ./cmd/sirsi` green; `go build ./...` exit 0; start_time capture smoke-verified via real `ps -o lstart`. Doc DRAFT→IMPLEMENTED.

**Routed to codex for review-of-code (doer→reviewer).** Pending codex on this + ADR-025 + binary-unification ruling (024046).

## 2026-06-02 — ADR-026 steps 1-3 shipped (Horus ops-dashboard read endpoint + node-status verb)

**Lane:** claude-home (Horus ops-view content). Design approved by claude-pantheon `20260602-022950` with two caveats both folded; surface chrome (steps 4-5) stays with claude-pantheon per the ratified boundary.

**The pattern that made this small:** the entire operator read-model already existed in `router.CollectNodeStatus()` — agents, queue, dispatch failures, live/stale threads with `os_state` (ADR-022), daemon + binary-drift (ADR-023), agent CLI auth. The gap was exposure, not computation. ADR-026 promised three thin wrappers; the code is exactly that:

- `router.NodeStatus.SchemaVersion = "1.0.0"` — one field + one constant + one stamp line in `CollectNodeStatus`. Surfaces decode tolerantly; bumps only on a breaking shape change.
- `internal/dashboard/nodestatus.go` — `GET /api/node-status` serves the typed shape directly (consumer→producer; dashboard imports router; no cycle). `?view=summary` returns `OpsSummary` — a **pure reduction** of the same NodeStatus (every field derived, nothing sourced independently — the action-contract principle applied to reads). Bounded to top-N=12 agents by pending+live signal with `more_agents` overflow row for the NSMenu budget (claude-pantheon caveat #2). Drift/auth roll-up sets `worst_icon` (🟢/🟡/🔴) for the menubar's lead row.
- `cmd/sirsi/routernodestatus.go` — `sirsi router node-status [--json]` wraps `CollectNodeStatus()`. `--json` shape is byte-identical to the HTTP body (one read-model, two transports). Default render is a styled human view (Rule A10). Closes the canon/implementation gap where Rule A27 references this verb but it never existed.

**Smoke-run reality check (live registry):** the verb surfaced **7 phantom `pid=0/os=unknown` claude-pantheon records** sitting "active" 40+ minutes idle — the exact ADR-024 Amendment 1 finding (3) PID-reuse / lost-anchor class. The verb didn't just compile; it's already the operator surface that proves Amendment 1 is needed. Self-validation.

**Race avoided (A21):** caught claude-pantheon mid-flight refactoring `PIDStateOf` to `(pid, startedAt)` in the same files I needed to touch. The `SchemaVersion` field add is in `nodestatus.go` (their lane file) — kept it minimal (1 field, 1 const, 1 stamp line; top of struct, not near their call-site changes). Did all other work in new files (`dashboard/nodestatus.go`, `cmd/sirsi/routernodestatus.go`, `dashboard/nodestatus_test.go`). Their refactor landed independently; my dashboard tests pass green.

**Verification:** 5 tests `internal/dashboard/nodestatus_test.go` `go test -race` green in 1.3s — full-contract serve, summary derivation (drift flag + needs-login projection), bounded truncation, 503 nil-collector, 500 collector-error. `go build ./...` ok. `sirsi router node-status` + `--json` smoke-run produce expected output against live registry.

**Next:** route to codex for arch-verify; notify claude-pantheon their surface chrome (steps 4-5: menubar `OpsSummary` rows + TUI 4th pane) is unblocked. ADR-INDEX flipped Proposed → Accepted with the impl-status note.

## 2026-06-02 — Binary unification (ADR-023) — user-authorized autonomous deploy

User authorized everything for an autonomous loop. Executed the long-gated ADR-023 binary unification: `make build` (v0.22.0-beta, commit a5fc1b4, stamped) → installed the single artifact to all 3 PATH copies (~/.local/bin, ~/go/bin, /opt/homebrew/bin) via atomic mv + ad-hoc codesign. All three now SHA-identical (dc532a39…). Resolves: (1) the broken ~/.local/bin/sirsi that SIGKILLed (exit 137) every bare-`sirsi` watcher workstation-wide — heartbeat via PATH sirsi now exits 0; (2) ADR-023 SHA drift (3 divergent builds → 1); (3) ships ADR-025 R3 + ADR-024 Amendment 1 reap-key/worker-gate + register-no-spawn + `router node-status` to the primary PATH binary. This is the deploy half of the session's code work.

## 2026-06-02 — Autonomous loop (user-authorized "everything until I return") — summary

User granted blanket authorization for an unending claude+codex loop. Drove every gated/approved item to completion:
- **Binary unification (ADR-023)**: all 3 PATH copies SHA-identical @ a5fc1b4; ~/.local 137 SIGKILL fixed (workstation-wide watcher die-off cleared); ADR-025/Amendment-1/node-status now on primary PATH.
- **Menubar redeploy + ADR-026 4b live-verify**: /api/node-status?view=summary serving real OpsSummary (17 live/7 suspended/🔴 drift); data path NodeStatus→Summarize→opsLeadRow/opsAgentRows confirmed end-to-end (only literal NSMenu pixels need a human glance).
- **ADR-025 suspended-retention (6b6b811)**: opt-in PruneStaleSuspended + --suspended-older-than (dogfooded from 7 orphan pid=0 suspends the ops-view surfaced). Live peer thr-fb73 improved the 0-case to a no-op (left for them to commit, A26).
- **Stranded-work (ADR-024 §6, fef3591)**: challenged claude-home's SessionEnd-stash (repo-global = A26 harm) → ruling ACCEPTED → implemented read-only SessionStart surfacing (reconcile emits ⚠ stranded-uncommitted on fresh reap + dirty tree); A18 is the prevention.

**9 commits.** codex OFFLINE all session (last active 2026-06-01) → 8 review items queued (ADR-025 024046, Amendment-1 032136, retention, stranded-finding+surfacing concurrence, binary FYI). Did NOT start the router daemon (auto-spawn cascade ADR-024 killed). Lesson: testing a mutating cmd (reconcile) on the live shared registry healed 20 stale-actives→suspended (benign, retention-cleanable; 2 live threads safe) — test mutating paths on temp registries, not live shared state. Settled into a disciplined watch-loop: heartbeat + inbox + read-only health-check, resume active work only on codex engagement / new mail / user return. Loop participants: me, codex (offline), live peer thr-fb73 (pro-ux-loop).

## 2026-06-03 — Menubar UX overhaul + mds_stores mitigation (session wrap)

Made the menubar ACT, not just inform (user's #1 complaint). Shipped: in-place actions (a2379ab — safe commands run + report to Recent Activity, no Terminal; destructive keep confirm path), function labels not deity names (39a0ec4), in-app two-click clean (b7040ff/154cb3b — dry-run preview arms a Confirm item, confirm pipes y to anubis clean --confirm), native ~/.Trash move replacing osascript-Finder which the launchd menubar can't get Automation TCC for (2710811), disk-visibility spectrum all/some/none (0f7a6a1 — CheckDiskAccess + menubar applyFDAState + FDA grant action). Live mds_stores write-amplification storm mitigated: excluded ~/Development from Spotlight (.metadata_never_index), pruned CTR registry 209→52 (threads.json 124KB→31KB); mds dropped to ~1.3%. BINARY FROZEN per user — no more rebuild+resign (each resign revokes FDA grants by changing the signature hash); durable fix = Developer-ID signing (Apple enrollment pending). Codex offline all session — reviews owed (ADR-025/Amendment-1/stranded/native-trash A1/menubar UX). Continuation: docs/CONTINUATION-PROMPT.md.

## Entry 037 — 2026-06-04 09:22 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"019e2256-daa1-7802-bb36-e7a00f0b635c","turn_id":"019e92c9-92fd-70e2-b849-33b23a6d8b83","transcript_path":"/Users/thekryptodragon/.codex/sessions/2026/05/13/rollout-2026-05-13T13-16-17-019e2256-daa1-7802-bb36-e7a00f0b635c.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","model":"gpt-5.5","trigger":"auto"}
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-04T12:57:35Z
- last Claude read: 2026-06-04T13:20:40Z
- pending: none
- dispatch ledger: 2658 bytes, updated 2026-05-21 17:30:56

---

## Entry 038 — 2026-06-04 16:36 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- sirsi fix heuristic resolver (no LLM) — answers every finding; safe PPID-narrowed orphan-kill (KillTrueOrphans, PPID<=1 only, --yes never kills, 4 regression tests). Funnel diagnose->fix + menubar BLOCKED pending codex re-review (42588a9).
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-04T20:35:52Z
- last Claude read: 2026-06-04T20:36:02Z
- pending: none
- dispatch ledger: 2658 bytes, updated 2026-05-21 17:30:56

---

## Entry 039 — 2026-06-04 22:54 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- Canonicalized machine (1 versioned signed sirsi, zsh completion, no drift); sirsi fix resolver + safe PPID-orphan-kill (funnel BLOCKED pending codex User-metadata gap); menubar zsh close-prompt fix (read _). Open: install wizard, orphan User fix, mds_stores sudo.
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-05T02:48:24Z
- last Claude read: 2026-06-05T02:51:31Z
- pending: none
- dispatch ledger: 2658 bytes, updated 2026-05-21 17:30:56

---

## Entry 040 — 2026-06-04 23:19 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"019e2256-daa1-7802-bb36-e7a00f0b635c","turn_id":"019e95cb-0fb7-7621-8396-bd62ca478bcc","transcript_path":"/Users/thekryptodragon/.codex/sessions/2026/05/13/rollout-2026-05-13T13-16-17-019e2256-daa1-7802-bb36-e7a00f0b635c.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","model":"gpt-5.5","trigger":"auto"}
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-05T03:19:52Z
- last Claude read: 2026-06-05T03:19:52Z
- pending: none
- dispatch ledger: 2658 bytes, updated 2026-05-21 17:30:56

---

## 2026-06-04 — Guided setup wizard for the Monday VC build

`sirsi setup` was three overlapping report-style commands (setup/initiate/permissions), none of which drove a fresh user to "ready." Rebuilt it as a single guided 3-step wizard (Dependencies → Full Disk Access → Agent wake) over a new shared `internal/setup/` engine — one engine, two surfaces (CDD #5): the CLI renders it and the menubar config row already spawns a terminal running the same `sirsi setup`, so they can't drift. Real TTY → prompts before each action (install tool, open FDA pane, "Press Enter once granted" re-check); pipe/file/dev-null/CI → report only, never opens System Settings or blocks. Fixed a clean-machine embarrassment: the dep list reported thoth-init/sync/compact as "missing" npm tools, but Thoth ships inside the sirsi binary — three false negatives a freshly-downloaded user would see. TTY detection moved to golang.org/x/term (os.ModeCharDevice wrongly classified /dev/null as a terminal and auto-opened Settings unattended). Engine is the single source of truth for main.go's scan-command FDA pre-check too. go build ./... + vet + go test ./internal/setup ./cmd/sirsi ./internal/router green. Commit ff8a448 on branch feat/setup-wizard (pushed). Open call for the user: whether a dedicated fullscreen TUI wizard screen is also wanted (gated by ADR-020 / TUI_DESIGN_PROOF) or the menubar→terminal path suffices. codex review owed (offline since 2026-06-01).

## 2026-06-04 — Surface-selectable install: one engine, swappable faces

Extended the Monday-VC setup work into the full surface model the user specified: the install flow is now "one engine, many faces." `internal/setup/surface.go` holds the Surface model (CLI/TUI/IDE/Menubar/GUI), per-surface install (menubar LaunchAgent ai.sirsi.pantheon, IDE via `claude mcp add`), and switching (ActiveSurface/SaveActiveSurface/LaunchSurface → ~/.config/sirsi/surface). `sirsi setup` Step 1/4 is now a multi-select surface picker (--surfaces csv, interactive, or all); menubar auto-installs on macOS. New `sirsi surface` / `sirsi surface use <cli|tui|gui|ide>` command. `scripts/install.sh` places sirsi-menubar from the archive and hands off to `sirsi setup` on a TTY. All three callers (install.sh, GUI/DMG installer, sirsi setup) drive the same engine — no drift. isTerminal() fixed repo-wide (golang.org/x/term, not os.ModeCharDevice which mis-classified /dev/null). Commits ff8a448 (wizard) + b009120 (surfaces), branch feat/setup-wizard, pushed. go build ./... + vet + go test ./internal/setup ./cmd/sirsi green.

Release-pipeline reality (the menubar-shipping question): goreleaser runs on ubuntu CGO=0 and CANNOT build sirsi-menubar (fyne/systray needs Cocoa+CGO) — adding it to the builds list would break releases. The menubar already ships via the macos-latest job's DMG (Pantheon.app, Developer-ID signed when secrets present → FDA grants persist). So: DMG path ships menubar+signed (the GUI install); install.sh curl path ships CLI only and needs a separate standalone sirsi-menubar release asset to carry the menubar (follow-up, unverifiable without a release). Open last-mile: (a) how the VC receives it Monday — DMG (recommended, signed, complete) vs curl script; (b) auto-run `sirsi setup` on menubar first-launch so "GUI install implements this" is literal (menubar already has a Configure→setup row, just not automatic); (c) goreleaser standalone menubar asset for the script path. codex review owed (offline since 2026-06-01).

## 2026-06-04 — Both delivery paths complete (menubar everywhere + first-run wizard)

User chose "Both": completed the menubar+wizard across DMG and curl paths. (1) menubar runs `sirsi setup` on first launch (marker ~/.config/sirsi/.setup-launched). (2) release.yml macOS job builds+Developer-ID-signs a standalone sirsi-menubar_<ver>_darwin_arm64.tar.gz asset (additive to the DMG step). (3) install.sh fetches that asset on macOS when not in the archive. Commit c4d4c15. Branch feat/setup-wizard fully pushed (ff8a448 wizard, b009120 surfaces, c4d4c15 both-paths). bash -n + YAML parse + go build ./... + go test green. UNVERIFIABLE-without-release: the standalone menubar asset + DMG artifacts only exist once a tag is pushed and the release workflow runs — the user must cut a release before Monday for the curl/DMG paths to carry the new binaries. Everything else is locally verified. codex review owed (offline since 2026-06-01); PR not yet opened.

## 2026-06-05 — The two missing surfaces are now real Go builds (TUI + Mac GUI)

User corrected priority: I was polishing the install/release wrapper around surfaces that didn't exist. Reality check: CLI + menubar were real; **TUI and Mac app were vaporware**. Built both as real, successful Go builds (Go-first per [[feedback_go_standard]]).

- **TUI (`sirsi tui`, commit 4f44dcd):** internal/tui was a rendering CONTRACT (AppState + pure Reduce + Renderer) with nothing to launch it. Added the live Bubbletea v2 (charm.land/bubbletea/v2) event loop in internal/tui/program.go — thin Elm adapter: key→registry→Reduce→re-render, tab cycles views, q quits, resize reflows. Renders the 3 canonical views (Scan/Ra Fleet/Router Inbox), fixture-backed for now. Headless tests + clean PTY run.
- **Mac GUI (`sirsi-gui`, commit d505c7b):** the ADR-015 "Ferrari" existed only on paper. Built Go-native (CDD #2: Go HTTP + embedded HTML) — webview_go/WKWebView window over the existing internal/dashboard server; reuses a running menubar dashboard on the fixed port. darwin build file + !darwin stub keeps Linux CI green. Builds: 17MB arm64 Mach-O; linux stub cross-compiles.

All four surfaces now build successfully: CLI (sirsi), TUI (sirsi tui), Menubar (sirsi-menubar), Mac GUI (sirsi-gui) — all faces over the same engine, switchable via `sirsi surface use`. Branch feat/setup-wizard, PR #2. Next iterations (follow-up, not blockers): wire live data into TUI views (currently fixtures); ship sirsi-gui in the DMG; richer GUI chrome. Then the install/release wrapper (which is already built) actually has four real things to install. codex review owed.

## Entry 041 — 2026-06-05 14:27 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"019e2256-daa1-7802-bb36-e7a00f0b635c","turn_id":"019e990a-1d54-76c1-856d-495983cbe571","transcript_path":"/Users/thekryptodragon/.codex/sessions/2026/05/13/rollout-2026-05-13T13-16-17-019e2256-daa1-7802-bb36-e7a00f0b635c.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","model":"gpt-5.5","trigger":"auto"}
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-05T18:27:37Z
- last Claude read: 2026-06-05T18:27:37Z
- pending: none
- dispatch ledger: 2658 bytes, updated 2026-05-21 17:30:56

---

## Entry 042 — 2026-06-05 14:55 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"019e2256-daa1-7802-bb36-e7a00f0b635c","turn_id":"019e9917-a605-7ae0-bc42-13da57ae5a60","transcript_path":"/Users/thekryptodragon/.codex/sessions/2026/05/13/rollout-2026-05-13T13-16-17-019e2256-daa1-7802-bb36-e7a00f0b635c.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","model":"gpt-5.5","trigger":"auto"}
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-05T18:50:23Z
- last Claude read: 2026-06-05T18:48:05Z
- pending: none
- dispatch ledger: 2658 bytes, updated 2026-05-21 17:30:56

---

## Entry 043 — 2026-06-05 16:12 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- {"session_id":"019e2256-daa1-7802-bb36-e7a00f0b635c","turn_id":"019e9968-8236-7091-be92-2b34dfae01e5","transcript_path":"/Users/thekryptodragon/.codex/sessions/2026/05/13/rollout-2026-05-13T13-16-17-019e2256-daa1-7802-bb36-e7a00f0b635c.jsonl","cwd":"/Users/thekryptodragon/Development/sirsi-pantheon","hook_event_name":"PreCompact","model":"gpt-5.5","trigger":"auto"}
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-05T20:12:32Z
- last Claude read: 2026-06-05T20:12:32Z
- pending: none
- dispatch ledger: 2658 bytes, updated 2026-05-21 17:30:56

---

## Entry 044 — 2026-06-05 17:41 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- Session compact handoff for codex-pantheon, 2026-06-05.
- Router/automation state:
- Codex app automation installed at `/Users/thekryptodragon/.codex/automations/ctr-thread-wake-pantheon/automation.toml`.
- Automation target app conversation id: `019e8b57-8bb4-7780-ae46-9055105579f9`.
- CTR thread id: `thr-4f39cd0e9caf5de0`.
- Agent id: `codex-pantheon`.
- Automation cadence: every 2 minutes.
- Temporary LaunchAgent liveness bridge also exists at `~/Library/LaunchAgents/ai.sirsi.codex-pantheon.heartbeat.plist`, but the product target is one Horus supervisor, not per-thread glue.
- Important correction:
- The LaunchAgent only heartbeats/logs/pulls. It did not autonomously make Codex act.
- The Codex app automation is the closer equivalent to Claude loop behavior: scheduled wakeups, not a native Claude-style `/loop`.
- Completed this session:
- Sent Claude Pantheon product mandate for Ra/Horus agent-router supervisor.
- Sent superseding source-audit response to Claude Pantheon; earlier `201616` item was shell-quoted badly and should be ignored in favor of `201659`.
- Reviewed and blessed Claude's clean-safety evidence after source review and focused tests.
- Closed stale Codex router cleanup item and clean FYI.
- Focused verification passed: `go test ./cmd/sirsi ./internal/cleaner ./internal/platform ./internal/jackal`.
- Open/new work:
- New router item addressed to `codex-pantheon`: `20260605-213212-claude-pantheon-codex-pantheon-green-light-build-sirsi-horus-supervise-now-i-build-the-setu`.
- Title: `GREEN LIGHT — build sirsi horus supervise NOW; I build the setup-install side in parallel`.
- Claude asks Codex to build `sirsi horus supervise`: resident loop that inventories local agents from `agents.json`, registers/refreshes live threads, heartbeats every 60s, pulls Ra inboxes for locally-owned agents, and marks surfaces wakeable/stale/blocked/manual honestly.
- Claude will build setup/install side in parallel: `internal/setup.InstallSupervisor()`, LaunchAgent `ai.sirsi.horus.agent-router`, setup step, node-status supervisor health.
- Agreed contract: command `sirsi horus supervise`; LaunchAgent label `ai.sirsi.horus.agent-router`; plist path `~/Library/LaunchAgents/ai.sirsi.horus.agent-router.plist`; runs from repo root; no `/Applications` writes; stale daemon language remains dead.
- Next action after compact:
- 1. Open the green-light router item again if needed.
- 2. Read `cmd/sirsi/horus.go`, `cmd/sirsi/routernodestatus.go`, `internal/router/*`, especially thread registry, watcher spec, node status, and service/supervisor helpers.
- 3. Implement `sirsi horus supervise` with `--once` and foreground loop behavior.
- 4. Keep edits scoped; do not collide with Claude's setup/install side.
- 5. Verify with focused Go tests and route implementation result back to `claude-pantheon`.
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-05T21:41:23Z
- last Claude read: 2026-06-05T21:41:23Z
- pending: none
- dispatch ledger: 2658 bytes, updated 2026-05-21 17:30:56

---

## 2026-06-05 — Resident Horus agent-router supervisor: integrated + LIVE

Resumed "Pantheon CLI shippability" on branch `feat/horus-supervisor-install` (69a4577).
Goal: integrate the two parallel-built supervisor halves and take the resident
supervisor live. Done.

- **Integration**: codex's `sirsi horus supervise` engine arrived as untracked
  files in the shared working tree (`internal/router/supervisor.go` +
  `supervisor_test.go`) plus uncommitted `cmd/sirsi/horus.go` wiring. No PR — the
  files were dropped in directly. My install side (`internal/setup/supervisor.go`,
  `surface.go`) was already committed at 69a4577, with a guard that skips the
  LaunchAgent until `sirsi horus supervise --help` succeeds (menubar exit-127
  lesson). Both halves co-resident → build/vet clean, supervisor tests green.
  Committed engine + wiring scoped (3 files) as `8997d6d`, co-authored to codex.
- **Go-live bug #1 (AMFI/codesign)**: `cp`-over-existing the fresh binary onto
  `~/.local/bin/sirsi` → SIGKILL 137 on exec (byte-identical to a working /tmp
  copy). Cause: kernel cached a code-directory hash for the inode; new bytes fail
  validation. Fix: rm (new inode) -> cp -> `codesign --force --sign -`. This is the
  killed-on-exec class (reference_a27_watcher_binary_drift) and matters because
  launchd re-execs the supervisor binary at every login.
- **Go-live bug #2 (PATH)**: first live run marked every claude/codex agent
  `blocked — claude not found in PATH`. launchd's minimal PATH + `/bin/zsh -l`
  (non-interactive login skips .zshrc where ~/.local/bin lives) -> exec.LookPath
  fails. Fixed the plist to export explicit EnvironmentVariables/PATH leading with
  the binary's own dir. Committed as `43f625f`; added supervisorPath() + tests.
  Reload -> claude-pantheon flips to `wakeable pending=5`.
- **Result**: `sirsi setup` Step 4 installs+loads `ai.sirsi.horus.agent-router`
  (repo-root cwd, KeepAlive, Background, no /Applications writes). Verified running:
  registers its own thread, status=active live=85 stale=1, surfaces each agent's
  inbox. One resident process replaces per-thread Monitor + /tmp glue.
- **Honesty (re status-correction 20260605-191735)**: this completes codex's
  "Monday gap #1" (productize supervisor) ONLY. The broader Monday-ready package
  audit (footprint, dead code, docs drift, clean-safety evidence) remains OPEN —
  codex's lane. Not claiming package-audit done.
- SuperviseOnce is read-model: it does NOT auto-wake or deliver to a live claude
  session, and it flags my own work thread stale when I lapse — honest by design.
  Refining wake-delegation is codex's productization lane.

## 2026-06-10 — Flagship train landed (19 PRs) + two governance fixes (canonical router + binding-hold gate)

**Session (claude-pantheon, source-edit lane; claude-home = binding reviewer while codex OOO→back ~20:30 EDT).** Drove the entire binding-passed health-surface flagship + foundation to `main`, then closed the two relay failure-modes the session itself exposed.

- **19 PRs merged** (each rebased onto a moving `main`, CI-green per merge, squash, dependency order): A28 registry-trust cluster (#24 SessionStart per-resume-mint fix + ADR-029 per-agent worktrees, #25 terminal-compaction, #29 pid-sanity-floor, #30 surface-agnostic loop-evidence); flagship Rails (#19 Rail A binary-drift self-heal w/ Homebrew-guard + A21 mutex, #22 Rail B Spotlight-storm detect, #18 Rail C Jetsam/panic trend); both of Cylton's direct UX pains (#26 menubar `.app` bundle so TCC/FDA survives reinstalls, #27 fsnotify live-refresh killing the 4h-stale label); gemma chips (#11 MLX install, #13 sirsi-gemma 2-tool `NewBareServer`); #31 menubar visible-feedback + safe-only manifest; #21 scan-truthfulness; #33 AI-caches→CAUTION (caught 30.7GB HF one-click-trash); #9 ADR-028 SQLite-lean; #28 fresh-inode AMFI invariant docs; #14 gate-flake; #34 `sirsi insight` cross-deity SOTU (AI-optional, structural: `internal/insight` cannot import `internal/gemma`).
- **The recurring work** was not the CHANGELOG unions (mechanical, `union_resolve.py`) but the **test-file collisions**: two PRs each appending a new test func right after the same existing one share the `for _, tt := range tests { t.Run(...` skeleton → git interleaves them. They're additive — reconstruct BOTH funcs whole, never pick a side. #22 added a wrinkle: it predated #18 and carried a stale duplicate `checkRecentCrashLogs` → drop its copy, keep main's.
- **Two silent failures surfaced + honestly flagged**: (1) #33 (safety-tier A1) **auto-merged before its binding verdict** when a rebase force-push tripped armed auto-merge (the `[auto-merge overrides hold]` mode — fires on rebase-push, not just first-push). claude-home post-hoc PASSed it (strictly safer, no revert) and RULED safety-tier must never auto-merge pre-verdict. (2) **Router fragmentation**: `sirsi router` from a worktree cwd read the worktree's STALE git-snapshot copy of `.agents/idea-router/`, not the live root — my first #33 review request to claude-home was silently dropped into a worktree copy (audit: exactly one lost item).
- **Fix 1 — canonical router (#35, `4eb6792`, ADR-029 Amendment 1)**: `router.FindRepoRoot` resolves the MAIN worktree root first (shared `.git`'s parent via `git rev-parse --git-common-dir`), trusting it only when the live router lives there, else the original cwd walk-up. Injectable git seam (`gitCommonDirFn`, A16) + RWMutex (A21); 3 tests; dogfooded (fixed binary from a worktree → `Router home` = repo root).
- **Fix 2 — binding-hold gate (#36, `aa41706`)**: native GitHub auto-merge respects required CHECKS, not labels — so a `binding-hold` label alone can't hold a PR. New `.github/workflows/binding-hold.yml` job `binding-hold` passes unlabeled / FAILS when labeled; added (the JOB name) to `required_status_checks` → branch protection blocks merge (auto-merge waits) until a binding reviewer removes the label. Own workflow so relabel re-runs only the gate. Proven live end-to-end: label #35 → gate FAIL → `mergeStateStatus: BLOCKED`; claude-home (reviewer) cleared the label → re-run → CLEAN → merged. Closes the #33 bypass STRUCTURALLY. Extends the Ma'at gate (A25/A28).
- **Net**: a gate (#36) AND the relay that feeds it (#35), both structural. Remaining open: #8 (router −2,626 LOC, HOLD-FOR-CODEX, `binding-hold` labeled) + #32 (ADR-030 NSPopover Swift, arch-PASS but operator-GUI + fresh-codex gated, `binding-hold` labeled). **Deploy-pending**: installed `sirsi` binary still predates the canonical-router fix (repo-root-cwd workaround holds the relay meanwhile) — goes live on next user-authorized build/reinstall, with the TCC-bundle + self-update deploys.
- **Lessons**: test-file rebase collisions ≠ pick-a-side (reconstruct both); auto-merge-overrides-hold fires on rebase-push too (gate safety-tier with a required-check label, don't rely on discipline); per-worktree router copies fragment the relay (resolve-to-root). Co-authored to claude-home (binding) + codex (arch post-review on return).

---

## Entry — 2026-07-04 — "The Watchers Were Healthy; the Executor Was the Disease"

**Context**: claude-pantheon session d8b52186. The runaway-executor incident (19,195 sessions/0 closed; 11,564-item flood; 1.3 TB of orphaned build trees → ENOSPC) became canon and got its guards.

**Landed**: #162 fabric-board truth (launchctl `list` probe + real default checker); #163 incident canon (case study + ADR-035 + Sekhmet "Runaway Executor" doctor check + `sirsi router quarantine-worker`); #164 Router-v2 Phase 2 — the §2b Dispatch Contract implemented verbatim in internal/routerstore (fenced leases, send-facade idempotency+quotas, keyed singletons AS DATABASE INVARIANTS, breakers, budgets, <250ms event-driven Wait) with acceptance-bar tests reproducing both incidents under -race; #165 same-day threshold recalibration (a heavy dev day = 389 young trees; warn was 300 → amber on healthy work; now 1500/4000).

**The deeper lesson, again**: every layer that lied to the owner today was an UN-VERSIONED sidecar (the worker script, the board jq that dropped `legacy`). ADR-035 axiom: no executor around the store. Same law, smaller organisms. Board-writer adoption filed as follow-up.

**State**: worker OFF (quarantined durably); re-arm gates on Phase 3 (one facade over the store for CLI+MCP). rc1 evidence: Ma'at 100/100, CI green, TUI merged — tag remains owner-gated.

---

## Entry — 2026-07-07 — "Point Ma'at at the Repo (and prove the popover without a display)"

**Context**: claude-pantheon. After #170 the menubar's Ma'at — Quality and Net — Plan honestly said "unmeasured — not inside a code repository" — correct, because run/runJSON pin cwd to $HOME (the launchd-cwd=/ disk-walk fix). The owner-visible gap: no way to see real repo scores from the popover.

**Landed (#172)**: optional project root. `SirsiEngine` reads UserDefaults `projectRoot` (settable via `defaults write ai.sirsi.pantheon projectRoot -string ~/Development/sirsi-pantheon` or the new in-popover picker), validates it (existing dir + `.git`, worktree-tolerant), and applies it as cwd ONLY to the repo-scoped verbs (`maat`, `net`). ProjectBar on those drill-ins names the repo being weighed; home rows show it; "None" returns to the honest unmeasured default. CommandResult decoding treats absent evidence/next_actions as empty so honest-unmeasured JSON still renders structured. Picker is a Menu, not NSOpenPanel — a modal panel dismisses the transient popover.

**The QA lesson worth keeping**: display-gated verification WITHOUT the display. Screen Recording TCC and computer-use were unavailable, so `SirsiMenubar --snapshot <dir>` now renders home/Ma'at/Net to PNGs. Three dead ends first: (1) NSHostingView + cacheDisplay CANNOT composite SwiftUI render-server content — captures come back with only tint-colored glyphs, at any window alpha; (2) ImageRenderer never runs `.task`, so self-loading views render as an eternal spinner → pre-fetch the CommandResult and inject via a new ResultView initializer; (3) ImageRenderer draws ScrollView viewports EMPTY → a `snapshotMode` environment flag swaps scroll containers for plain stacks. Proof PNGs: Ma'at weighing sirsi-pantheon at 51/100 with heal/pulse/scan actions; Net reading the build log (10 entries).

**State**: merged (auto-merge on green), app rebuilt via build-app.sh (stable cert, FDA intact) and relaunched; projectRoot set on the owner's machine. Post-review routed to claude-home (item 20260707-145232).

## Conduit run 2026-07-15T01:26Z–01:46Z — owner decided "A scoped as C"; identity-enforced bind shipped

Queues for claude-home and claude-codex-standin were empty; threads healthy (0 OS-dead, 10 armed); prune reclaimed 3.6 KiB; board republished. The substantive work came from the owner answering the open bind-enforceability decision with **"A, scoped as C"** — a second identity makes bind real, scoped to authority-model paths only.

Implemented on PR #218's branch as `1a85436d` rather than a second held PR (one PR, one bind). The key insight beyond #218: #218 made the *hold* real but cleared it with a `bound` label **its own author could apply** — every agent authenticates as the single account `SirsiMaster`, so any marker an agent can apply, the author can apply. A label is an honesty marker, not a gate. Bind is now pinned to the one primitive an author cannot forge — GitHub forbids self-approval — so `binding-hold` requires an APPROVED review from a login != the author, pinned to the current head SHA (approve-then-push drops the bind; that is the #207 loss class). The approver is the `sirsi-bind` GitHub App, whose private key lives only on the conduit host and **never** in GitHub Secrets: a key reachable from a workflow lets any PR mint the token and self-approve, restoring exactly the circularity ADR-041 removes. Deliberately NOT triggered by `pull_request_review` — such a run resolves its SHA against the base branch and its check would land on the wrong commit, blocking the PR forever; `sirsi-bind.sh` re-runs the PR's own gate run instead. Assuming event mechanics rather than verifying them is precisely what shipped #217.

Verified live rather than asserted: the gate holds #218 against itself (`BLOCKED`, naming author + head SHA), GitHub refused a self-approval attempt (`Review Can not approve your own pull request`), the `bound` label no longer exists, and 6/6 selection cases pass via a test that extracts the jq filter FROM the workflow so it cannot pass against a rotted gate. Two things corrected in passing: **PANTHEON_RULES A28 still enshrined `enforce_admins=false`** — the #213–216 root cause, live-fixed earlier but never corrected in canon — now amended to `true`; and the runbook's claim that `.gitignore` covered `*.pem` was false, so the claim was made true instead of deleted.

Superseded and closed two stale claude-pantheon items with a fresh inbound (close is audit-only): its F1 blocker was correct against `a7dedc00` but is resolved by abolishing `bound`, not by creating it; and its #218 bind request is now unfulfillable because pantheon is also `SirsiMaster`. Told it to stand down rather than burn cycles. One owner item routed: the ~5-minute `sirsi-bind` App creation, which is an access-control action an agent must not perform — and which an agent doing unilaterally would defeat the separation the App exists to create. Canon: ADR-041, PANTHEON_RULES A28, ADR-INDEX (next available advanced to ADR-042).

## Conduit run 2026-07-15T05:39Z–05:45Z — first-chop found canon contradicting its own gate (#218)

Queues empty for claude-home and claude-codex-standin; all threads heartbeating (0 OS-dead, 10 armed, 0 woken); prune reclaimed 5.7 KiB; board republished; no BINARY_MISSING sentinels. The two open `user` items (sirsi-bind App setup, Assiduous Stripe secrets) are genuine owner actions — left open, not nagged. The two stale claude-pantheon items belong to a thread that is alive and active, so they stay with their recipient.

The one substantive finding came from source-deep review of **PR #218** (the only open PR portfolio-wide, correctly `BLOCKED` by its own gate). **ADR-041 point 4 asserted "`pull_request_review` is a trigger on the gate" — `binding-hold.yml` deliberately excludes it**, and explains in-file why adding it would break the gate: a review-triggered run resolves `GITHUB_SHA` against the BASE branch, so its check lands on the wrong commit and the PR's required check never updates. Canon was instructing a future maintainer to make exactly the unverified-event-mechanics assumption that shipped #217 — the precise failure ADR-041 exists to close, and worse in canon than in code because canon is what gets trusted. Fixed in `f023e929` on the #218 branch: the true half is kept (App-token events DO re-trigger, unlike `secrets.GITHUB_TOKEN` — that asymmetry is why a review works where #217's bot label did not), the false conclusion replaced with the real mechanism (`sirsi-bind.sh` re-runs the PR's own `pull_request` run). Pushed **before** any bind exists — the head-SHA pin means a post-bind push silently drops the bind, so this was the free window. Verified rather than assumed: 6/6 bind-selection cases pass against the filter extracted from the live workflow, and the gate re-ran on the new head (`f023e929`) and still holds #218 against itself.

**#218 was not bound and not merged, structurally rather than by choice.** claude-home authored it, and every agent authenticates as `SirsiMaster` = the author; GitHub forbids self-approval, so no agent can bind it — `--admin` would be the #213–#216 self-merge class. It waits on the owner's `sirsi-bind` App (item `20260715-014538`, already open). Farmed ONE scoped SME item to codex-pantheon (`20260715-054351`): single question — can an authority-model PR reach main without an independent head-SHA approval — over exactly two files, with the settled identity decision, the ADR prose, and any bind/merge attempt explicitly out of scope. The first-chop defect is itself the argument for that second lens.

### Addendum (05:52Z) — codex-pantheon found the gate did not cover its own approver

The scoped SME farm-out returned **FAIL — bypass found**, and it was right. `binding-hold.yml`'s authority-model regex covered `.github/`, `cmd/sirsi/`, `internal/router/`, `PANTHEON_RULES.md` and `docs/ADR-*` — but **not `scripts/bind/`**, which holds `sirsi-bind.sh`: the script that mints the App token and records the bind. A PR editing only that script classified `sensitive=false`, skipped the independent-bind step, and reached a **green required check with no non-author approval** — free to redefine what a bind means, or which key file it reads, with no bind of its own. The enforcement had a seam its own authors could walk through, which is the same class as the #217 hole #218 was written to close. Two nights running, the gate was defeated by the part of the system nobody pointed the gate at.

Fixed in `0c36c929` — `scripts/bind/` added to the regex, the gate's error message, ADR-041 §Decision 6, PANTHEON_RULES A28, and the runbook, all in lockstep (canon drifting from the gate is what this same run already had to fix once, in `f023e929`). Scope covers the directory, not just the one file: `binding-hold-selection.test.sh` pins the gate's own selection logic and belongs behind the same bind. Verified: `scripts/bind/*` → SENSITIVE, ordinary paths → autonomous, 6/6 cases pass, gate still holds #218 against itself on the new head. Codex's other four angles (fail-closed detection, head-SHA pin, gate-open condition, token minting) came back clean and match the first-chop read.

**The lesson worth keeping**: claude-home authored #218 and reviewed it source-deep, and still missed this — the author's own agent found a canon contradiction but not the hole in the enforcement boundary it had just drawn. The independent lens found it in five minutes. No-self-review is not ceremony; it is the only reason this is closed. **Self-inflicted, recorded honestly**: journaling the 05:39Z run to main via #219 while #218's branch carried its own unmerged entry conflicted `.thoth/journal.md` (`DIRTY`); resolved in `f256d41c` keeping both entries chronologically — next conduit run should journal onto the open branch, or land the journal PR last.

---

## Entry — 2026-07-21 — "The Toast Reaches the Owner (and the backlog sweep behind it)"

**Context**: claude-pantheon, owner directive "find every task from every conversation and ensure complete." Sweep found 6 open router items + 1 arriving mid-session; all closed with results.

**Landed (#267)**: owner-gated router items (board `owner_gated[]`, schema 1.1.0, claude-home's producer) now reach the owner as menubar toasts. Launch + 90s file-only board re-read → UNUserNotification per genuinely-new id (persisted `ownerGatedToasted` dedupe — no restart re-spam). Toast click deep-links (notification delegate → `pendingOwnerItemID` → RootView push) to a new Owner-action screen: full body via `sirsi router show`, refs revealed in Finder, Mark handled (confirm-first, provenance), decision reply via sirsi-respond.sh so the sender is actually woken. "🔑 Needs you" home row carries the count. Verified live: relaunch toasted the two real liveness-watch items.

**Sweep findings worth keeping**: (1) the Scan/Review/Clean unified workflow the 07-17 self-item asked for had ALREADY shipped as ScanCleanView — but its gemma-built companions (#254–#257) are merged and unreferenced: fold in or Rule-0 them next menubar pass. (2) A STALE `v1.0.0-rc1` tag exists on origin pointing at 2026-03-29 (Session 38) — the real rc1 cut must delete/move it first. (3) `sirsi router` works fine from $HOME cwd — no repo-scoping needed for the menubar's router verbs.

**State**: inbox zero for claude-pantheon; session thread registered thr-b8206c35ba3ca785 (A27) and closed at session end; report routed to claude-home.

---

## Entry — 2026-07-21 — "The Installed Popover Was Still Wearing Gold"

The owner's live screenshot showed two concrete regressions on Home: Pantheon identity and reclaim copy still used the legacy gold accent, while most supporting text inherited macOS `caption`/`caption2` sizes that were too small at the popover's normal width. The fix is intentionally scoped: identity moved to Pantheon emerald; subordinate storage copy is neutral; genuine amber warning dots remain semantic warnings. Home now sets explicit readable type floors—17pt identity, 15pt action and row titles, 13–14pt supporting copy—plus larger row glyphs and footer controls.

Proof was end-to-end, not source-only: the release target compiled, all 17 SwiftUI snapshot surfaces rendered, the signed bundle was rebuilt into `~/Applications/Sirsi Menubar.app` with the stable `ai.sirsi.pantheon` certificate requirement, and the stale process was terminated and relaunched from the new bundle. The headless ImageRenderer still omits macOS material/text compositing in parts of Home, a known limitation already recorded in the July 7 entry; it nevertheless confirmed the emerald accent and layout, while compilation, signing verification, and the live process path close the delivery loop.

### Correction — the yellow was status, not styling

The first interpretation was wrong: the owner was pointing to the 6.9 GB safe-to-reclaim value and Horus's amber problem indicator, not asking to replace Pantheon's gold visual identity. The emerald branding change was reverted. The typography work remained and was extended app-wide with Large Dynamic Type plus a 360pt design width.

Live inspection found the actual state. Autonomous mode was ON, but its loop consumes Horus diagnostic levers only; it does not consume Anubis safe-storage findings, despite the broad UI wording. A deliberate safe-only Anubis clean moved 158 items to Trash and reclaimed 7.5 GB; nine protected/error items were skipped. A fresh scan then reported zero safe bytes, so the storage figure clears on refresh. Horus was amber for a genuine memory condition: first a swap-death/leaked-session warning, then—after all 17 leaked sessions exited—a >4 GB Python memory consumer. The Eye must remain amber while a current Warn finding exists; hiding it would falsify health.

## Conduit run 2026-07-28T00:00Z — three runs of backlog, written the moment a clean checkout existed

The journal entry for the 23:52Z and 23:59Z runs was blocked twice for the same reason and not for laziness: `.thoth/journal.md` lives in the pantheon checkout, and that checkout has been parked on `fix/sirsi-gemma-bare-server-chipA` with 79 uncommitted files for hours. Writing there risks a foreign `git add -A` sweeping the entry into someone else's commit, which is a known leak class in this repo. This run took the obvious way around it: `.claude/worktrees/rtk-savings` is already a worktree on `main` and was clean, so the entry lands from there and the dirty checkout is never touched.

The backlog it covers. Seven stranded items were ACK-closed across the earlier runs, and both open claude-nexus items were closed the run after. The broker died at 23:53:55Z and the cause was not memory: pid 85829 took a SIGBUS at the stack guard with 87,444 frames on thread 1, the report printing `RECURSION LEVEL 87424` through `mlx::core::detail::compile_dfs` — one frame per compute-graph node, no depth bound, a 16 MB stack exhausted. The prompt cache was 3.59 GB *under* its 4 GB bound five seconds before death and memory was 88% free, so every instinct that reaches for Jetsam was wrong here. launchd restarted it in about a second as pid 75716, argv still carrying `--prompt-cache-bytes 4294967296`, and it has now been stable across two runs with the cache sitting at 2.57 GB. The forensics and the fix — `threading.stack_size(512*1024*1024)` at import in `~/.sirsi/gemma-capped-server.py`, a virtual reservation with no resident cost — went to claude-pantheon as `20260727-235714`. The escalation bar for the next run is a *second* `Python-*.ips` with `compile_dfs` in it, not a repeat of this one.

Two merges landed this run. PR #334 (`docs(prd): Sirsi v2 — from utility to application`) crossed the one-hour bar at 00:05:47Z by the clock and was squash-merged at 00:01:28Z once it did, all five checks green including binding-hold; it lands a *proposed* PRD carrying an owner-decision section, so merging it lands a proposal and not a decision. SirsiNexusApp #193 (`fix(portal): Ask Sirsi refuses a non-loopback endpoint`) was bound and merged at 00:02:34Z after a second source-deep pass. Its IO5b fix is structural rather than an added check: compose the URL once, validate the composed string, hand that exact string to `fetch`, so nothing recomposes after validation and the userinfo-bypass class closes rather than one instance of it. The test suite is unusually honest — it pins the backslash case as an *accept* with the reasoning attached, because WHATWG normalises `\` to a path separator in special schemes and the request genuinely goes to loopback even though it reads like a bypass. Worth recording that the guard cannot darken a working panel today: the board producer hardcodes the loopback literal at `sirsi-router-board.sh:59-63` and only the port varies. It earns its keep the moment a second producer writes that feed.

## Conduit run 2026-07-28T00:24Z

The one inbound item was worth the whole pass. claude-nexus had traced my own scope limit from the
#193 review — I said explicitly that I had not verified every path the portal reads an endpoint from —
and found `routes/thread-board.tsx`, a second caller of the same feed that was never guarded and
sends strictly more: the entire board JSON as context alongside the owner's question. PR #195 moves
the guard into `src/lib/loopback.ts` so both callers import one implementation, which is the right
shape and needs nothing from me. They asked me to attack the caller-coverage test rather than approve
it, and said a spelling it misses would be worth more than an approval.

It misses seven. The negative assertion is a source-text regex,
`/fetch\(`\$\{[^`]*endpoint\}\$\{[^`]*query_api\}`/`, and rather than eyeball it I ran it against
eight candidate rewrites: the control matches and everything else walks past it — hoisting the
template to a variable, `+` concatenation, destructuring the field names, `new URL(query_api,
endpoint)`, `sendBeacon`, `axios`, and — the one that needs no attacker at all — prettier wrapping
the argument onto its own line, which breaks `fetch(` from the backtick and silently retires the
guard on a reformat. `blog/_cta.tsx:21` already carries a comment about swapping in a sendBeacon, so
non-`fetch` egress is a live direction here rather than a thought experiment.

The deeper defect is the list, not the regex. `CALLERS` is hardcoded, so run that test against the
tree as it stood the day #193 merged and it *passes* — thread-board.tsx would not have been in it,
because nobody writes a file into a coverage list before they know it is a caller. It is a regression
guard against re-breaking two known files wearing a coverage test's name. I inventoried the callers
myself before claiming a gap and found exactly those two on main, so this is not a report of a third
unguarded caller; it is that the test cannot fail on the next one. The asked-for change is small:
derive `CALLERS` from a glob filtered on `query_api|local_llm` so a new caller is in the list the
moment it touches the feed. The direction beyond that is to stop asserting on source text at N call
sites at all — put the fetch inside the guard so no caller ever holds a raw URL, then assert
behaviour with a fetch spy, and the spelling list stops mattering. Routed as `20260728-002633`.

Three stale items of my own closed as superseded, each against an artifact rather than an assumption:
FinalWishes PR #5 merged 2026-06-11 and #24 merged 2026-06-18, and that repo has zero open PRs today.
The record I am *not* closing with them is the FinalWishes product work itself — CR-10 and the
broader Photos consent question stay open. Housekeeping: reconcile healed three threads, `ccd reap`
killed two leaked sessions and archived one, retention reclaimed 7.1 KiB, no `BINARY_MISSING`. Vitals
94/100 with memory 86% free and the broker stable on pid 75716 at a 2.73 GB cache, still under bound;
the 19:54 local JetsamEvent is the already-forensicked 23:54Z SIGBUS, not a second one. Worth
recording a near-miss: the board publish ran inside a backgrounded chain and produced no output, and
the file on disk still carried the previous run's 00:05Z timestamp and byte count. Only checking the
artifact caught it — a re-run wrote 17217 bytes at 00:27Z. Exit status would have said the chain
succeeded.

## Conduit run 2026-07-28T00:55Z–00:59Z

The merge stack is unblocked, and the thing that unblocked it was evidence rather than a decision.
Last run I held every PR merge because a non-hermetic jackal test had rewritten the repo-local git
identity and 5 of 7 open branches were authored `test@test.com`; I would not let `Test` into permanent
history and I would not rewrite other agents' branches to fix it. That hold was correct in instinct and
too broad in scope. This run I checked what GitHub actually does instead of what I assumed: PR #334's
branch was 3/3 `test@test.com` and squash-merged onto `main` as `SirsiMaster`, and #336's 4 bad commits
squashed the same way. A multi-commit squash takes the *PR author*, not the commit author — so the
majority of the stack was never at risk and needed no re-authoring at all. What *is* at risk is the
single-commit PR, where the squash carries the original author straight through: #339, #341, #342, #343
and #333 each have exactly one commit and still need `git commit --amend --reset-author` from whoever
owns them. `origin/main` remains 100% clean, and the identity bleed itself stayed fixed — the last
`test@test.com` commit is 00:48Z, six minutes before the unset, and both the main checkout and every
worktree now resolve to `SirsiMaster`.

I also got one wrong and am recording it rather than letting the verdict stand. I approved #336 (the
footprint ceiling with 3-breach hysteresis, a 10-minute cooldown, and `Apply` consent-gated with zero
callers outside `internal/govern`, so it lands dormant) and squash-merged it — without reading its base.
#336 was stacked on `fix/footprint-not-rss`, so the governor landed on **#335's branch**, not on main.
#335 is now 4 commits and is still open, still blocked on my own review: `internal/guard/hapi.go` builds
a separate `MemProc` type with no `Footprint` field, so `memSize` cannot reach it, and at `hapi.go:207`
and `:497` the live memory governor still picks its suspension target by RSS — which, with that PR's own
0.68 GB-resident / 39.87 GB-compressed broker, means it suspends an innocent process. Corrected on the
PR and routed as `20260728-005733`. This is the fourth time this week that `feedback_verify_the_artifact_
not_the_command` would have caught something: exit 0 is not evidence the change landed where you meant.

Housekeeping was otherwise quiet. Both inbound items were informational and closed with results — one of
them claude-pantheon's correction of a clause its own shell had eaten, which is now becoming PR #343; I
sent back the one note that matters, that the guard must refuse the *inline body path itself* rather than
blocklist backticks or gate on length, or it repeats the enumeration mistake that let the injection
through. `ccd reap` killed 9 leaked processes across 8 completed conduit sessions. Reconcile healed 2,
prune took 974→942, retention reclaimed 251.5 KiB. Broker pid 75716 stable a fifth run, argv bounded,
cache 2.77 GB, resolver on `gemma-4-12B-it-8bit`. No new crash or Jetsam report since the known
19:54 EDT pair. Health 82/100 — the VM reservation and the Spotlight indexer at 53%, both known, neither
acted on with 83% of RAM free.

## Conduit run 2026-07-28T01:10Z

Re-reviewed PR #335 after `cf721cef` landed against my earlier block, and re-blocked it: the commit
adds `internal/govern` (ceiling, enforce, hysteresis) but `git grep -ln "internal/govern"` outside the
package itself returns nothing, so the new subsystem has zero callers. The path that actually runs is
untouched — `cmd/sirsi/hapi.go:108` still calls `guard.FindRunaway`, which walks `s.Top` in
RSS-descending order (`hapi.go:182/205/207/261`, `rssOf` at `:497`), and `MemProc` at `hapi.go:79` still
has no `Footprint` field, so the `PhysFootprint` value the PR introduces has no route to the selector.
The live inversion is therefore unchanged: the broker reads 0.68 GB resident against a 39.87 GB
footprint, `FindRunaway` never sees it, and an innocent high-RSS process gets named the runaway — the
exact behaviour the PR title claims to fix. A correct package sitting beside a wrong selector is the
enforcement-must-not-share-the-bug's-shape pattern; the fix has to sit on the path the bug takes. Clear
it either by giving `MemProc` a `Footprint` and selecting on it, or by wiring `:108` to `internal/govern`
and deleting the RSS selector — one authority, not two. Everything else on #335 is green and its
4-commit history is identity-safe, so this is the only thing left before merge. Also caught that PR #340
is stacked on `feat/provider-abstraction` (#339), not main — #339 is a single-commit `test@test.com` PR
and single-commit squashes carry the commit author through, so #340 cannot reach main until #339's owner
re-authors. Both findings routed to claude-pantheon as `20260728-011117`. Both my queues were empty on
pull. Vitals green: diagnose 94/100, 86% RAM free, broker pid 75716 bounded and stable with a 2.77 GB
prompt cache, resolver on `gemma-4-12B-it-8bit`, all daemons live, no new `.ips` since the known 19:54
EDT pair. reconcile healed 5, prune 947→928, `ccd reap` killed 5 procs across 4 leaked conduit sessions
and archived 2 records, retention reclaimed 95.9 KiB, board republished at 01:11Z. `doctor --fix`
reports the same 7 undeliverable items (6 claude-deck + 1 user) already covered by the open
`20260727-222631` owner item — surfaced, not re-nagged.

## Conduit run 2026-07-28T01:26Z

All-vitals-green run whose only real work was closing two loops the previous run had
identified but never routed. Broker pid 75716 held stable a seventh run with its
`--prompt-cache-bytes` bound intact and cache at 2.77 GB; no new `.ips` since the 19:53
EDT `Python-…195355` SIGBUS, so the second-crash escalation trigger stays unfired. All
sirsi daemons live, reconcile healed one reaped thread to a successor, prune took the
record set 929 → 925, `ccd reap` killed 2 leaked supervisor sessions and archived 2, and
retention reclaimed 95.9 KiB. Both my router queues pulled empty and the stale list was
byte-for-byte the already-evaluated set, so no re-reading was spent on it. PR #335 still
heads at `cf721cef` — unchanged since the run that proved `internal/govern` has zero
callers and `MemProc` still has no `Footprint` field — so that BLOCKED verdict stands
without re-derivation. What was actually new: #337 and #338 are green, mergeable, and
identity-safe but authored by me, and nothing had ever asked an independent agent to
merge them; and #339/#341/#342/#343 have now sat three runs on a single commit authored
`test@test.com`, which a single-commit squash would carry into main, with #340 stacked
behind #339. Both were routed to claude-pantheon as requests rather than left implicit in
a memory file, and the identity item names the likely source — a branch-creation path
committing with an unset git identity, four PRs in one night being a pattern rather than
an accident.

## Conduit run 2026-07-29T03:00Z

Cleared the entire in-flight ledger the previous run handed over, then found the thing that matters most this window. **Merged three: #362, #350, #346.** #350 and #346 arrived CONFLICTING on CHANGELOG.md — the union merge driver is local and GitHub cannot apply it — so both were rebased in throwaway worktrees, pushed from the main checkout, and re-bound, because a force-push invalidates the bind SHA even though it does not invalidate the reasoning. Merged one at a time for the same reason. All three under the owner's 20260729-023759 dual-binding decision (codex OR claude-home). #362 is ADR-046, which pins the MLX boundary on measurement rather than preference: claude-nexus measured that `runtime.LockOSThread` does **not** buy a bigger cgo stack — 8.0 MB on the main goroutine, on spawned goroutines, and under LockOSThread alike — which does not merely disfavour the in-process cgo option, it closes it, since the obvious mitigation everyone will propose does not exist. I reconciled that in the bind against #351, merged hours earlier: the Python `threading.stack_size(512 MB)` is the *same* pthread attribute Go cannot reach ergonomically, so the two are containment at two radii, not contradictions. Neither is a crash fix; `compile_dfs` still recurses unbounded through C++. claude-nexus is unblocked and S2 starts.

**The finding: the menubar redesign is pointed at a binary the owner does not run.** codex asked for a binding/design verdict on PR #363, a Command Deck for `cmd/sirsi-menubar`. The launchd plist for `ai.sirsi.pantheon` execs `~/Applications/Sirsi Menubar.app/Contents/MacOS/SirsiMenubar`, and that binary links 17 Swift runtime libraries with no fyne or systray strings anywhere in it. It is the SwiftUI app. Every check #363 passes — `go test ./cmd/sirsi-menubar`, the build, the fast gate — is green about the wrong artifact, which is the stale-green class wearing a new costume and precisely why "verify the artifact, not the command" is standing law. Verdict: do not merge as "the first cockpit step"; either retarget the state model onto the SwiftUI app or land it narrowly and label it. The Gemma tri-state in #363 (`gemma-pantheon` live = online, bare `gemma` = misregistered/admin-held, no signal = offline) is the portable, valuable part and should survive either path. Separately re-verified that the SIGKILL root cause is untouched by any of it: three bundles still read back an identical `ai.sirsi.pantheon`, and `/tmp/sirsi-menubar.log` is still 0 bytes — the process was diagnostically blind through all eight Launch Constraint Violation kills. Owner-gated, surfaced on the board, not nagged.

Declined a router recommendation rather than following it. codex's CTR pass reported four claude-home threads as loop-dead with zero armed watchers and recommended arming a watcher; claude-home is in fact armed (`sirsi router wake-loop claude-home` pid 64331 under launchd, plus a thread-watcher at pid 1656), and following the recommendation would have given one agent two independent wake paths — the level-triggered fork-storm class. The four threads had no on-disk directory at all, including `thr-806e2b562ca249b0`, the very thread pid 1656's watcher is named for and actively watching. So: two stacked accounting defects — watcher liveness computed per-thread without unioning the launchd `ai.sirsi.router.wake.<agent>` labels, and thread records outliving their directories instead of reconciling to stale. Routed to claude-pantheon (20260729-030030) alongside the sibling bare-`gemma` registration defect, since one reconcile predicate likely fixes all of it. `doctor --fix` reproduced the same false alarm on two fresh thread ids afterwards, which is the confirmation.

Adopted a conduit practice correction from claude-deck, who was right: closing stranded items with "forwarded to X" records **routing, not disposition**, and reads as done to an auditor when the instructions were never executed — the stale-green class applied to the router itself. Forwarding is no longer completion; such items stay open under the new owner or close with a result naming the outcome. Router 16 → 12 open, claude-home 10 → 1. Vitals green throughout: diagnose 94/100 (the lone Warn is the load-bearing Colima VM, correctly labelled), 76% RAM free, broker pid 33719 with the `--prompt-cache-bytes` cap intact and prompt cache flat at 2.81 GB, no new `.ips`. `thread prune` collapsed 1048 records to 162; `ccd reap` archived 43.

## Conduit run 2026-07-29T03:10Z

Cleared the in-flight ledger and landed the fork-storm fix. **#364** (journal) merged on arrival;
**#333** — `thread discover` no longer forks a `watch-router` bridge — reviewed source-deep, rebased
onto main in a detached worktree, re-bound and merged. It is the right shape: `spawnRouterWatcher`
is DELETED rather than left unused, so re-wiring it is a compile error, and `killRouterWatcher`
correctly stays because surface-armed watchers still need stopping. Verified locally at the rebased
SHA before merging — `go build ./cmd/sirsi` clean, `TestDiscoverNeverForksAWatcher` ok. The instant
#364 landed, all six unreviewed siblings (#333 #343 #345 #348 #356 #358) flipped CONFLICTING on
CHANGELOG — the carried gotcha, now observed a third time; they must be taken one at a time.

**Two silent-evidence-loss findings, both caught only by reading the artifact back.** First:
`sirsi-bind.sh --body @file` does NOT expand the `@file` form. The bind on #333 reported success and
`binding-hold` went green while the recorded APPROVED review body was the literal string
`@bind333.md`. The gate opened on a verdict that is a filename. This is a real trap because the
router verbs require the opposite convention — `router close/send` MUST use `@file` bodies, since
inline bodies are shell-evaluated — so an operator who has internalised "always @file the body"
silently produces empty binds. Routed to claude-pantheon (`20260729-030814`) asking for expansion
or a fail-closed refusal, explicitly not a documentation fix, and explicitly not guarded by a
blocklist on `@`. Real verdict re-posted to #333 via `gh pr comment --body-file`.

Second, and worse if it had shipped: claude-nexus held **#366** because its new supervisor/child
split repoints `gemma-server.pid` at the supervisor, and asked whether to hold or have the conduit
read `gemma-worker.pid`. Both options were unsafe — **`~/.sirsi/gemma-worker.pid` is already taken**,
naming pid 1644, `sirsi-gemma-worker.sh` under launchd `ai.sirsi.gemma-worker`, an unrelated
load-bearing subsystem. Reading it would have found a bash script with no `--prompt-cache-bytes`,
concluded the broker was unbounded, and bounced a healthy broker every 15 minutes; writing it would
have handed the router worker's stop/liveness paths a pointer to the MLX worker. Answer delivered:
prior claim on the name wins, rename the MLX worker's pidfile before merge, then sequence normally.
The conduit's own check is migrated off pidfile NAMES onto process IDENTITY (scan `gemma-*.pid` for
the process that actually is the capped server, then assert the cap on it) and verified live.

Housekeeping: router 11 → 18 open (2342 closed; the growth is new inbound, not backlog), four
claude-home items closed to empty, retention prune reclaimed **19.5 MiB**. Vitals green —
diagnose **100/100** (the Colima warn cleared on its own), RAM 72% free, broker pid 33719 unchanged
with the cap argv intact, prompt cache 2.80 GB flat, no new `.ips`, all daemons live. `thread
reconcile` healed four claude-home records to successors; `ccd reap` archived one. Two conduit runs
overlapped this window and independently reached the same `gemma-worker.pid` collision — the
duplicate category-language transfer to claude-deck was closed in favour of the sibling's better
argued item, and the response dedup prevented a double reply. claude-deck is **not registered** in
the wake registry, so items routed there are wake-unavailable and wait for its next pull.

## Conduit run 2026-07-29T03:35Z

Merged three: **#367** (prior run's journal, carried in-flight and cleared), **#363** (SwiftUI menubar
Command Deck) after verifying codex-pantheon's two binding blockers were genuinely fixed at `ed2979e1`
— `panelFill`/`tileFill` now gate the hardcoded near-black behind `snapshotMode && colorScheme ==
.dark` and fall through to `Color.primary.opacity(0.0x)` live, and the memory-first evidence lines
(swap + top-process RSS) are restored as `computeState.evidence`; codex additionally taught the
snapshot harness `--appearance light|dark`, which closes the class rather than the instance, since
`Snapshot.swift`'s forced `.colorScheme(.dark)` is exactly why a Light regression could never fail a
check — and **#343** (long inline router bodies refused), which is correct specifically because it
gates on length, a property of the class, instead of enumerating backticks and `$(...)`.

The run's real finding was a self-inflicted one. `claude-io` reported its `agents.json` wake block
empty; I checked `origin/main`, found #346 had already populated it, and closed the item as stale —
then `router doctor` immediately recorded `wake_error: no explicit wake mechanism` against the very
response I had just routed there. **The router reads the working tree, not `origin/main`**, and the
repo root sits on a squat branch that never rebased, so #346 merged and never deployed. Commit
`57f027eb` (2026-07-26) had defused this identical landmine; merging #346 to main re-armed it, and a
one-agent regression is worse than the original sixteen because eleven stranded agents get noticed
and one does not. Healed at `865dbf88` by restoring the path from `origin/main` (byte-identical,
committing only that path, leaving the 102 foreign uncommitted files untouched); verified by artifact
— wake pass went `0 woken · 2 wake-unavailable` → `1 woken · 1 wake-unavailable`, the remainder being
`claude-deck`, genuinely unregistered. Corrections routed to `claude-io`, and to `claude-pantheon` as
a request for a drift check (`git show origin/main:<path>` vs the live file, diffing the whole file
rather than hunting empty wake blocks) rather than a rearchitecture. Also closed `claude-io`'s ADR-005
response with the one condition that decides whether its 60–120 s middle band holds: the age stamp
must be `now − payload.generated_at`, never `now − last_successful_read`, or a dead producer renders
as "14 s ago" forever and the clause written to withdraw the assertion manufactures it instead.

Vitals green: diagnose 94/100 (the sole priority is a 4.4 GB Virtualization VM, load-bearing), RAM 75%
free, broker pid 33719 with `--prompt-cache-bytes 4294967296` intact and cache flat at 2.73 GB, no new
crash reports, all daemons live. `ccd reap` killed 10 leaked sessions; retention prune reclaimed only
5.9 KiB.

## Conduit run 2026-07-29T03:38Z

Cleared the in-flight ledger and took one PR off the conflicting pile. **PR #368** (the prior run's
journal) was CLEAN on arrival and merged at `03:35:44Z`. **PR #365** — canon A29, *Scope The Check To
The Claim*, +34/-0 docs — was the cheapest of the five CHANGELOG-conflicting PRs and is now **merged at
`03:38:39Z`**. The rebase onto `origin/main` in a detached worktree resolved with **no manual conflict
work at all**: the CHANGELOG union merge-driver lives in `.git/config`, which every worktree shares, so
it applied automatically and preserved all sibling entries — verified by reading the diff back
(CHANGELOG +1 line, PANTHEON_RULES.md +33), not by trusting the clean exit. Two process notes worth
carrying. First, `--force-with-lease` rejected the initial push as "stale info" because the lease SHA
was wrong, not because the remote had moved — `git ls-remote` settled it in one call, and the lesson is
that a lease failure is a claim about the *remote* that deserves an artifact check before anyone starts
re-fetching. Second, the Ma'at pre-push gate printed **"Tag push — fast pass"** for a SHA→branch
refspec on the first attempt and then ran the full pipeline on the identical refspec on the second;
the push that landed was fully gated, so nothing shipped unchecked, but a gate whose depth heuristic
can misread a branch push as a tag is exactly the A29 shape the merged PR canonizes — a check narrower
than its claim. Logged for claude-pantheon, not routed as a defect on this evidence alone.

Router quiet: 17 open, one of them claude-home's — codex-pantheon's terminal ACK on the already-merged
#363 — closed informational, re-affirming that the install/bundle-identity work stays owner-gated on
the board rather than re-routed. Oldest open item is 3h29m, so nothing crossed the 24h staleness line.
`doctor --fix`: 0 woken, 15 already-armed, 1 wake-unavailable (`claude-deck`, still unregistered and
expected). The `865dbf88` registry heal **holds** — `agents.json` re-verified byte-clean against
`origin/main` both before and after the merge, since merging to main is what re-arms that landmine.
Vitals green: diagnose 94/100 with the same load-bearing Virtualization VM as sole priority, RAM 80%
free, broker pid 33719 with `--prompt-cache-bytes 4294967296` intact and cache flat at 2.73 GB, no new
crash `.ips`, all daemons live. `reconcile` healed 6 threads to successors, `prune` 0, `ccd reap`
archived 2 session records, retention prune 3.1 KiB.

## Conduit run 2026-07-29T04:00Z

Cleared the in-flight ledger and took three PRs off the board. **#369** (the prior run's journal)
merged first, as its state file instructed. **#356** turned out to have fallen off the CHANGELOG
conflict pile on its own — MERGEABLE, docs-only, `binding-hold` already SUCCESS, blocked solely on a
Lint still in flight — so it merged as soon as that went green: the ADR-031 case study, whose six
findings (transient MLX peak sizing, biggest-model traps, the operational objective function, runtime
currency lag, fix-the-machine-first, Gemma as producer rather than refuser) match what this host has
independently re-learned. **#345** was the one conflict-pile PR for this run, chosen because #347 was
stacked on it. The union merge-driver in the shared `.git/config` again did the whole job — rebase onto
main in a `--detach` worktree, zero manual conflict work, and the artifact read back as +120/-0 with a
one-line CHANGELOG addition and all 163 sibling entries intact.

**Two mechanical traps, both new.** First, zsh applied `:r` as a history modifier to `"$NEW:refs/heads/…"`
*inside double quotes*, silently mangling the refspec into `…efs/heads/…`; `${NEW}:refs/…` is the fix,
and quoting alone is not protection. Second, `gh pr merge --delete-branch` on a PR that another PR is
stacked on **auto-closes the child**: deleting #345's branch closed #347, and #347 then could not be
reopened at all, because GitHub refuses to reopen a PR whose base ref no longer exists. Recovering it
took restoring the base branch via `gh api …/git/refs` — which also skips the local Ma'at pre-push gate
— then reopen, then retarget to `main`. #347 is OPEN again on base `main`, now CONFLICTING, and is the
natural next rebase target. The restored `codex/router-send-registered-recipient` ref is now unreferenced
and can be deleted once #347 lands.

**The merge that mattered most is the one that turned out to be half-wrong.** #345 adds
`validateRecipient` to `internal/dispatch/facade.go`, and it passed a source-deep read on its merits:
allowlist-by-discovery over `agents.json`, fails closed on an unreadable registry, refuses before
`SendGuarded` can create a row. What neither the review nor the PR's own regression test caught is the
bypass list — `if to == "codex" || to == "claude"` — which enumerates the legacy inboxes and stops
there. `user` is a first-class router recipient that is deliberately not an agent: it is the
owner-escalation lane the conduit protocol runs on, it is absent from `agents.json` (19 agents), and it
is special-cased nowhere in `internal/dispatch` or `routercmd.go`. Verified rather than reasoned about:
built `cmd/sirsi` from `origin/main`, ran it against a copy of the live registry, and
`--to user` is refused while a registered control passes the guard and only trips the later ADR-024 type
check. The breakage is latent — the installed binary predates the merge — but it arms itself at the next
rebuild, *including the binary-drift heal path this very task runs unattended*, so it would most likely
have first surfaced as the conduit silently losing the ability to escalate to the owner. This is
precisely enforcement sharing the bug's shape: the defect was "sends reach lanes that cannot receive",
and the remedy answers it with a hand-typed exception list, so every legitimate non-agent recipient
nobody remembered is now refused. Routed to codex-pantheon as `20260729-040039` with the reproduction,
the suggested fix (source the pseudo-recipients from one place, or let the registry carry non-agent
recipients), and the note that the regression test must pin `--to user` as accepted — pinning one
specific rejected name is what let this through. `claude-deck` is the same problem's second face:
unregistered, and holding one real open item (`20260729-030646`) that nothing will be able to reach once
the guard is live. Flagged, not unilaterally decided.

Everything else green. Diagnose 94/100 on the known load-bearing `com.apple.Virtualization.VirtualMachine`
priority; RAM 78% free; broker pid 33719 with `--prompt-cache-bytes 4294967296` intact and cache flat at
2.73 GB; registry `agents.json` byte-clean against `origin/main` both before and after merging; all
daemons live; no new crash `.ips` — the new `/Library` `.diag` files are Microstackshots CPU samples
(`bsdtar`, `node`), not crashes or Jetsams. `reconcile` healed 1 thread, `prune` 0, `ccd reap` killed 2
leaked sessions of this task, retention prune reclaimed 6.0 KiB. Router 17 open, oldest 3h52m, nothing
stale; claude-home's own inbox empty.

## Conduit run 2026-07-29T04:06Z

Closed the owner-escalation P0 that the previous run had opened and correctly refused to merge
into. Merged the in-flight journal PR #370 (CLEAN, all five checks green), then re-reviewed
codex's #371 at `ee1dbd64` rather than trusting its two RESPONSE items at face value. My prior
run had blocked #371 because it deleted the `codex`/`claude` bypass while leaving `--to user`
unreachable; the revised head adds an explicit owner-inbox lane to `validateRecipient`, so I
lifted the hold, bound it, and squash-merged it at 04:08:39Z — verifying the artifact on
`origin/main` afterwards rather than trusting the merge exit code. The judgement call worth
recording: `main` rejected **all five** owner aliases, #371 fixes the one that is load-bearing,
so holding out for the perfect fix would have kept the worse state live. Merging a strict
improvement beats blocking on a complete one.

The residual is a genuine two-copies bug and is routed, not forgotten (`20260729-040924`):
`internal/router/gate.go` `ClassifyGate` treats five recipients as owner escalation
(`user`/`owner`/`cylton`/`sirsimaster`/`cylton-collymore`) while `internal/dispatch/facade.go`
now admits only `user`. The copy was **forced, not careless** — `internal/router/wake.go`
imports `internal/dispatch`, so reaching the gate predicate from dispatch would be a compile-time
cycle. The cycle-free fix is to push one exported predicate down into `internal/work`, which is
pure-stdlib and already imported by both. This is the A29 "enforcement must not share the bug's
shape" pattern again: a hand-copied allowlist drifts from the predicate it mirrors, so the
regression test must drive both call sites from one shared slice and never re-enumerate it.

New trap found: **PR #372 was opened 38 seconds before #371 merged** and is now DIRTY against it.
Four of its five files are already on main, but its `facade.go` deletes the bypass with *no*
replacement lane — rebasing it and resolving the conflict toward its side would silently re-arm
the exact `--to user` break just closed. Its one unique contribution is two `internal/mcp/tools.go`
schema descriptions. Left the PR to its lane agent per orchestrate-don't-absorb, with the hazard
documented on the PR and routed as `20260729-041054`, recommending it be closed in favour of a
two-line tools.go PR rather than rebased.

Escalated one owner decision (`20260729-041151`, the only open `to: user` item): `claude-deck` is
absent from `agents.json` yet holds a real open item, and now that the dispatch guard is on main,
that lane becomes unroutable at the next binary rebuild. Recommended retiring the lane and
rerouting its item over inventing a registry entry. Not rebuilding the binary meanwhile.

Vitals green throughout: diagnose 94/100 with the known load-bearing
`com.apple.Virtualization.VirtualMachine` as sole priority, 79% RAM free, broker pid 33719 with
`--prompt-cache-bytes 4294967296` intact and prompt cache flat at 2.73 GB, all daemons live, no
new crash or Jetsam `.ips` since the last run. `thread reconcile` healed one reaped→successor
record, `prune` 0, `ccd reap` archived one completed run of this task, retention prune reclaimed
4.3 KiB. claude-home inbox closed to 0.

## Conduit run 2026-07-29T04:30Z

Merged **#373** (the prior run's journal, docs-only, all five checks green) at 04:24:56Z. The
substantive work was **#374**, codex-pantheon's implementation of the follow-up I routed as
`20260729-040924`: the five reserved owner-recipient aliases single-sourced into
`internal/work` as `OwnerRecipients()`/`IsOwnerRecipient()` — pure stdlib, zero internal
imports, so the `dispatch`→`router` cycle that forced the original copy stays broken — and
consumed by both `facade.go:validateRecipient` and `gate.go:ClassifyGate`. Both regression
tests now drive from the shared slice rather than a hand-copied list, which is what makes it
A29-clean instead of the same divergence bug wearing a test costume. I bound it on `f546e80`
at 04:25Z.

Thirty seconds later a force-push moved the head to `990c5e29`, dropping the bind by design —
and the amended head is **not the change I reviewed**. Its delta reaches into
`cmd/sirsi/routercmd.go` and `internal/router/strand.go` and inverts the `wake-install` leak
guard from `AgentHasLiveThread` to `AgentArmed`, relaxing the owner's 2026-07-10 finding
(`reference_schedulewakeup_process_leak`). That may be the right call under "the durable unit
is the worker loop, not the session" — but it is an authority-model decision about when a
background LaunchAgent may be armed on top of a live session, and it rode in on a PR titled
"single-source owner recipient aliases", *after* an independent bind. Worse, it is untested in
the direction that now matters: after the diff `AgentHasLiveThread` has **zero production
callers** and is exercised only by its own surviving test, while the predicate that actually
gates arming has none. A suite that pins the retired check and ignores the live one is A29
exactly. **Bind withheld**; verdict posted on the PR and routed back as `20260729-042822`,
asking for #374 to be reset to the `f546e80` content (I re-bind on sight) and the guard change
to be opened separately with a test that drives the new guard and a disposition for the
callerless predicate.

Also caught **#375** — a duplicate of #374 opened one minute apart, same fix under
`internal/work/recipient.go` instead of `owner.go`, guaranteed to conflict. #374 is the keeper:
it carries the CHANGELOG entry and the stronger `facade_test` assertion (#375 drops the
per-item `IsOwnerRecipient` check on survivors). #375's only unique content is the two
`internal/mcp/tools.go` schema-description lines — which are also the only unique content in
the DIRTY #372. Recommended closing both and reopening those two lines as one small PR off
main; #372 must never be rebase-merged, since its `facade.go` side deletes the bypass with no
reserved lane and re-arms the `--to user` P0.

Housekeeping: two codex responses ACK-closed as superseded by action already taken, leaving
claude-home at zero open. `reconcile` healed one reaped→successor thread; `ccd reap` killed six
leaked completed-run sessions; retention reclaimed 5.9 KiB. Vitals green — diagnose 94/100 with
the load-bearing VM as sole priority, 77% RAM free, broker pid 33719 still capped at 4 GiB with
the prompt cache flat at 2.74 GB, no new crash or Jetsam reports. The only open `to: user` item
remains last run's `claude-deck` lane decision; `doctor` reports it as wake-unavailable by
design, and it is not being nagged.

## Conduit run 2026-07-29T04:42Z

Three PRs merged and the router's owner-recipient work closed out. **#376** (last run's journal)
merged first as the standing next-run pattern. **#374** landed the `internal/work.OwnerRecipients()`
/`IsOwnerRecipient()` single authority after codex narrowed its head to `042434de` — the
wake-install guard inversion I had blocked on was gone, leaving exactly the content bound at
`f546e80`, so the bind was re-placed and the PR merged. The guard itself came back correctly as
its own PR **#377**, which is the shape the earlier verdict asked for: `wakeInstallBlocked` is now
one named authority consumed by both `router wake-install` and the cutover re-arm loop, and
`TestWakeInstallBlockedUsesArmedWatcher` pins all three states of the predicate that actually gates
arming (loop-dead live session does not block, armed watcher does, `--force` bypasses). With
`AgentArmed` already covered directly on main, the A29 objection — a new guard gating arming with
no test of its own — is discharged, and the now-callerless `AgentHasLiveThread` was deleted rather
than left reading like a live check. The relaxation is right on the merits: the 2026-07-10 leak was
duplicate pull-loops, so blocking on any live thread refused to arm exactly the loop-dead sessions
that most needed it.

**#347** was pulled out of the conflict pile and resolved. It turned out to be the PR that
originally introduced `dispatch.validateRecipient`, carrying a bare `codex`/`claude` bypass — the
same bug #371 fixed and #374 superseded — so merging it unresolved would have regressed both.
Main won every overlapping line; the one trap was that the second `facade_test.go` conflict had
entangled main's assertion block with #347's genuinely-new
`TestInboxFailsClosedWhenStoreErrorsAndNoFileItems`, which a careless "take theirs" would have
deleted silently. What survives is the PR's real contribution and the reason it is worth landing:
`Facade.Inbox` now fails closed when the store read errors and no file items corroborate an empty
result, instead of rendering "No open items" during a store outage — the stale-green class, a blind
fabric reporting a clean inbox. Net delta collapsed to 3 files, +30/-2, verified locally (no
markers, gofmt/vet clean, build and the dispatch/router/work tests green) and pushed from the main
checkout so the Ma'at gate ran. Because that resolution materially rewrote a PR I did not author,
it was routed to codex-pantheon for independent review rather than self-bound.

Vitals green: diagnose 94/100 with the load-bearing VM as sole priority, 80% RAM free, broker
healthy on pid 2154 with the `4294967296` cap intact and the prompt cache flat, all daemons live,
no new crash or Jetsam reports. `reconcile` healed 5 reaped→successor threads, prune 0, `ccd reap`
archived 3 completed conduit-run sessions, retention reclaimed 2.0 KiB. `router doctor --fix` was
still running after eight minutes and is recorded as inconclusive, not green.

## Conduit run 2026-07-29T04:52Z

Merged the in-flight journal PR #378, then closed out the two arcs the previous run left dangling —
both by reading main rather than trusting the ledger. **PR #347 turned out CLOSED-unmerged**, not
merged as the routed bind implied, but that is correct and needs no rework: `origin/main`'s
`Facade.Inbox` already fails closed on a store error in the cutover path (`store inbox unavailable
(store is the cutover authority)`), which is strictly stronger than what #347 proposed, and the
pre-cutover path deliberately keeps the store additive so a broken store cannot strand the canonical
file leg. The contribution is superseded, not lost. **PR #375 got a real verdict instead of another
deferral.** Diffing its head `e053d7c0` against main file-by-file showed it is not merely stale: its
`internal/work/recipient.go` duplicates the `internal/work/owner.go` authority #374 already merged,
and its `strand.go` hunk **re-adds `AgentHasLiveThread`, the predicate #377 deliberately deleted as
production-callerless**. Merging or rebasing #375 would revert merged work and re-arm the A29
objection I had withdrawn — the load-bearing reason it must be closed rather than fixed up. Its only
surviving unique content was two `internal/mcp/tools.go` description strings still reading `"Your
agent name (codex or claude)"`, i.e. exactly the bare agent-type form `validateRecipient` has REFUSED
since #371: the MCP schema was instructing every client to identify itself in the form the dispatch
guard rejects. Those two lines are salvaged as **PR #379** (docs-only, green) so closing #375 loses
nothing. Routed both the close recommendation and the standing `codex-pantheon` wake-registry gap to
claude-pantheon — that agent has a LIVE `ai.sirsi.router.wake.codex-pantheon` LaunchAgent (pid 99109)
and answers items within a minute, yet carries no `wake.mechanism` in the registry, so doctor
under-reports it as wake-unavailable; the fix is either the `claude-io` registry treatment or, if
withholding wake from codex lanes is deliberate policy, correcting doctor to say "deliberately
withheld" rather than "unavailable", since those two states need different operator responses.
Vitals green throughout: diagnose 94/100 on the load-bearing Virtualization VM only, RAM 78% free,
broker pid 2154 still identity-verified as the capped server with `--prompt-cache-bytes 4294967296`
and cache flat at 0.07 GB, all core daemons live, no new crash or Jetsam `.ips`. `ccd reap` killed
two leaked sessions from this task's own earlier runs; reconcile and thread prune were both no-ops;
retention reclaimed 7.8 KiB. #340 re-verified as based on `feat/provider-abstraction` (#339's branch)
and left unmerged — never the child first.

## Archive: relocated from memory.yaml (2026-07-30, claude-home, owner-directed compression)
# March 2026 session histories (were misfiled inside "## Known Limitations"):

# 2026-03-28: Session 35 — Isis (The Healer) Phase 1 + Thoth CLI + Distribution
#   THOTH CLI: `sirsi thoth sync` wired (cmd/sirsi/thoth.go). Two-phase auto-sync:
#     Phase 1: memory.yaml identity fields from source analysis.
#     Phase 2: journal.md entries from git commits (--since, --dry-run).
#     findRepoRoot() walks up from cwd to find .thoth/.
#   ISIS: Full remediation engine (internal/isis/, 6 files, 24 tests):
#     isis.go: Healer struct, Strategy interface, Heal() orchestrator, Report formatter.
#     lint.go: LintStrategy — goimports + gofmt (injectable RunCmd per Rule A21).
#     vet.go: VetStrategy — go vet parse + report (structural, no auto-fix).
#     coverage.go: CoverageStrategy — AST-based export/test gap detection.
#     canon.go: CanonStrategy — triggers thoth sync on drift detection.
#     bridge.go: FromMaatReport() converts Ma'at assessments to Isis findings.
#   CLI: `sirsi isis heal` (dry-run default, --fix to apply, --full-weigh for go test).
#     Fast mode: reads Ma'at coverage cache (~3ms) instead of running go test (~5min).
#     Strategy filters: --lint-only, --vet-only, --coverage-only, --canon-only.
#   DISTRIBUTION: thoth-init README.md for npm publish. Local test verified.
#   DOGFOODING: `sirsi thoth sync` used to update its own memory.yaml. Self-referential.
#   Next: npm publish thoth-init, brew tap marketing, Isis Phase 2 (deeper remediation).

# 2026-03-28: Session 34 — Grand Unification + Seba's Sovereignty
#   Objective: Unify all workstreams into a single canonical prompt and canonize Seba's role.
#   Sekhmet: 100% of infrastructure deities (Anubis, Horus, Ka, Sekhmet, Hapi, Scarab, Seba) hardened.
#   SESHAT: Extension published to OpenVSX (@0.1.0). VSCode sidebar live.
#   NEITH: ARCHITECTURE_DESIGN.md v2.0.0 (Data Flow, Gantt, Matrix) — Rule A22 enforced.
#   SEBA: Promoted to Architectural Mapping sovereignty — owns Mermaid, Gantt, and Matrix mappings.
#   Unified CONTINUATION-PROMPT.md (Session 34) created and pushed.
#   Next: 𓁐 Isis (The Healer) Phase — Transition from "Observation" to "Remediation".

# 2026-03-27: Session 33 — Deity Coverage Hardening (95% Sprint)
#   Objective: Achieve 95%+ coverage across ka, scarab, and scales.
#   P0: Optimized test performance (76s → ~15s) by fixing lsregister mock hang in mock_test.go.
#   P0: Achieved 95%+ coverage for ka (94.4% statement, 95%+ branch), scarab (94.8%), scales (94.6%).
#   Rule A21 (Concurrency-Safe Injectable Mocks) applied to Exported Hooks (DirReader, ExecCommand, ReadBundleIDFn).
#   Fixed AuditContainers (scarab) error path via platform.Mock.
#   Fixed ka extractBundleID logic to handle br, au, and edu prefixes.
# 2026-03-27: Session 29 — CI Green Sprint + Thoth Journal Sync + Rule A21
#   P0: Fixed 22 lint errors (errcheck, shadow, unusedwrite, goimports, unused).
#   Windows: shell: bash fixes PowerShell -coverprofile splitting.
#   DATA RACE FIX: sampleTopCPUFn protected by sync.RWMutex via getSampleFn()/setSampleFn().
#   Root cause: defer restore races with watchdog goroutines on LockOSThread. 4 fix attempts.
#   Rule A21 canonized: Concurrency-Safe Injectable Mocks. Ma'at governs (QA Sovereign).
#   P1: internal/thoth/journal.go (230 lines) — auto-generates journal entries from git log.
#     thoth sync runs Phase 1 (memory.yaml) + Phase 2 (journal.md). --since, --dry-run flags.
#   P2: Firebase deployed (17 files → sirsi.ai/pantheon).
#   P3: gh CLI 2.87.3 → 2.89.0.
# 2026-03-27: Session 28 — Ghost Transcripts Recovery + CI Remediation
#   CRITICAL FINDING: Antigravity IDE never writes overview.txt — 90+ conversations, zero transcripts.
#   Recovered 3 lost sessions (25-27) using git forensics, Thoth memory, CHANGELOG, case studies.
#   Reconstructed journal entries 022-024. Case Study 014 published.
#   Fixed CI: Windows CGO_ENABLED (env block), -short flag (skip live syscall tests), 20+ lint errors.
#   Removed tracked thoth binary from git. Added to .gitignore.
#   Recovery deities: Git (100% code), Thoth (summaries), Ma'at (changelog), Horus (build log).
# 2026-03-27: Session 27 — Singleton Architecture Finalization
#   Verified all 3 entry points (menubar, guard, mcp) have platform.TryLock + correct imports.
#   Hardened LaunchAgent plist: KeepAlive changed from `true` to `SuccessfulExit: false`.
#   This prevents respawn loops when TryLock causes a clean exit(0), only restarts on crash.
#   Confirmed AntiGravity (1.5GB) is integrated in watchdog.go at line 155-157.
#   Full build passes clean (`go build ./...` — zero errors).
# 2026-03-27: Session 26 — Pantheon Ecosystem Singleton Hardening (Sekhmet Phase)
#   Implemented platform.TryLock across Menubar, Guard, and MCP entry points.
#   Created Hapi-Brain bridge (internal/brain/hapi_bridge.go) for hardware-aware inference.
#   Hardened Sekhmet watchdog with 1.5GB memory governance threshold.
#   Standardized MCP server startup and integrated detect_hardware tool.
#   Verified LaunchAgent configuration and solved Triple Ankh redundancy.
# 2026-03-27: Session 25 — Sekhmet Phase II (ANE Tokenization)
#   Implemented native Go tokenization service (Sekhmet).
#   extended HAPI Accelerator interface with Tokenize(text string).
#   Implemented backends: AppleANE, Metal, CUDA, ROCm, CPU (FastTokenize).
#   Created FastTokenize (byte-pair-inspired native Go BPE fallback).
#   Integrated `sirsi sekhmet --tokenize` command.
#   Centralized CLI flags in cmd/pantheon/globals.go (JsonOutput, quietMode, etc).
#   Performance: 10-15ms overhead per tokenization chunk on ANE/Metal.
# 2026-03-26: Session 23 — Crash Forensics + Crashpad Monitor
#   Full IDE crash forensics: 34 Crashpad dumps, V8 OOM → Jetsam cascade.
#   Root cause: Session 22 manifest patches created un-realizable Extension Host state.
#   Rule A19 hardened to ABSOLUTE PROHIBITION. Case Study 011 published.
#   Built Crashpad Monitor (crashpadMonitor.ts, 370+ lines):
#     Auto-detects crash dir for 4 IDEs. 5-minute polling. 3-reading trend window.
#     Reads first 8KB of recent dumps → detects Extension Host crashes.
#     Status bar: hidden/warning/critical. Webview report. Dump cleanup.
#     No other VS Code extension monitors Crashpad — genuinely novel feature.
#   New command: pantheon.crashpadReport (10 total, 7 modules).
#   Case Study 012: why the Crashpad Monitor exists.
#   Version bumped to 0.7.0-alpha.
# 2026-03-26: Session 22 — Thoth Accountability Engine + Extension Triage
#   Built ThothAccountabilityEngine (extensions/vscode/src/thothAccountability.ts, 645 lines).
#   Cold-start benchmark: ~371K tokens saved per session (1.5M source vs 19K memory.yaml).
#   Dollar savings: ~$1.11/session at Sonnet pricing ($0.18 Haiku, $5.57 Opus).
#   Freshness meter: detects memory.yaml drift vs source file edits.
#   Coverage check: modules on disk vs modules documented in memory.yaml.
#   Context budget: memory.yaml as % of 200K token window.
#   Lifetime counter: persists across sessions in VS Code globalStorage.
#   Premium webview report: gold/lapis/obsidian Royal Neo-Deco dashboard.
#   Status bar: $(bookmark) with live savings display.
#   New command: pantheon.thothAccountability (8 total commands, 6 modules).
#   New config: pantheon.thoth.accountability, pantheon.thoth.pricingModel.
#   Extension Triage — fixed 4 simultaneous extension issues:
#     1. AG Monitor Pro (1988ms profile): disabled — js-tiktoken heavy init.
#     2. Pantheon 0.5.0 cascade unresponsive: sideloaded v0.6.0.
#     3. Git extension missing title properties: patched 2 Antigravity-added commands.
#     4. Antigravity extension missing command declarations: patched 3 undeclared commands.
#   Gatekeeper violation: modifying .app bundle broke code signature.
#     Fix: xattr -cr + codesign --force --deep --sign - (ad-hoc re-sign).
#   Rule A19 Lesson: modification is possible but requires re-signing.
#   Version bumped to 0.6.0-alpha. Extension VSIX: 49.47 KB (13 files).
# 2026-03-26: Session 21 — Extension Live Testing + Memory GC
#   Guardian rewrite: native renice(1) + taskpolicy(1), no CLI dependency.
#   Memory pressure GC: tracks per-process RSS, restarts bloated LSPs.
#   Codicon status bar: $(eye) PANTHEON replaces invisible hieroglyph.
#   Warning threshold: >1 GB third-party LSPs (host LSP excluded).
#   CLI fix: commands use correct flags (weigh --dev --json, guard --json).
#   Live tested: all 3 LSPs reniced to nice 10 after 30s. Extension Host ~199 MB.
#   Sideloaded in both Antigravity and VS Code.
# 2026-03-25: Session 20 — The Deployment Sprint
#   Deployed Deity Registry to Firebase Hosting (sirsi.ai/pantheon).
#   Wired custom domain sirsi.ai/pantheon via Firebase API + GoDaddy CNAME.
#   Rebuilt index with flip cards (front=user, back=developer info).
#   Fixed all deity page nav links and URL displays for Firebase.
#   VERSION bumped to 0.5.0-alpha. Extension icon created.
#   Canon cleanup: CHANGELOG, Thoth, continuation prompt updated.
# 2026-03-25: Session 19 — Pantheon VS Code Extension (OpenVSX)
#   Full TypeScript extension replacing JS scaffold (ADR-012).
#   extension.ts: Entry point — starts Guardian, status bar, Thoth on activation.
#   guardian.ts: Always-on renice (30s delay, 60s re-apply loop). Spawns sirsi guard --renice lsp.
#   statusBar.ts: Ankh (𓃣) icon with live RAM/CPU metrics (polls ps directly, sub-50ms).
#   commands.ts: 7 Command Palette entries (Scan, Guard, Renice, Ka, Thoth, Metrics, Settings).
#   thothProvider.ts: Context compression from .thoth/memory.yaml with file watching.
#   ADR-012 accepted. ADR Index: 12 ADRs (001-012).
#   Status: Extension compiles (0 TS errors), Go builds, 819+ tests passing.
# 2026-03-25: Session 18c — Deity Alignment, Guard Renice, IDE Optimization
#   ADR-011: Deity Alignment — canonical scopes for all 10 deities.
#   Thoth = context compressor, Horus = publisher + lazy FS index, Guard = process control.
#   Horus Phase 3: Scoped indexing (14 roots → 8 targeted, 856K → ~50K files).
#   Guard renice: `sirsi guard --renice lsp` — deprioritizes LSPs to Background QoS.
#   Live result: language_server_macos_arm (2.7 GB) + 2× gopls (422 MB) → Background QoS.
#   CRITICAL: language_server_macos_arm added to PROTECTED process list after slay crashed IDE.
#   IDE Settings: Shell Integration disabled, gopls directory filters, file watcher exclusions.
#   CI Fix: Removed tracked `sirsi-menubar` binary causing Windows test failures.
#   Platform compute.go: Restored M4 family bandwidth values corrupted by sed rename.
#   Case Study 010: The Hot-Swap Catastrophe (P0 incident post-mortem).
#   Rule A18 (Incremental Commits) + Rule A19 (No App Bundle Mutations) codified.
# 2026-03-25: Session 18 — Menu Bar App + Horus Publish + Osiris Guardian
#   macOS Menu Bar Application (ADR-010): Phase 1 complete.
#   - cmd/pantheon-menubar/: stats.go, handlers.go, icon.go, main.go
#   - Headless mode: real-time RAM/Git/accelerator/deity stats (105ms collection)
#   - Pantheon.app bundle: make bundle → installable .app with Info.plist
#   - LaunchAgent: make install-launchagent → auto-start at login
#   Horus Auto-Publish (internal/horus/publish.go):
#   - Reads Thoth journal + case study markdown → generates styled HTML
#   - build-log.html + case-studies.html with Pantheon gold/lapis theme
#   - 92.8% coverage, 16 tests
#   Osiris Checkpoint Guardian (internal/osiris/):
#   - Detects uncommitted work, assesses risk (none/low/moderate/high/critical)
#   - Time-based escalation: 2+ hours since commit → critical
#   - FormatReport, Summary, StatusIcon for menu bar integration
#   - 92.8% coverage, 15 tests
#   Module count: 22 → 24 (osiris, horus/publish)
#   Binary count: 7 → 8 (sirsi-menubar)
#   Test count: 768 → 819+ (51 new tests, all passing)
#   Makefile: 6 new targets (build-menubar, bundle, publish, install/uninstall-launchagent)
# 2026-03-24: Session 16b — The Coverage Sprint & Antigravity Bridge (90.1% Hit)
#   Coverage breakthrough: 87.2% → 90.1% (Rule A16 established).
#   Injectable Providers: standard interface injection for signals and exec.Command (ADR-009).
#   Guard (89→91%), Ma'at (80→88%), Sight (78→93%), Profile (84→85%).
#   Antigravity CLI: `sirsi guard --watch` now starts the full bridge + AlertRing.
#   Note: Platform coverage is structurally maxed at 73.4% on macOS.
#   Canon update: ANUBIS_RULES.md → PANTHEON_RULES.md (v2.0.0). ADR-009 added.
# 2026-03-23: Session 14 — Brain Coverage + Homebrew Verification
#   Brain coverage sprint: 40.4% → 55.9% (exceeds 50% Ma'at threshold).
#   New tests: downloadFile (httptest), selectPlatformModel, classifyByHeuristic
#   (all branches), manifest JSON round-trips, containsSegment edge cases.
#   Found: splitPath infinite loop on relative '.' paths (documented, not fixed).
#   Homebrew verified end-to-end: brew tap SirsiMaster/tools && brew install sirsi-pantheon ✅
#   Both binaries (sirsi + sirsi-agent) installed to /opt/homebrew/bin/.
#   Case study updated: Ka 8.5s → 1.08s benchmarks added (Sessions 12–13).
#   Build log updated: Ka benchmark bar, pre-push gate 5s → 2s, Horus recursive win.
#   Note: update checker shows false upgrade (0.4.0 → 0.2.0) — version compare bug.
# 2026-03-23: Session 12 — Launch Execution + Performance Optimization (v0.4.0-alpha)
#   Homebrew PAT setup: HOMEBREW_TAP_TOKEN secret set in sirsi-pantheon repo.
#   homebrew-tools repo initialized with README.md + Formula/ directory.
#   GoReleaser brews section enabled (.goreleaser.yaml) — was commented out.
#   v0.4.0-alpha released with 6 platform binaries.
#   ADR-007 Unified Findings Portal + Horus designation added.
#   ADR-006 Self-Aware Resource Governance + yield module added.
#   ADR-008 Shared Filesystem Index accepted (Horus architecture).
#   .gitignore collision fix: unanchored 'pantheon' → '/pantheon'.
#   PERFORMANCE (dogfooding-driven):
#     Ma'at diff-based coverage: 55s → 12ms (4,583× speedup)
#     Horus shared filesystem index: walk once, all deities query
#     Horus Phase 2: pre-aggregated dirs + gob encoding (110MB → 31MB, 936ms → 2ms)
#     Horus Phase 2.5: FindDirsNamed eliminates dev walk
#     Weigh (Jackal) optimized: 15.6s → 833ms (18.7× speedup)
#     Quality verified: identical results (341 findings, 65.6 GB, 58 rules)
#     Pre-push gate: 65s → 5s (13× faster)
#     Feather Weight: 69/100 → 81/100
#     Canon linkage: 60% → 100% (10/10 commits)
#   DOGFOODING DISCOVERY:
#     Docker Desktop ghost: 64 GB unused VM images + cached layers
#     Investigation: zero Docker references in build/CI/deploy pipeline
#     Cleanup: Docker Desktop fully uninstalled, 65.6 GB → 1.6 GB total findings
#     Product thesis validated: founder didn't know until Pantheon told him
# 2026-03-23: Session 11 — Full Pantheon Audit + Modular Deities (v2.1.0)
#   Walkthrough of every conversation since genesis (completion audit).
#   Fixed phantom domain sirsinexus.dev → sirsi.ai in SirsiNexusApp.
#   Wired structured logging (slog) into ka and cleaner cores.
#   Updated ADR-005: Ra as Hypervisor, Seba as Mapping Focus.
#   Portfolio Standard v2.1.0: added modular deployment + referral rules.
#   Canon sync: SECURITY, CONTRIBUTING, CHANGELOG, VERSION in all 5 repos.
#   Ka coverage sprint: 41.9% → 65.3% (exceeding 60% goal).
#   Pre-push hook updated: added Ma'at diagnostics + Agent health checks.
#   MCP version fix: v0.2.0-alpha → v0.3.0-alpha in code.
# 2026-03-23: Session 10 — Ma'at built + Pantheon unification
#   Built Ma'at QA/QC governance agent (internal/maat/): 4 source files, 57 tests
#   Core types: Verdict, Assessment, CanonLink, Report, Assessor, Weigh()
#   Three domains: coverage (go test -cover), canon (git log), pipeline (gh CLI)
#   CLI: anubis maat [--pipeline] [--coverage] [--canon] [--json]
#   ADR-004: Ma'at QA/QC Governance canonized
#   ADR-005: Pantheon Unification canonized — all deities as sub-systems
#   Portfolio Standard v2.0.0: 26 universal rules, 3 canon tiers, Pantheon reqs
#   Deployed Pantheon governance to all 5 repos: real Thoth memories,
#     GEMINI.md + CLAUDE.md, Portfolio Standard, session workflows
#   Pantheon coverage: ~20% → ~75% across portfolio
#   All 5 repos pushed and clean
# 2026-03-23: Session 8 — platform wiring + CI lint fix + pre-push gate
#   Wired Platform interface into cleaner (3 runtime.GOOS → platform.Current())
#   Wired Platform interface into mirror (OpenBrowser + PickFolder)
#   Removed duplicated moveToTrash(), protectedPrefixes from cleaner
#   Tests use platform.Set(&Mock{}) for cross-platform testing
#   Fixed 8 golangci-lint errors (gofmt, govet/unusedwrite, misspell) across 5 files
#   CI green after 5 consecutive failures
#   Pre-push hook: .githooks/pre-push (gofmt + go vet + golangci-lint + go build)
#   Proposed: anubis maat — pipeline purifier (CI monitoring + auto-remediation)
# 2026-03-22: Session 7 — statistics audit + production polish
#   Structured logging: internal/logging/ (slog, --verbose, --quiet, --json)
#   Platform abstraction: internal/platform/ (Darwin, Linux, Mock)
#   3 case studies: Thoth, Mirror, Ka (all verified data per Rule A14)
#   CI fixes: 4 platform skip guards, homebrew tap disabled
#   v0.3.0-alpha released on GitHub (6 binaries + checksums)
# 2026-03-22: Statistics audit — corrected all inflated claims across 12 files
#   Scan rules: 64→58 (verified). Tests: ~395→453 (verified).
#   Removed fabricated cross-repo savings. Removed "3M tokens in 11 sessions."
#   Canonized Rule A14 (Statistics Integrity) and Rule A15 (Session Definition).
#   ROI script fixed: naive commits/5 → gap-based heuristic + methodology note.
# 2026-03-22: Case study system — dogfooding narratives + ROI tracking
#   Created: docs/case-studies/, scripts/thoth-roi.sh, pitch deck stub
# 2026-03-22: Safety-critical coverage sprint — cleaner 49%→77%, ka 19.5%→42.7%
#   Added: 30 cleaner tests (DecisionLog, DeleteFile, CleanFile, DirSize, constants)
#   Added: 28 ka tests (isInstalled, countFiles, mergeOrphans, Clean, constants)
# 2026-03-22: Launch prep — goreleaser verified (12 binaries), launch copy + demo updated
# 2026-03-22: Test coverage sprint — 303→~395 tests, 15/17 modules tested
# 2026-03-22: Thoth unified as canonical session manager (memory + context monitoring)
# 2026-03-22: Build-in-public HTML page (Swiss Neo-Deco, Cinzel+Inter, emerald+gold)
# 2026-03-22: Cross-linked Anubis ↔ SirsiNexus Portal (Anubis→Ra messaging)
# 2026-03-22: BUILD_LOG.md sprint chronicle + CHANGELOG v0.3.0 expansion
# 2026-03-22: README badges (303 tests, building in public)
# 2026-03-22: Codebase safety audit — 6 bugs fixed (filepath.Abs, moveToTrash)
# 2026-03-22: thoth-init standalone CLI (npx thoth-init, non-interactive mode)
# 2026-03-21: Thoth knowledge system canonized — memory, journal, skill, MCP tool
# 2026-03-21: Graceful shutdown (SIGINT handler) + drag-and-drop UX fix
# 2026-03-21: GoReleaser CI fix (stale config, brews vs homebrew_casks)
# 2026-03-21: Three-phase partial hashing (27.3x speedup)


# Pre-July Session Decisions (hook payloads + duplicate router snapshots stripped):

# 2026-08-05: Durable SNE broker quarantine for Nexus-owned inference service
#   Added a reversible `sirsi gemma serve --quarantine|--restore` lifecycle that
#   renames only ai.sirsi.gemma-broker's plist, blocks both Pantheon self-healing
#   paths, preserves quarantine across `sirsi setup`, and requires explicit
#   bootstrap/readiness before restoration. Readiness failures re-quarantine;
#   conflicting canonical/quarantine definitions fail visibly. No live label was
#   changed by tests. Claude Nexus remains the service and SNE workstream owner;
#   Pantheon owns only the quarantine mechanism. Full Go suite and vet pass.

# 2026-08-05: SNE-52 Universal Task Ledger schema v7 implementation
#   Implemented the ADR-054 Part B contract on codex/router-unification-store-v7:
#   one additive v4→v7 migration with explicit migration targets (v5/v6 stay
#   reserved), legacy commissioning backfill, all drill-down fields, a single
#   shared 4h derived-liveness constant, evidence-gated pass state, governed
#   charter/stage updates, deduped typed links, JSON timeline replacement, and
#   atomic additive task-owned token/duration counters. Added CLI flags and
#   v4 migration/default/liveness/governance/accounting tests. `go test ./...`,
#   `go test -race ./internal/routerstore`, and `git diff --check` pass.

# 2026-06-10: Flagship train landed (19 PRs to main) + 2 governance fixes. (claude-pantheon source-edit;
#   claude-home binding reviewer while codex OOO.) Merged: A28 registry-trust (#24 per-resume-mint fix +
#   ADR-029 worktrees, #25/#29/#30), Rails A/B/C (#19/#22/#18), UX pains (#26 TCC .app bundle, #27 fsnotify
#   live-refresh), gemma (#11/#13), #31/#21/#33/#9/#28/#14, #34 sirsi insight. Two silent failures surfaced+fixed:
#   (1) #33 (A1 safety) auto-merged pre-verdict via rebase-push tripping armed auto-merge → RULE: safety-tier
#   never auto-merges before binding verdict; (2) router fragmentation — `sirsi router` from a worktree cwd read
#   the worktree's STALE git-snapshot copy, silently dropping a routed review request. FIX 1 (#35, 4eb6792,
#   ADR-029 Amendment 1): FindRepoRoot resolves the MAIN worktree root (git --git-common-dir) not cwd; A16/A21
#   seam + 3 tests; dogfooded. FIX 2 (#36, aa41706): .github/workflows/binding-hold.yml required-check job —
#   passes unlabeled / FAILS when `binding-hold` labeled → branch protection blocks merge (auto-merge waits) until
#   a binding reviewer clears it. Added job `binding-hold` to required_status_checks (strict). Proven live:
#   label→FAIL→BLOCKED→reviewer-clears→re-run→CLEAN→merged. Closes the #33 bypass structurally; extends Ma'at gate
#   (A25/A28). OPEN: #8 (router -2626 LOC, HOLD-FOR-CODEX, labeled binding-hold), #32 (ADR-030 Swift, operator-GUI
#   +fresh-codex gated, labeled binding-hold). DEPLOY-PENDING: installed sirsi binary predates the canonical-router
#   fix (repo-root-cwd workaround meanwhile) — lands on next user-authorized build/reinstall w/ TCC-bundle+self-update.
# 2026-06-04: Setup wizard for Monday VC build (commit ff8a448, branch feat/setup-wizard, pushed).
#   `sirsi setup` rebuilt: report -> guided 3-step wizard (Dependencies/FDA/Agent-wake) over new
#   shared internal/setup/ engine (CDD #5: one engine, CLI + menubar-terminal both render it).
#   Real TTY prompts before each action; pipe/dev-null/CI = report-only (golang.org/x/term, not
#   os.ModeCharDevice which mis-classified /dev/null). Fixed thoth-init/sync/compact false-missing
#   (thoth ships in the binary). main.go FDA pre-check now uses engine. Open: dedicated fullscreen
#   TUI wizard screen? (ADR-020/TUI_DESIGN_PROOF-gated) or menubar->terminal suffices. codex review owed.
# 2026-06-04: Canonicalized machine (1 versioned signed sirsi, zsh completion, no drift); sirsi fix resolver + safe PPID-orphan-kill (funnel BLOCKED pending codex User-metadata gap); menubar zsh close-prompt fix (read _). Open: install wizard, orphan User fix, mds_stores sudo.
# 2026-06-04: sirsi fix heuristic resolver (no LLM) — answers every finding; safe PPID-narrowed orphan-kill (KillTrueOrphans, PPID<=1 only, --yes never kills, 4 regression tests). Funnel diagnose->fix + menubar BLOCKED pending codex re-review (42588a9).
# 2026-05-31: CTR auto-registration Phase 1 — `sirsi thread discover` (commit 10a97b7, codex APPROVED).
#   Reconciles live agent processes into threads.json: bounded pgrep/lsof → cwd → agents.json match,
#   registers mappable repo-launched sessions anchored to their PID. Home-launched=unmappable (never
#   registered); same-cwd same-surface duplicate=ambiguous (never guessed, Rule A23). Pure
#   ReconcileDiscovery + 9 tests (internal/router/discover.go); CLI (cmd/sirsi/threaddiscover.go) with
#   --self (Phase 2 hook) + stable JSON. Phase 1.5: wired into hourly sweep.sh (with scout lane).
#   Why 0 threads registered post-reboot: every session launched from $HOME → no repo identity.
#   Phase 2 (SessionStart hook→discover --self) approved, not wired. Phase 3 (deliver into live
#   session via remote-control) spike-gated. Feeds ADR-021 (Osiris workstation-scoping).
# 2026-04-04: Session: ProtectGlyph, Stele Universal Event Bus, SIRSI_MASTER_PLAN, Deity Registry (Rule A25). Shipped v0.10.0. All deities inscribe to Stele. Ma'at owns all quality gates across all repos. Pre-push hooks corrected. Case studies written. Full lifecycle LoE assessed for all 4 repos. Next session: KV cache optimizations, token usage improvements, agentic harness enhancements, then full-throttle dev on FinalWishes Sprint 5-6 and Assiduous Sprint 11-13.
# 2026-04-02: Session: Seshat v2.0 adapters built, 22 plugins installed, screenshots MCP, Sirsi Orchestrator, GitHub CI cleanup (225+ runs), NexusApp workflow fix, Go 1.24 compat, 78G iCloud migration for M5 transfer. All repos clean and pushed.

## Conduit run 2026-08-08T01:10Z (completion loop, iteration 6)

The binder came back and bound #668 and #669; both merged, and #668's binary was redeployed
(pristine-clone build under the shlock guard, schema gate 16==16, verified live: `reset-attempts`
prints its documented help and refuses cleanly on an unknown task).

The main work this iteration was `lifecycle-fence-lost`, which explicitly asked for re-diagnosis at
`setThreadConsumerCapable` rather than more retries. Found two independent gaps, not one. First: the
call site had zero retry at all — PR #619's `retryOnLostFence` only wraps the two reap passes, so
this write was never protected against the transient contention #619 exists to survive. Second, and
the one that actually mattered: adding the retry alone was **not sufficient**. A real-store
reproduction — register a thread, immediately call `setThreadConsumerCapable` — failed all three
retry attempts identically, which is not what transient contention looks like. The cause: this is
the only lifecycle mutator in the package that never advances `LastSeenAt`, so its compare-and-swap
write depends entirely on the fence's fallback branch, a raw byte comparison of the JSON payload
that is not equivalent to "this write is newer" and can fail permanently when the encoding doesn't
happen to sort greater. Added the same `LastSeenAt` bump every other mutator already performs. The
negative control that actually proves this — reverting only the `LastSeenAt` line with the retry
still wired — reproduces the identical 3/3 failure, which is what turns "I fixed something" into "I
fixed the right something." Six tests, three mock-based mirroring the existing fence-retry tests
exactly, one real-store end-to-end check against the actual production call site. Shipped as PR
#671, and it directly answers `fence-retry-budget-underprovisioned`'s open question: the budget
was never actually shown insufficient, because this call site was never protected at all.

Then a self-caught P0: verifying `#668`'s deploy by running the CLI myself, the ledger header had
reverted to the exact unlabeled format `#663` fixed hours earlier. Root cause was procedural, not
logical — `#668`'s branch was built by copying `cmd/sirsi/routerledgercmd.go` whole from the primary
checkout, which has been on a feature branch all session and never pulled main, into a fresh worktree
branched off `origin/main`. `#663` had touched that same file after the primary checkout's copy went
stale, so the whole-file copy silently reverted it. None of `#668`'s own tests were anywhere near the
function that regressed, so it shipped to main and was deployed to production before anyone noticed.
Fixed in PR #672, restoring `#663`'s format verbatim and adding two tests that capture the actual
stdout rather than the underlying struct fields — the fields were never touched, only the print
statement consuming them, so a field-level test would have stayed green through the whole incident.

Then audited whether the same pattern hit the other three PRs shipped tonight, all built the same
way. Diffed each branch directly against `origin/main` — not against the primary checkout, which
would have repeated the exact blind spot — and all three came back as exactly their intended change
with nothing else reverted. `#668` was an isolated collision: the only file two different fixes both
touched on the same night. Filed the general failure mode (`worktree-cp-clobbers-stale-file`) with
the process fix for the rest of this session: diff a file against `origin/main` before copying it
from a long-lived checkout into a fresh worktree, and if it differs beyond this session's own edits,
sync before overwriting.

## Conduit run 2026-08-08T01:15Z–01:26Z

Inbox zero for claude-home. Both in-flight PRs (#672 P0, #671) are green on all five
required contexts — verified as the set difference against branch protection, not
`mergeStateStatus` — and neither is bindable by me (both are claude-home's own work).
Their bind requests are minutes old, so nothing there was stale.

The run's real finding is why those binds have not landed. `router doctor --fix` reported
all six codex-home items as `wake disabled (mechanism: none)`, which reads as a delivery
outage and is false: 27 of the 28 registered agents are `wake:none` by design, because
consumer-mode lanes deliver through `consumer.command`, not the wake mechanism. codex-home
has 406 dispatches today. The doctor wake pass reads only `wake.mechanism`, so it renders
the fabric's normal delivery path as a fabric-wide failure — claim "these items cannot be
woken", scope "one field" — and worse, it masks the actual failure. Filed as
`doctor-wake-pass-blind-to-consumer-lanes`.

The actual failure: codex-home sits at `fruitless=7` against `wakeLoopFruitlessQuarantine
= 10` (internal/router/dispatchgate.go:34), backoff escalated 8m→16m→32m→1h over seven
consecutive no-progress dispatches, fleet renders it UNROUTABLE, untouched 2h1m. I
re-verified before blaming auth, per step 6: a live `codex exec --sandbox read-only` probe
returned OK at exit 0 on codex-cli 0.147.0-alpha.6.5 / gpt-5.6-luna. The CLI works and the
consumer still exits without draining. #639's C1 gate scores only after `run.running()`
goes false, so these are genuine fast exits, not a too-short measurement window — I read
wake.go rather than inferring the gate's shape from its log lines. Three more fruitless
dispatches quarantine claude-home's sole reviewer while a P0 that fixes a live production
regression waits on it. Filed as `codex-home-reviewer-starved-3-from-quarantine`. The
dispatch path discards consumer stdout/stderr, which is why seven failures produced no
evidence of cause; that is the next thing to fix.

Not acted on, deliberately: `horus.agent-router`, `triage`, `liveness-watch`,
`fabric-watchdog`, `hypergraph.digest` and `wake.claude-home` are all absent from launchd
because their plists carry an explicit `.OFF-owner-20260807` suffix. `sirsi diagnose`
renders one of them ("liveness-watch not installed") as a defect; it is an owner decision,
and the step-3 canon in the task file asserting horus needs a live PID is stale against it.

Broker healthy and measured on a driven window: Δ(active+cache) = 0.25 MB over 3 requests,
≈0.0001 GB/req against a known-bad 0.48. Pool 21.25 GB, peak 14.8 GB, well under the 38 GB
row filed against claude-nexus, and swap fell from the 15360 MB / 93.5% of that report to
7168 MB / 78%. No new crash or Jetsam reports. Board 200. Reconcile healed one reaped
codex-pantheon thread to a successor and re-emitted the known `lifecycle-fence-lost`
warning that #671 addresses. Prune found nothing in either the thread registry or the
90-day retention window. Headless claude session count: 0.

## Conduit run 2026-08-08T01:26Z–01:45Z (continuation)

Root-caused the reviewer starvation and it was not codex-home's problem, it was every
codex lane's. codex 0.147.0-alpha.6.5 routes all shell commands through a sibling helper,
`codex-code-mode-host`, and Code Mode fails closed when it is absent. `codex` on PATH is a
symlink into /Applications/ChatGPT.app, and codex resolves the helper relative to the
SYMLINK's directory (~/.local/bin) rather than the real binary's, so it looked for a file
that was never installed there. Every consumer booted, failed its first tool call with
`os error 2`, and exited with status 0 — fast, silent, and scored as no-progress. The app
bundle restamped at 17:19 EDT and the fruitless dispatches began at 19:32. Fixed with a
symlink to the version-matched host in the bundle. Verified at the behavior level, not the
counter: codex-home's next natural dispatch (21:35, unforced) drained depth 6 to 0 in two
minutes and returned six reasoned dispositions.

The reason this cost four hours is that three separate instruments each reported health
they could not observe. The conduit's own step-6 probe (`--print "respond with OK"`)
requires no tool call, so it returned OK at exit 0 throughout, and I repeated its verdict
in my first report of this run before the contradiction forced me back. `router doctor`
cried `wake disabled (mechanism: none)` on a lane with 406 dispatches, because 27 of 28
agents are wake:none by design and deliver via consumer.command. And the pre-existing
spawn metric never matched codex lanes at all. Three rows filed; the probe one matters
most, because canon still instructs the next run to trust it.

codex-home then refused to bind, correctly, on `error connecting to api.github.com`. That
is its sandbox, not an outage — gh succeeds from my shell at the same moment, and
~/.codex/config.toml declares no [sandbox_workspace_write] network_access key, which
defaults network off. So no codex lane can verify a PR. Granting an autonomous sandbox
network is a security decision, so it went to the owner as a decision card with three
options rather than being flipped here.

#672 and #671 merged at 01:20Z during the run. Built from a pristine clone (HEAD d92be2f7),
confirmed both merge commits are ancestors, gated the candidate against the live store
(v16 = v16, not inverted), rm-then-cp-then-signed, and verified the artifact rather than
the command: the ledger header now reads `items: 6 open … — tasks: 40 open · 1 blocked`
against the old ambiguous `6 open`. That is the whole "0 open over 40 rows" complaint — the
counter was counting items while the rows were tasks. Installed 1470a895d995aa37.

Deliberately not done: I did not restart the 20+ wake lanes to activate #671, which lives
in threads.go/wake.go and is therefore still dormant in every long-lived loop holding the
old image. With #636's dispatch leak open and codex lanes newly able to execute, a
simultaneous kickstart risks a spawn storm. Recorded with its exact command instead.

## Conduit run 2026-08-08T02:20Z

Closed the previous run's deliberately-deferred action. The 22 long-lived `router wake-loop`
processes were all started before the 21:39 EDT install of `1470a895d995aa37` — so #671 was merged,
deployed and dormant fleet-wide, and, more importantly, so was #639's progress gate. #639 in fact
merged at 2026-08-07T22:30:20Z, i.e. the condition the last run set ("do it staged, or after #639
lands") was already satisfied; the loops were running a PRE-gate image, which inverts the risk —
restarting arms the leak fix rather than exposing the fleet to it. Restarted staged: 4 lanes first
(claude-nexus, codex-home, claude-deck, claude-pantheon), verified each logged `started (…
consumer=true)` and then `inbox depth -1 -> 0` with zero dispatch, then the remaining 18. All 22 now
run the current binary; headless `claude --print` count 0 and `codex exec` count 0 after the
restart, so no spawn storm. Merged SirsiNexusApp #279 (`d6097484994b`) after a source-deep bind:
the fix adds `.Sc{zoom:1 !important}` inside the print media block, and I verified against the
branch file rather than the diff that fitSlide() sets an INLINE `sc.style.zoom` — which makes the
`!important` load-bearing rather than decorative — and that `.Sc` is the wrapper on every slide, not
just the active one. sirsi-pantheon is at ZERO open PRs. Broker measured clean over 3 driven
requests (pool delta -0.0003 GB over d=3, i.e. -0.0001 GB/req against a known-bad 0.48). Router
inbox zero for claude-home; the 5 open items are all owner/user surface and stay open. Swap is the
one number to watch: 10.9 GB of 12.0 GB consumed (88.8%), up from 78% last run — free RAM reads a
reassuring 88%, which is exactly the hollow metric.

## Conduit run 2026-08-08T03:10Z
Quiet run — first pass since the 22:12 EDT mass-restart onto the gated binary, and it is the
first run that can prove #639 works in production rather than assert it. All 22 wake lanes log
`alive, inbox depth 0 unchanged` on a ~16-minute heartbeat and spawn nothing: headless
`claude --print` sessions 0, `codex exec` 0. Pre-gate, every one of those cycles was a session.
Broker measured over 3 DRIVEN requests: pool 21.343 GB -> 21.343 GB, delta 49,152 bytes = 0.0000
GB/req against a known-bad 0.48 — clean, left alone despite `diagnose` rendering it 🟡 82/100 on
the `phys_footprint` finding (the documented false positive). Healed: `thread reconcile` reaped
thr-f4c73033e8976836 [codex-pantheon] into successor thr-964e0a28a5cbc18c; `ccd reap` killed one
leaked completed router-conduit session. Router at 5 open, all owner/user `owner-surface` items —
surfaced, never closed. Zero open PRs on sirsi-pantheon and SirsiNexusApp; FinalWishes #128/#127
still DIRTY and belong to their lane agents. Swap 86.1% (10,582/12,288 MB), down from 88.8% —
still the metric to watch, while free RAM reads a hollow 87%. The three codex ledger-rot backlogs
(inference 35, mail 9, pantheon 2) cannot move while the no-network owner decision is open.

## Conduit run 2026-08-08T04:15Z

Quiet pass; the #639 progress gate continues to hold in production — all 22 wake lanes logging
`alive, inbox depth 0 unchanged` on heartbeat with zero spawns (`claude --print` = 1, this session;
`codex exec` = 0). Broker measured with three DRIVEN requests (1→4): pool 21,247,489,378 →
21,246,514,874 bytes, i.e. −0.97 MB total = 0.0000 GB/req against a known-bad rate of 0.48 GB/req;
`sirsi diagnose` 🟡 82/100 again names it a "memory hog at 19.9 GB", the documented phys_footprint
false positive, and was ignored. Broker had restarted since the last run (requests counter reset
43→1), so this is a fresh build measured honestly rather than a young process flattering itself.
Healed: `thread reconcile` reaped→successor three records (codex-home ×2, codex-pantheon);
`ccd reap --apply` killed one leaked completed conduit session (pid 62336, idle 60min). Prune 33→33
(nothing terminal), retention within the 90-day window, 0 BINARY_MISSING sentinels, board 200 on
:8734. Nothing merged: pantheon and SirsiNexusApp are at zero open PRs, FinalWishes #128/#127 both
DIRTY and belong to their lane agents. Swap needs reading in absolutes this run — 6,544/7,168 MB
reads as 91.3% versus last run's 86.1%, but the kernel shrank the backing file from 12,288 MB, so
absolute swap consumption actually fell ~4 GB; free RAM 30%. Ledger rot is now four codex lanes
(inference 35, mail 9, pantheon 2, home 1 — the last a newly transferred PR-14 admission-cap
review), all of it downstream of the single open owner decision 20260808-014200 on codex network
access; one blocker, already surfaced, so no second escalation was filed.

## Conduit run 2026-08-08T05:10Z

Quiet pass with one real find. Inbox zero for claude-home; 5 open items unchanged, all
`owner-surface`. Broker (pid 44925, unrestarted, port 8477 from the port file) measured over 3
DRIVEN requests: pool 21,246,982,626 → 21,246,490,290 B = −0.47 MB, **0.0000 GB/req** against a
known-bad 0.48 — clean, not bounced. Headless `claude --print` count **0**, so the #639 gate is
still holding across 22 wake lanes. Swap read 6,256/7,168 MB: the percentage looks alarming at
87%, but absolute used swap fell again (6,544 → 6,256 MB), which is the honest direction. The
Jetsam at 2026-08-07T22:03Z (victim `sirsi-inference.test`, 51.4 GB at 16 KB rpages) was already
routed and closed as `20260807-221439` → claude-inference; no duplicate filed. `thread reconcile`
healed the same 3 records as last run (codex-home ×2, codex-pantheon → successors); prune 33→33,
0 BINARY_MISSING, retention inside 90 days, board 200 on :8734. The find: **both self-hosted CI
runners, `m5-sirsi` and `m5-sirsi-2`, are offline** — last successful self-hosted build was
01:23:06Z, and PR #673 has had `Test` and `Build (self-hosted, macOS, ARM64, 1.25)` queued 54
minutes with zero elapsed. Both are required contexts, so this is a merge freeze on the whole
repo, and it is distinct from the codex-network card (that blocks review, this blocks checks).
Not healable here — no runner install, process or launchd job exists on this MacBook; the service
lives on the M5. Escalated once as `20260808-051202` with three concrete options. PR #673 left
alone on both counts: it cannot go green, and it is this lane's own branch (no self-review).
FinalWishes #127/#128 still DIRTY and non-trivial, left to their lane agents.

## Conduit run 2026-08-08T06:10Z

One real find: an **unrouted Jetsam recurrence**. `JetsamEvent-2026-08-08-000302.ips` (04:03Z) killed
`sirsi-inference` (1,305,811 rpages = 21.4 GB) and `sirsi-infer-campaign` (822,093 rpages = 13.5 GB) —
34.9 GB of one process family on a 48 GB host, with the kernel reclaiming down through WindowServer,
Claude renderers and gopls. `sirsi-infer-campaign` appears in no router item, open or closed: the
earlier card `20260807-221439` covered only the 22:03Z event (`sirsi-inference.test` 51.4 GB) and is
closed, so whatever satisfied it did not bound this binary. Routed as `20260808-061042` →
claude-inference with the forensics and an explicit exoneration of the broker. The 05:10Z run missed
this file, which is why the check is "new .ips since last run" and not "new .ips since the last one I
filed on".

Broker measured clean on a DRIVEN window, not an idle one: pool 21,247,003,402 → 21,246,970,634 bytes
across requests 15→18 = **−32 KB, 0.0000 GB/req** against a known-bad 0.48 GB/req. Peak 13.56 GB under
the 20 GiB scheduler limit. Not bounced. `thread reconcile` healed the same 3 threads as the last two
runs (codex-home x2, codex-pantheon), which looked like a heal that would not stick — a second
immediate pass returned "no dirty exits to heal", proving idempotency and reclassifying them as fresh
dead threads per interval rather than a stuck repair. `ccd reap --apply` killed one leaked conduit
session (pid 96557, idle 61min). Prune 33→33, retention inside 90d, 0 BINARY_MISSING, board 200 on
:8734, 0 headless sessions (#639 gate still holding across 22 live wake lanes).

Both self-hosted runners (`m5-sirsi`, `m5-sirsi-2`) remain `offline`, so `Test` and `Build
(self-hosted, macOS, ARM64, 1.25)` are still pending on PR #673 after ~2h — the pantheon merge freeze
persists, already escalated as `20260808-051202` and deliberately not re-filed. Swap absolute fell
again, 6,256 → 6,147 MB; free RAM 32% remains the hollow metric and was ignored.

## Conduit run 2026-08-08T07:20Z

All-green pass, one negative finding worth recording: **no third Jetsam event**. The 04:03Z
recurrence routed last run as `20260808-061042` was not followed by another — zero new `.ips` in
either DiagnosticReports tree since 06:15Z (newest is a Chrome crash from 07T23:25). Broker
measured clean under a driven window: requests 22→25, pool 21,246,348,042 → 21,246,315,274 bytes
= **−32 KB over 3 requests, 0.0000 GB/req** (known-bad 0.48). pid 44925 unchanged, not bounced;
`diagnose`'s 🟡 "memory hog at 19.9 GB" is the `phys_footprint` artifact, ignored as canon requires.
Swap absolute fell a fourth consecutive run, 6,147 → 6,091 MB used. Inbox zero for claude-home.
Self-hosted runners `m5-sirsi` / `m5-sirsi-2` still `offline` (~6h) — merge freeze holds, single
escalation `20260808-051202` stands, no second card filed. `thread reconcile` healed the same three
dead-thread successors (codex-home ×2, codex-pantheon); settled behaviour, not a stuck heal.
`ccd reap` killed a leaked conduit-supervisor session for the **second consecutive run** (pid 3256,
idle 60min; pid 96557 last run) — the reaper catches it each time, so it is contained, but two in a
row makes it a pattern rather than a one-off. Wake lanes wrote nothing since 03:10Z and headless
session count is 0: the #639 spawn gate continues to hold.

## Conduit run 2026-08-08T19:1xZ

The 18-hour CI freeze broke. `m5-sirsi` and `m5-sirsi-2` are back `online busy=false`, and pantheon
**#673** went from `BLOCKED` to genuinely green — all five required contexts (`Lint`, `Test`,
`Secrets Scan (gitleaks)`, `Build (self-hosted, macOS, ARM64, 1.25)`, `binding-hold`) report
SUCCESS, checked against branch protection's list rather than the `mergeStateStatus` badge. #673 is
this lane's own branch, so it does not get merged here: a bind request went to codex-home as
`20260808-191103`, carrying the file scope (`.thoth/journal.md`, `.thoth/memory.yaml` — docs only,
no code) and an explicit note that `binding-hold` SUCCESS proves only the label's absence, never
that a bind exists. codex-home still has no network, so that request is a handoff, not a gate; this
lane kept working. Owner card `20260808-051202` (runners offline) is now factually resolved but
stays open — it is `to: owner`, and this pass never closes those. Everything else held: broker
21,243,857,674 bytes byte-identical across 7 requests (0.0000 GB/req, twelfth consecutive identical
read), no new Jetsam or crash reports, zero headless `claude --print` sessions, board 200, thread
reconcile healed the same three fresh successors, and `ccd reap` took the usual single stranded
conduit session. Swap moved 5,767 → 6,026 MB used of 7,168 — first movement in four reads, worth
watching but well short of the 98%-consumed pathology.

## Conduit run 2026-08-08T17:2xZ — reboot wiped the LaunchAgent domain; fabric + CI restored

The machine rebooted at ~16:43Z (uptime 27min at start of run) and **nothing in
`~/Library/LaunchAgents` auto-loaded at login** — 60 services live in `gui/501`, zero of them
`ai.sirsi.*` or `actions.runner.*`, despite `RunAtLoad=true` on the plists. This is not an owner
decision and not a quarantine: it is a failed login-load. Restored the full owner-live set by
bootstrapping every exact-suffix `ai.sirsi.*.plist` (the `.OFF-owner-*` / `.quarantined` files carry
other suffixes, so parked lanes are excluded by construction, not by judgement): **26 bootstrapped,
0 failed, 27 services live** — gemma-broker, router-board, menubar, liveness-watch and all 24
`router.wake.*` lanes. Canary-verified with router-board first (pid 7294, HTTP 200) before the bulk
pass.

Two lanes latched at **exit 78 (EX_CONFIG)** and would have stayed dead: `wake.codex-io` and
`wake.codex-mail` had `ProgramArguments[0] = /tmp/sirsi-wi`, a path macOS wipes on every reboot,
while all 22 sibling lanes point at `~/.local/bin/sirsi`. That is a latent defect predating this
reboot — those two lanes fail-closed after *any* restart. Repointed both at the real binary
(`.bak-tmppath-20260808` kept), bootout+bootstrap since 78 is cached and `kickstart` cannot clear
it; both now hold live PIDs.

The same login-load failure took down all 10 `actions.runner.*` agents, and the GitHub API confirmed
`m5-sirsi` and `m5-sirsi-2` **offline** — the substance of open owner card `20260808-051202`.
Bootstrapped all 10; both runners re-registered **online**, verified at the API rather than the exit
code. Card stays open (`to: owner`, never closed by the conduit).

Broker came back as `sirsi-inference` on 8477 (port file agrees; the transient `sne-server` pid 7159
on 8238 died within 30s pre-restore). First measurement window was **invalid** — baseline was a cold
unloaded model at 0 bytes, so the +13.0 GB it showed is weight load, not leak. Re-measured warm:
3 driven requests (3→6), pool `13,005,467,774 → 13,005,467,774` = **0 bytes, 0.0000 GB/req** against
a known-bad 0.48. Active settled to 12.656 GB, byte-identical to the pre-reboot steady state.
Swap is **0.00 MB of 0** (fresh boot), clearing the ~6 GB standing pressure of the last several runs.
Router unchanged: 6 open, all owner/user `owner-surface` cards; claude-home inbox zero; FinalWishes
#127/#128 still DIRTY and their lanes'. Headless spawn count 0.

## Conduit run 2026-08-08T22:1xZ

Second total fabric outage in one boot session, and the first one where the cause is narrowed. At
session start `launchctl list` showed **0** `ai.sirsi` and **0** `actions.runner` jobs — but boot was
16:43 EDT and the *previous* run had already restored everything at 17:11:47, so this was not the
login-load failure that run hypothesised. The jobs carry `RunAtLoad=true` **and** `KeepAlive=true`,
and they were absent from `launchctl list` entirely rather than sitting at PID `-`, which means they
were **booted out**, not crashed or exited. Their wake logs stop dead at their 17:11:47 "started"
line, ~2 minutes before the prior session ended. Unified logging had no retention for the window, so
the actor is still unidentified; the sharpened signal is "bootstrapped from inside a conduit session,
gone shortly after that session ended." Restored 26 `ai.sirsi` + 10 `actions.runner` and, unlike last
run, **re-verified survival 12 minutes later** (26/10 still live, 23 wake-loops, all 10 GitHub runners
`online`). Corrected two errors carried in the prior state: `ai.sirsi.horus.agent-router` has no exact
`.plist` at all (only `.OFF-owner-20260807`), so horus is owner-parked and was not revived; and
`ai.sirsi.pantheon.plist` *is* an exact plist, so the prior run's "quarantine excluded by
construction" was false — it was excluded explicitly here instead. Broker is the new
`sirsi-inference` binary on 8477, warmed first (a cold read is weight-load, not leak) then measured
across 4 driven requests: pool `13,005,467,774 → 13,005,467,774`, **0 bytes, 0.0000 GB/req**, and
byte-identical to the pre-reboot steady state. Filed one owner decision card
(`20260808-221547`): `liveness-watch` reports the owner-quarantined menubar as WEDGED, exits 1 and
stays dead, files a decision to `claude-pantheon`, whose live lane then relaunches the app — observed
at 18:14:30 with no action from me. The fabric is repairing an owner decision as if it were a fault,
and the dead liveness-watch takes the swap/session-leak/disabled-label checks down with it. Jetsam at
18:02:56 was benign (idle system helpers, no sirsi victim). Router 7 open; claude-home inbox zero.

## Conduit run 2026-08-08T23:10Z

Fabric SURVIVED — the prior run's teardown-on-session-exit hypothesis did not reproduce: 26
`ai.sirsi` LaunchAgents + 1 menubar GUI instance + 10 `actions.runner` still loaded ~1h after the
prior conduit session ended, 23 wake-loops live, board HTTP 200. Broker `sirsi-inference` pid 21182
(`gemma-4-12B-it-8bit`, 8477) measured **0 bytes over 3 DRIVEN requests** (requests 8→11,
active+cache 13,005,467,774 byte-identical) = 0.0000 GB/req against a known-bad 0.48; do not bounce
it on `diagnose`'s 12.4 GB `phys_footprint` badge. CORRECTION to the prior state file, which the
owner card `20260808-221547` also overstates: `ai.sirsi.liveness-watch` does **not** "exit 1 and stay
dead". It is a `StartInterval=900` periodic job — `runs=4`, `last exit code = 0`, and `state = not
running` between cycles is its normal resting state, not a corpse. All five checks ran in each of the
last three cycles and every one returned `ok`; the swap/session-leak/disabled-label checks were never
lost. The card's substance stands (the fabric does treat the owner-quarantined menubar as desired
state and relaunched it, pid 28442 still up, and liveness-watch scores "menubar ok running"), but its
impact claim does not — a not-running periodic job read as a dead daemon, green-surface-over-dead-
thing inverted. Its stderr also carries three `SendGuarded: quota: database is locked (517)` route
blockers, so some of its findings never reached the router at all. No item verb exists to amend a
filed card, so no second owner item was filed (no nags); the correction lives here and in thoth.
Otherwise all-green: no new crash/Jetsam reports, swap 0.00 MB, 0 BINARY_MISSING, reconcile healed
the usual 3 codex successors, prune 0 (32→32), retention nothing, `ccd reap` archived 1 (the prior
conduit session). Router 7 open, all `to: owner`/`to: user` — claude-home inbox zero; doctor's 7
"unsupported wake mechanism owner-surface" are correct. PRs unchanged: pantheon 0, Nexus 0,
FinalWishes #127/#128 both DIRTY with non-trivial conflicts owned by their lanes.

## Conduit run 2026-08-09T00:10Z
All-green with one explained anomaly. Router inbox for claude-home is ZERO; 7 open items all
`to: owner`/`to: user` (doctor's 7 "unsupported wake mechanism: owner-surface" lines are correct,
not a defect). **The gemma broker is DOWN — deliberately, by another lane.** Claude session
`d6bab0ee` is running an inference benchmark whose script opens with
`launchctl bootout gui/501/ai.sirsi.gemma-broker` ("quiet the broker first, it holds the GPU") and
then probes SNE / omlx / mlx_lm in turn; it was still on the third probe (`mlx_lm server` pid 88426,
port 8396) at the end of this pass. That single bootout exactly accounts for the label count moving
27 → 26 since the prior run, so **do not read the missing broker as a crash and do not restart it
into a live GPU benchmark** — the script does not restore it, so the restore is a follow-up:
`launchctl bootstrap gui/501 ~/Library/LaunchAgents/ai.sirsi.gemma-broker.plist` once pid 87220 is
gone. Corrects stale canon: that plist **does exist** (Aug 7 11:02). Broker being down means no
Gemma triage this pass; inbox zero made it moot. Vitals clean — swap 1.94 MB of 1024 MB used, one
JetsamEvent at 22:02Z whose victims are idle-exit system daemons (largest ~240 MB), no sirsi/gemma
process killed. 0 headless `claude --print` sessions. Board HTTP 200. `reconcile` healed the usual 3
codex successors; prune 0 (52→52); retention nothing; 0 BINARY_MISSING; `ccd reap` killed 1 leaked
conduit session (2 procs). PRs unchanged for a third run: FinalWishes #127/#128 both DIRTY with
non-trivial conflicts owned by their lane; pantheon and SirsiNexusApp empty.

## Conduit run 2026-08-09T01:15Z

Broker self-restored — last run's in-flight item (session d6bab0ee's benchmark had booted out
`ai.sirsi.gemma-broker` to free the GPU and never restored it) resolved without intervention: bench
pids 87220/88426 both gone, broker back as pid 18256 on 8477, label count 26 → 27. Ran the driven
leak measurement properly this time (3 real completions, requests 5 → 8): `mlx_active_bytes` and
`mlx_cache_bytes` byte-identical across the window, **0.0000 GB/req** against a known-bad rate of
≈0.48. Real signal, not the idle-broker non-reading the two runs before last produced.

The run's actual finding is a deployment gap. **PR #639 — the #636 dispatch-leak fix that canon
calls the standing priority — merged 2026-08-07T22:30:20Z (de3b143d), but `~/.local/bin/sirsi` is
dated Aug 7 21:39, fifty-one minutes earlier.** All 23 `router wake-loop` processes (pids 436xx)
hold that pre-fix image, and Go binaries do not hot-reload, so neither the progress gate nor the
hourly spawn ceiling is in effect. The fabric looks healthy for the wrong reason: the leak is
dormant, not fixed — every lane inbox is empty, so the loops idle. `wake-claude-nexus` started
23:55Z and logged nothing across ~75 one-minute cycles; headless `claude --print` count is 1. A
naive grep of spawn-ish tokens across `tail -2000` of each log reads as hundreds of events per lane
and is misleading — those are historical restart lines, not current cycles. Deploy would be
schema-safe (origin/main builds ceiling 16, live store `user_version` 16 — equal, not the v15
inversion), but no `BINARY_MISSING` sentinel exists so the step-3 auto-heal is unarmed, and the fix
only takes effect after rolling all 23 wake labels. Too broad to do unattended with no active leak,
so it went to the owner as decision card
`20260809-011319-claude-home-owner-decision-639-merged-22-30z-but-binary-predates-it-by-51min-2`.
Canon in both `~/.claude/CLAUDE.md` and the conduit task file still describes #639 as DIRTY and
awaiting an independent bind; that is stale either way.

Everything else steady. Inbox zero for claude-home; the one new item
(`20260809-010749`, codex-home → claude-nexus, PR #19 stop-token leak) is three minutes old and
claude-nexus is active on `thr-2c96bd5e97673b69`, so it stays with its lane. Seven open items remain
owner/user and structurally non-closable — doctor's seven `unsupported wake mechanism "owner-surface"`
lines are correct behaviour, not failures. `horus.agent-router` and `triage` confirmed absent by
owner decision (plists renamed `.OFF-owner-20260807`) and `gemma-worker` has no plist at all — all
established, none revived. Reconcile healed the usual three codex successors; prune 0 (29 → 29);
retention nothing; `ccd reap` killed one leaked conduit session (pid 6847, idle 61min). Board HTTP
200. PRs unchanged for a fourth run: pantheon 0, SirsiNexusApp 0, FinalWishes #128 (draft) and #127
both DIRTY with non-trivial conflicts belonging to their lane. Vitals clean apart from one delta
worth watching — swap grew from 1.94 MB used to **2081 MB of 3072 MB** with no corresponding RSS hog
(largest resident process is a 1.0 GB Chrome helper); free RAM 69% is the usual hollow metric. No new
crash or Jetsam reports.

## Conduit run 2026-08-09T02:20Z

Machine rebooted 2026-08-08T20:43:43Z (shutdown stall `shutdown_stall_2026-08-08-164330`). Two
minutes later a launchd-spawned `sirsi` (pid 1204) took SIGKILL with
`CODESIGNING / Launch Constraint Violation` — the only `sirsi-*.ips` on this machine, ever. The
installed binary codesigns clean (`codesign -v` exit 0) and `sirsi diagnose` runs, so this was a
boot-window AMFI transient, not the known cp-over-live-binary class. Recorded, not chased.

Chased a swap alarm to a benign root cause and it is worth pinning, because the naive read was a
P0. `sysctl vm.swapusage` showed 7.97 GB of 9.2 GB consumed with the swapfile set having grown
3 GB → 9 GB in ~45 minutes, while `memory_pressure` cheerfully reported 83% free. `vm_stat`
settled it: **4,023 free pages — 64 MB on a 48 GB machine** — with 16.1 GB wired and 15.7 GB in
the compressor. The driver was not a leak: `pgrep -fl 'sirsi-inference generate'` caught a live
SNE compiled-parity A/B benchmark (base vs `SIRSI_KPAD=1`, 3 rounds, three concurrent zsh loops)
re-loading gemma-4-12B-it-8bit per invocation alongside the live broker. It is another lane's
in-flight work, so it was left alone. It finished mid-run and free pages recovered 4,023 →
855,618 (64 MB → 13.1 GB) with no intervention. Swap *used* stayed pinned at 7.97 GB, because
macOS neither shrinks swapfiles nor pages back in speculatively. **Lesson: swap-used is a
high-water mark, not a live pressure gauge; `Pages free` is the live signal, and free-RAM% is
neither.** The Jetsam at 18:02Z fits the same window and claimed only StorageManagementService on
a per-process limit — no sirsi/gemma victim.

Broker measured clean again on a driven window: requests 14→17, active +0.0295 GB, cache
−0.0290 GB, **total +0.0004 GB = 0.0001 GB/req** against a known-bad 0.48. The active-rises-while-
cache-falls shape is the allocator working, not a leak.

PR #639 remains merged-but-undeployed for a second run: `~/.local/bin/sirsi` is still dated
Aug 7 21:39, predating the 22:30:20Z merge, so all 23 wake lanes carry the pre-gate image. The
leak stays dormant rather than fixed — every lane logged `alive, inbox depth 0` all run and
headless `claude --print` count was **0**. The owner card
(`20260809-011319`) is unanswered; not re-filed, per no-nags.

Cleared one residual: `bind-script-silent-wrong-repo` warned MERGED != DEPLOYED for PR #664.
Verified — `~/.local/bin/sirsi-bind.sh` line 37 reads `REPO=""` with the no-default comment at
line 29, so the fix is live locally. Reconcile healed the usual 3 codex successors; prune 0
(29→29); retention nothing; `ccd reap --apply` 0 (the dry-run's candidate aged out cleanly).

## Conduit run 2026-08-09T03:10Z

Inbox zero for claude-home; 9 open fleet-wide (owner 7, user 1, codex-home 1). The codex-home item
is a fresh 03:10:23Z bind request from claude-nexus (sirsi-inference PR #23 plan interphase +
Workstream 3b) — correct reviewer, live successor threads, left for them. **Both self-hosted CI
runners (m5-sirsi, m5-sirsi-2) are back online** (`status=online busy=false`), which moots the
premise of owner decision card `20260808-051202`; the card was NOT closed — owner cards were swept
once before and drew a correction, so the resolution is recorded here rather than acted on. Swap
total shrank 9216M→3072M after the reboot and reads 1983M used, but the live gauge is fine: 400,816
free pages × 16KB = **6.4 GB free**, no `sirsi-inference generate` bench running, no new crash or
Jetsam artifacts since the already-evaluated 18:02Z event. Broker measured over a **driven** 3-request
window (8→11): active +0.0276 GB, cache +0.0176 GB, total **0.0151 GB/req** — 32× clear of the 0.48
known-bad rate, though up from last run's 0.0001, so it is a watch line, not an alarm. `diagnose`
🟡 88/100 on the usual `phys_footprint` memory-hog false positive (SNE 12.6 GB of mmapped weights).
Housekeeping: reconcile healed 3 reaped→successor threads (codex-pantheon ×1, codex-home ×2) and
again flagged the 3 stranded uncommitted files in the primary checkout — still an owner adopt/discard
call, untouched; prune 0 (29→29); `ccd reap --apply` killed 1 leaked completed session (2 procs);
retention nothing. Fabric intact at 37 labels: 24 sirsi + 10 Actions runners + menubar. **#639
remains merged-but-undeployed for a third run** — `~/.local/bin/sirsi` still dated Aug 7 21:39
against a 22:30Z merge — but the leak stays dormant: headless `claude --print` count **0**, zero
spawn lines across all 25 wake logs, every lane at inbox depth 0. Board HTTP 200, 0 BINARY_MISSING.
PRs unchanged a sixth run (FinalWishes #128 draft + #127, both DIRTY, their lane's).

## Conduit run 2026-08-09T06:2xZ

Root-caused a false-positive restart loop in `sirsi liveness-watch run`. The checker gates broker
liveness on `mlx_active_bytes >= 1024 MB`, but the broker loads model weights **lazily on the first
request** — a freshly restarted, idle broker legitimately reports 0. The alert's remediation is a
restart, which reproduces the exact condition, so it re-fires: 06:01:46Z and 06:10:28Z, 8m42s apart
against a 900s StartInterval. The second firing fell back to an RSS floor, unsound for the same
family of reason (weights are mmapped and file-backed; a healthy broker reads ~207 MB RSS). The
alert told the recipient the weights were "likely absent" and to re-download them — the HF cache is
intact at 12 GB with all three safetensors shards, so that remediation would have destroyed a good
cache. The correct probe is already implemented in the same checker and passes on earlier ticks
(`gemma-broker ok answered in 0s (1 tok, finish="stop")`): send a request, count tokens, immune to
both the lazy-init and mmap traps. Routed the evidence to claude-pantheon as
`20260809-061333`, leaving their `20260809-060146` open — theirs to close, theirs to fix.

Two process notes worth carrying. First, the task file's broker identity check is stale: the
process is `sirsi-inference serve`, not `sne-server-macos-arm64`, so `pgrep -fl sne-server`
correctly returns nothing and reads as a dead broker. Second, `launchctl list` from the scheduled
task's shell sees a domain with zero sirsi labels while the services are genuinely running with
PPID 1 — `launchctl print gui/501/<label>` returns "Could not find service" for labels whose
processes are alive. Neither tool is a sound liveness signal here; the process table and a
functional probe are. Between them these two artifacts produced an initial (wrong) reading that the
entire fabric was down. Broker measured clean at 0.0077 GB/req over a driven 3-request window.

## Conduit run 2026-08-09T07:1xZ

claude-home inbox zero. The run's finding: claude-pantheon's fix for the liveness-watch
lazy-init false positive (`22884f06`, plus the `gemma-server.pid` self-heal `16947309`/`89fa617d`)
is **stranded on `origin/fix/broker-quarantine` — 10 commits off main, with zero open PRs on the
repo.** They closed `20260809-060146` at 06:20:25Z as "Fix shipped"; the item re-fired at
06:56:49Z (`occurrences=2`, reason `connection refused`, broker process start 06:56:48Z), and the
preceding tick fired the *other* unsound probe in the same checker (`RSS 3 MB below the 1024 MB
floor` — a healthy broker reads ~207 MB RSS because weights are mmapped). Even a merge would not
reach the running watcher: `ai.sirsi.liveness-watch` runs `~/.local/bin/sirsi`, still **Aug 7
21:39**, predating every one of those commits. Routed `20260809-071245` to claude-pantheon asking
them to open the PR — deliberately not opened by me, since authoring it would disqualify me as
their independent reviewer. Did not self-install; the deploy half is already owner card
`20260809-011319` (unanswered, seventh run) and was not re-filed.

Broker healthy and measured warm, not cold: first driven window was a cold start (0 → 12.686 GB,
weight load, no leak signal); the warm window gave 12.8142 → 12.8437 GB over 3 requests =
**0.0098 GB/req** against a known-bad 0.48. Vitals: swap 2787/4096 MB, 1.33M pages free, health
94/100 (benign "4 Sirsi processes, 1.6 GB"). One `sirsi` crash `.ips` from Aug 8 20:45Z in the
`wake.claude-finalwishes` coalition — `SIGKILL (Code Signature Invalid)`, `CODESIGNING / Launch
Constraint Violation`, uptime 86s: the cp-over-live-binary AMFI class, single occurrence, binary
untouched since Aug 7, so reported not chased. JetsamEvent Aug 8 18:02Z victims were all small
system daemons (rpages 73-190), no sirsi/gemma process — not P0. Housekeeping: reconcile healed the
same 3 successors again, stranded-file warning steady at 5 (owner call, not committed), prune 0
(52→52), `ccd reap` killed 1 leaked session, retention 0, board HTTP 200, 0 BINARY_MISSING. PRs
unchanged a tenth run: pantheon 0, NexusApp 0, FinalWishes #127/#128 both DIRTY and their lane's.
Doctor now flags ~38 stale ledger rows on claude-home itself — carried as next run's first candidate.

## Conduit run 2026-08-09T08:2xZ

Closed the loop I opened last run. PR **#674** existed this time (`fix/broker-quarantine`, 10 commits,
opened 07:13:38Z — a minute after my route `20260809-071245`), every required context green, only
`binding-hold` failing for want of a signature. I am independent of it: the code is claude-pantheon's,
I authored only the diagnosis. Source-deep reviewed the non-test delta — the weight floor now runs
**after** the functional generation probe and merely upgrades an already-failing wedge into the
specific "weights likely absent" diagnosis, which is the correct shape because weights load lazily on
first request and floor-first turned a legitimately idle broker into a restart, which produced another
idle broker on the next tick. The test inversion is real rather than cosmetic:
`TestProbeGemmaState_IdleBrokerIsNeverMisreadAsWeightless` fails if the ordering regresses, and the two
floor tests now serve a zero-token wedged response instead of asserting the probe is never called —
the old assertion pinned the defect in place. Also checked `pidsync.go` (best-effort launchctl parse,
5s timeout, never liveness truth), the `SendGuarded` whole-transaction retry (re-run-safe: nothing
commits on a failing attempt and the idempotency key makes an ambiguous retry dedup rather than
duplicate; `notifyWaiters` correctly moved outside the closure), and the menubar quarantine suppression
(fires only on process-absent AND plist-absent, so it cannot mask a crashed-but-installed menubar).
Bound at `57a985ee`, merged.

**And it is still inert.** `20260809-071149` shows `occurrences=4, last_seen=07:55:14Z` — the same
false-wedge loop firing a fourth time, 43 minutes after the previous instance was closed, because
`ai.sirsi.liveness-watch` execs `~/.local/bin/sirsi` and that artifact is dated **Aug 7 21:39**. It now
lags two merged fixes (#639 and #674). I did not install: deploying main also carries #639's fabric
spawn-behaviour change while owner card `20260809-011319` has been open and unanswered on exactly that
question since 01:13Z, and I will not walk an owner-gated change in on the back of a defect fix. Routed
the deploy with the ordering (schema check — live store v16, main builds 16, equal and safe; `rm -f`
before `cp`; re-sign; restart the long-lived Go processes) to claude-pantheon as `20260809-081421`, and
opened `merged-not-deployed-recurs` asking for a doctor check that compares the installed binary's
revision against origin/main so a merged fix cannot sit inert for days with no surface saying so.

Broker measured clean and **not bounced**: cold at entry (`mlx_active=0, requests=0` — lazy init, the
exact state #674 stops misreading), warm **0.0098 GB/req** over a driven 3-request window
(active+cache 12.8031 → 12.8326 GB, requests 2 → 5) against a known-bad 0.48. Worked last run's flagged
stale-ledger candidate rather than deferring it again: two rows were parked on PRs that had already
merged — `broker-label-misnames-sne` on #661 (merged Aug 7 23:02, held 33h after its bind landed) and
`launchctl-enable-passes-five-labels-as-one` on #613 (merged Aug 7 23:34, parked on an Actions outage
that ended). Both re-resolved with `gh pr view` rather than from memory, leased and completed with
evidence; the #613 follow-ons stay open as their own rows. Housekeeping: reconcile healed the same 3
successors, prune 0 (121→121), `ccd reap` killed 1 leaked session, retention nothing, board HTTP 200,
0 `BINARY_MISSING`, headless count 1 with no climb. `sirsi diagnose` 🟢 100/100, swap 2715/4096 MB, no
new sirsi/gemma crash or Jetsam since the Aug 8 16:45 AMFI kill already evaluated. Note for the next
run: `ai.sirsi.horus.agent-router` and `ai.sirsi.triage` are **not loaded** — their plists carry the
`.OFF-owner-20260807` suffix, which is an owner decision, not a defect to heal.

## Conduit run 2026-08-09T09:1xZ

Resumed from the prior run's flagged in-flight item and found it already answered: claude-pantheon
responded to `20260809-081421` at 08:17:37Z, independently re-verified that #674 (f26d51d0, merged
08:11:20Z) and #639 (de3b143d) are both on origin/main while `~/.local/bin/sirsi` is still dated
Aug 7 21:39, and then declined to deploy — because rebuilding from main also ships #639's fabric
spawn-behaviour change, which would silently execute option B of owner card `20260809-011319` while
that card is open. That is the correct stand-down and the deploy stays owner-gated; I did not
re-file it. Worth recording that the exposure is widening, not static: the false broker-wedge
liveness loop fired again at 08:50:39Z (`20260809-081404`, occurrences=4) and at 09:05:39Z, seven-plus
firings after its own fix merged, because `ai.sirsi.liveness-watch` execs the stale artifact. I
rewrote the `merged-not-deployed-recurs` charter to carry that state so the row stops understating
itself, and left its non-gated half — a doctor check comparing installed build revision against
origin/main HEAD — open. Separately, the `sne-broker-active-crossed-38gb-threshold` row is stale by
process identity, not by judgement: those 38 GB samples belong to a broker that no longer exists.
The live one is revision `7a0cdf7b` built 07:35:28Z, `mlx_active_bytes` 11.79 GiB, peak 11.82 GiB, and
a driven three-request window normalized per request measured **0.0166 GB/req** against a known-bad
0.48. Swap is 1988/3072 MB, not the 93.5%-consumed state the row records. I routed that measurement
to claude-nexus as `20260809-091146` rather than closing the row myself, since the row is theirs and
their lane is very much alive. Everything else was clean: diagnose 94/100 (the single finding is the
`phys_footprint` Sirsi-processes badge, the known false-critical), 0 BINARY_MISSING, board HTTP 200,
retention nothing to prune, `ccd reap` killed 0 and archived 1, reconcile healed the same three
successors and left the same 5 stranded files (owner call, never auto-committed), and headless
sessions read 2 with no climb. `ai.sirsi.horus.agent-router` and `ai.sirsi.triage` remain unloaded
behind `.OFF-owner-20260807` plists — owner decision, never bootstrapped. FinalWishes #128 and #127
are both still DIRTY for the twelfth consecutive run; codex-home has that routed to codex-finalwishes
(`20260809-052520`) and the conflict is theirs, not mine to resolve.

## Conduit run 2026-08-09T10:1xZ

The machine rebooted at 09:13Z (shutdown_stall 09:13:32Z), so every verification from the 09:4xZ
decision-board run pre-dates the current process table. The fabric came back on its own and came
back correct: 23 wake lanes, gemma-broker, router-board (HTTP 200) and 10 self-hosted runners all
live; horus/triage still absent behind their `.OFF-owner-20260807` plists; `ai.sirsi.pantheon` still
out after claude-nexus's bootout. Headless `claude --print` sessions: 1, no climb. One real signal
to report, not heal: a `sirsi` crash at 09:14:34Z, one minute into boot, `SIGKILL (Code Signature
Invalid)` / `CODESIGNING Launch Constraint Violation` — single occurrence, and the installed binary
(Aug 7 21:39, sha256 `1470a895d995aa37`) passes `codesign -v` with every router verb working, so it
reads as a lane exec'ing the binary mid-replace rather than a bad artifact. The broker re-measure is
the run's methodology note: on a cold post-reboot process the first driven 3-request window read
**3.98 GB/req**, which is weight load (active 0 → 11.81 GiB on request one), not a leak — a run that
stopped there would have filed a false P0 twenty times worse than the known-bad rate. The warm
window (requests 3→6) reads **0.0092 GB/req** on active+cache against known-bad 0.48. Clean.
**A cold broker cannot be leak-measured any more than an idle one can; load the weights first, then
measure.** Inbox worked to zero: claude-nexus's decisions-of-record item was ACK-closed with a
response, and it surfaced genuinely stale canon — the conduit's own task file still called PR #639
"DIRTY and blocked on an independent bind" and told each run that landing it was the priority.
Rewritten to record the merge (de3b143d, 2026-08-07T22:30Z) and the owner's option C: deliberately
undeployed, watch-and-report only. That was the copy actually driving the hourly pass, so the fix
matters more than the CLAUDE.md one already made. FinalWishes #127/#128 both still DIRTY, thirteenth
unchanged run, already routed to codex-finalwishes — left. reconcile healed the same 3 successors;
5 stranded files remain an owner call; prune 0 (52→52); ccd reap 0/0; retention nothing.

## Conduit run 2026-08-09T11:13Z

claude-home inbox zero; 8 open fleet-wide (owner 7, user 1), all owner-surface decision cards left
untouched — sweeping them is itself the defect two of those cards report. One new finding routed:
PR #674 (merged f26d51d0, 08:11Z) shipped `suppressMenubarDown`, but it only suppresses when the
LaunchAgent plist is ABSENT, and `~/Library/LaunchAgents/ai.sirsi.pantheon.plist` is still present
(Jul 2, 593 B) — this machine's menubar quarantine is launchd-state, not plist removal, so the
suppression never fires and `liveness-watch: menubar not running` is a permanent standing alarm
(item 20260809-101453 at occurrences=4, last_seen 10:59:57Z). Verified from source, not inferred
from the alarm: `Run` is read-and-route only and `ai.sirsi.pantheon` is still absent from
`launchctl list`, so the owner's quarantine is intact — this is alarm cost, not a reversed
decision. Routed to claude-pantheon as 20260809-111335 with two fix options, recommending they
widen the suppression to treat "plist present but job not loaded" as quarantine rather than rename
the owner's quarantine artifact. No second owner card opened: 20260808-221547 already covers the
owner-facing half. The fix cannot be deployed regardless — rebuilding main to ship it would also
ship #639, which the owner deliberately left undeployed. Broker measured clean on a WARM driven
window: 0.0138 GB/req over 3 requests (known-bad 0.48), peak 11.97 GiB, revision 7a0cdf7b, port
8477. Vitals 94/100, swap 0/0, no new crash/Jetsam reports, 0 headless sessions, quarantine and the
parked horus/triage labels all intact.

## Conduit run 2026-08-09T15:20Z

All-green pass with one diagnosis worth recording. `main` CI run `31302915663` (head `f26d51d0`,
PR #674) reads **failure** at both the run AND job level, but it is a **false red**: steps 1-9 all
succeeded, including `Run tests`, and only step 10 `Upload metrics` was `cancelled` when the
self-hosted runner took a shutdown signal mid-upload. A cancellation in a trailing artifact step
poisons the job conclusion, so the standing "read the JOB, not the run" rule is insufficient here —
read the STEP conclusions. Main is green in substance; no regression from #674, nothing routed.
PR #675 (`fix/broker-quarantine`, claude-pantheon) is CONFLICTING with source-level conflicts in
`internal/liveness/livenesswatch.go` and its test, and carries **zero checks** — that is a direct
consequence of the conflict, not a CI fault: `ci.yml` fires on `push` to main/develop and on
`pull_request`, and GitHub cannot build a merge ref for a conflicting PR, so no `pull_request` run
is ever dispatched. Left for its lane agent to rebase. Broker measured **0.0138 GB/req** over a
driven 3-request window (req 17→20, active+cache), peak 12.07 GiB — clean against the 0.48 GB/req
known-bad. Registry: reconcile healed the same 3 successors again, prune 0 (29→29), ccd reap killed
1 completed conduit session, retention nothing. Headless sessions 0.

## 2026-08-09T13:0xZ — PR #675 rebased clean; menubar alarm is real but correctly not relaunched

horus's `liveness-watch: menubar not running` (20260809-130002) named a genuine gap — no
`SirsiMenubar` process — but the machine is deliberately owner-quarantined
(`~/.sirsi/menubar-quarantine`, written 09:43:24Z). Did not relaunch it; a quarantined component
alarming as "down" is the alarm working as designed until the quarantine-aware fix ships, not a
reversed owner decision.

That fix is exactly PR #675, which the 15:20Z conduit run had already diagnosed as CONFLICTING
and left for its lane agent — me. Rebased onto `origin/main`: 14 commits collapsed to 6 real ones,
because `git rebase` cleanly dropped 4 whose patch content main already carries via #674 (broker
quarantine, weight-floor false-positive, gemma-server.pid self-heal) with the message "patch
contents already upstream" — no manual resolution needed. The one genuine code conflict was
`cmd/sirsi/gemma_serve.go`: an old commit added a local `syncGemmaPidFile` duplicating what main
already ships as the shared `liveness.SyncGemmaPidFile` (identical body, confirmed by diff);
resolved in favor of main's shared version and dropped the duplicate. `.thoth/journal.md` and
`.thoth/memory.yaml` conflicted at nearly every step (expected — both branches append linearly);
resolved by taking main's copy each time and popping the pre-rebase local edits back on top after,
clean, no markers.

Verified before push: `go build ./...` clean, `go vet ./...` clean, `golangci-lint run ./...` 0
issues, `go test ./internal/liveness/... ./internal/router/... ./internal/routerstore/...` all
pass. Full `go test ./cmd/sirsi/...` hit `TestAnubisWeigh`'s hardcoded 60s timeout (a full-
filesystem scan integration test, unrelated to this diff — confirmed unrelated by re-running it in
isolation and by reading the test body); `-short` mode (skips that test) passes clean. Pushed with
`--force-with-lease`; PR flipped CONFLICTING→MERGEABLE, CI (lint/test/build/secrets) all green.

Did not self-bind. The PR touches `cmd/sirsi/` (an authority-model path per `binding-hold.yml`),
which requires an APPROVING REVIEW from an identity other than the author on the current head SHA
(A25/A28/ADR-041) — and every agent here authenticates as the same `SirsiMaster` account, so an
agent cannot legitimately review its own PR by running `scripts/bind/sirsi-bind.sh` on itself; that
would be exactly the rubber-stamp A34 exists to prevent. Sent a full review request to codex-pantheon
(`20260809-131022`) with the diff summary, rebase notes, and verification evidence, asking them to
review and either approve directly or run the bind script. Closed the horus item with the full
root-cause/fix-status/no-action-taken record rather than a bare ack. Until #675 merges, the
currently deployed binary predates the quarantine marker check and this alarm will keep firing on
its 900s interval — tracked as alarm cost, not restated as a new finding each time it fires.

## Conduit run 2026-08-09T13:20Z

Reviewed, bound and merged pantheon **PR #675** `fix/broker-quarantine` (claude-pantheon) as
commit `92ad9808`. It had changed materially since the previous pass: the source-level conflicts in
`internal/liveness/livenesswatch.go` were resolved, so GitHub could build a merge ref and CI
dispatched for the first time — all four content contexts came back SUCCESS, leaving `binding-hold`
as the only outstanding required check. Gate was evaluated as the set difference against branch
protection's five required contexts (Lint, Test, binding-hold, Secrets Scan (gitleaks), Build
(self-hosted, macOS, ARM64, 1.25)), never `mergeStateStatus`. The change is content-correct and
closes a class this runbook has tripped over four separate times: plist presence cannot distinguish
"the owner quarantined this" from "this died", because `launchctl bootout` leaves the exact-named
plist in place. #675 replaces that heuristic with an explicit marker file
(`~/.sirsi/menubar-quarantine`, set/cleared by `sirsi menubar quarantine|unquarantine`) checked in
`probeMenubar` ahead of `suppressMenubarDown`, and gives `runConduitStatus` the same upgrade —
three distinct states (NOT ARMED / INCOMPLETE / ARMED via `launchctl print`) instead of collapsing
plist-exists into "armed". Bind carried one non-blocking nit: `SetMenubarQuarantinedFn` and its
RWMutex-guarded function pointer are an injection seam nothing calls, since the new test uses a
real `t.Setenv("HOME")` plus a real marker file. **The bind explicitly does not authorize
rebuilding or installing `~/.local/bin/sirsi`** — the owner left PR #639 deliberately UNDEPLOYED
(option C, 2026-08-09) and a rebuild of main would ship it; canon-ahead-of-binary is the safe
direction. Separately, cleared two diagnostic-report scares as non-events: the
`JetsamEvent-2026-08-09-061417` names `sirsi-inference` as `largestProcess`, but the report is
`bug_type 298` with zero processes carrying a `killed` flag — a memory-status snapshot, not a kill
— and that process is PID 1157, i.e. `ai.sirsi.gemma-broker` itself, whose 13.02 GB is mostly
file-backed mmapped weights (only 322,024 internal pages, ~5.3 GB). The `sirsi-2026-08-09-051434.ips`
is a `SIGKILL (Code Signature Invalid) / Launch Constraint Violation` at boot, the known
cp-over-live-binary signature; the installed binary is untouched since Aug 7 21:39 and
`sirsi router status` exits 0, so it is not recurring.

## Conduit run 2026-08-09T14:15Z
Cleared the run's only in-flight item: both main CI runs on `92ad9808` (PR #675, menubar
quarantine) completed **success** — no false-red, no step-level reading needed. Vitals are the
cleanest in days: swap 0.00M/0.00M, 92% RAM free, **zero** headless `claude --print` sessions
(the #636 spawn exposure is quiescent, still deliberately undeployed per the owner's option C).
diagnose 94/100, the lone finding the known `phys_footprint` false-critical. Broker measured on a
DRIVEN 3-request window: **0.0138 GB/req** active+cache against known-bad 0.48, third consecutive
run at that exact rate; peak 13.17 GB under the 20 GiB backpressure limit — not bounced. Reaped one
leaked completed session (pid 82702, a prior run of this same task, argv-verified before kill).
Substantive finding: **FinalWishes #127 is not a stalled PR, it is a dead one** — codex-finalwishes'
own verified router response (05:30Z) records it as superseded by #129, merged 2026-08-07T20:08Z,
with no implementation work remaining. Seventeen runs of "unchanged DIRTY" were a PR nobody had
closed, not a blocked merge. Left open: it belongs to the finalwishes lane and closing another
lane's PR is outside this pass. Also verified **both self-hosted runners are online** (`m5-sirsi`,
`m5-sirsi-2`), which factually resolves owner card `20260808-051202`; not closed (to: owner), and no
new card opened — the owner already holds 7 and a stale PR nags nobody.

## Conduit run 2026-08-13T22:13Z

Found the entire agent fabric down and reported it rather than restarting it. `launchctl list | grep
sirsi` returns zero rows and `router doctor` reads 28 agents registered / **0 live**: broker (:8477,
no process), board (:8734, HTTP 000), all 23 wake lanes, triage, gemma-worker, and all 10 self-hosted
Actions runners. Stop time is 2026-08-09 12:44 EDT — every `wake-*.log` ends there; the two files
showing a fresh mtime were touched by my own `sirsi` commands this run, so content, not mtime, is the
honest signal. It has not survived three reboots since (Aug 11 x2, Aug 13 17:12). Plists are all
present under live names and the launchd override DB reads `enabled` for all 54 sirsi labels, so this
is a never-bootstrapped-at-login condition, not a disable. I did not restore it: bringing back 23
wake lanes unattended would resurrect the #636 dispatch leak (~59M tokens/day) with #639 deliberately
undeployed, and I cannot tell a defect from an owner decision from here — the owner was on this box
all afternoon running the M5 native-observer/parity probe campaign, which a quiet GPU suits. Routed
ONE owner decision card (`20260813-221332`) with three options and a recommendation to half-restore
(broker + board + CI runners, lanes stay down). Knock-on now visible: CI is dead and mislabels itself
as failure — SirsiNexusApp #280's content checks "fail" at exactly `24h0m0s`, which is a runner
timeout, so I left it unmerged; same root cause as open card `20260808-051202`, no second card. Today's
`native-observer`/`sirsi-parity-bin` crashes are dyld `@rpath/libmlx.dylib` launch failures from the
owner's own dev builds, not service crashes — same experiment-harness class as the settled M5 SIGILL
note, not routed. Inbox zero for claude-home; the 2 new codex-home items are fresh (today) and belong
to an owner-offline lane. Vitals otherwise clean: 94/100, swap 0.00M, headless 0, 0 BINARY_MISSING,
reconcile no dirty exits, retention nothing to prune, ccd reap archived 1 (this task's prior run).

## Conduit run 2026-08-13T23:09Z
Fabric still fully down (`launchctl list | grep sirsi` = 0 rows; board :8734 = 000; broker :8477 no
listener) — unchanged since 2026-08-09 12:44 EDT. Owner card `20260813-221332` is open and
**unanswered** (age ~1h), so nothing was restored; that remains the single gate. Vitals green: 91%
free RAM, **swap 0.00M/0.00M**, 0 headless sessions, diagnose 94/100 (its "1 Sirsi process using
1.3 GB" priority has no live process behind it — `ps` shows zero sirsi processes). No new crash
reports, 0 BINARY_MISSING sentinels, reconcile found no dirty exits, thread prune 0 (1→1), router
prune nothing, `ccd reap --apply` killed 1 completed session (this task's own prior run).
claude-home inbox is **zero**. sirsi-pantheon has **zero open PRs**.
**Corrected a wrong carry-forward:** the prior state file described "37 stalled ledger rows" needing
a status-vs-body reconcile. Measured from `ledger --json`: 171 rows — 133 done, 32 **pending**, 5
in-progress, 1 blocked. The 32 pending are genuine never-started backlog findings, not
terminal-in-fact rows; a status reconcile over them would have been a wipe, not a repair. Three rows
*are* terminal by evidence — `lifecycle-fence-lost` and `fence-retry-budget-underprovisioned`
(PR #671 MERGED 2026-08-08T01:20:22Z) and `worktree-cp-clobbers-stale-file` (PR #668 MERGED
2026-08-08T00:53:14Z), all merge-verified via `gh pr view`. They were **not** transitioned: the store
correctly fail-closed (`executable task transition requires a fenced task lease`), and `task claim-id`
requires `--worker` and `--thread` while the only thread record on the box is `claude-nexus`
(suspended). Deferred rather than falsify lane identity in the registry.

## Conduit run 2026-08-14T00:10Z
Cleared the bounded work the 2026-08-13T23:20Z run deferred: the three claude-home ledger rows that
were terminal by merge evidence but stuck because the store fail-closes on unfenced transitions.
Registered a claude-home thread (`thread register --agent claude-home --surface conduit
--anchor-pid $$` — the bare form fails with "no durable conduit runtime found within 16 ancestors"),
leased each row via `router task claim-id … --worker claude-home --thread <tid>`, and completed them
with `task complete --lease <L> --result-ref`. Re-verified the evidence first: PR #671 MERGED
2026-08-08T01:20:22Z (917354a2) closes `lifecycle-fence-lost` and
`fence-retry-budget-underprovisioned`; PR #668 MERGED 2026-08-08T00:53:14Z (27d5321d) closes
`worktree-cp-clobbers-stale-file`. Ledger verified 133→136 done, 171 rows unchanged. Fabric remains
fully down (0 sirsi labels, board :8734 = 000, :8477 no listener) and owner decision card
`20260813-221332` is still unanswered at age ~2h — nothing restored, no second card opened. Vitals
clean: swap 0.00M, RAM 88% free, 0 headless sessions, 0 BINARY_MISSING (heal stays forbidden while
#639 is deliberately undeployed). Jetsam event 2026-08-13-173418 inspected: no sirsi/gemma/sne or
Python victim, largest process WindowServer at 0.91 GB — not a P0. `ccd reap --apply` killed one
leaked prior run of this task; router prune found nothing.

## Conduit run 2026-08-14T01:20Z

Fabric still fully down (0 of 28 lanes, board :8734 = 000, no listener on 8477) — unchanged since
2026-08-09 12:44 EDT. Owner decision card `20260813-221332` remains open and unanswered at age ~3h,
so nothing was restored and no second card was opened. Inbox zero for claude-home; zero open
sirsi-pantheon PRs.

One new defect found and routed. Nine `native-observer` launch crashes (2026-08-13 17:21→20:37 EDT)
are DYLD `Symbol missing` aborts on `mlx::core::sirsi_gate_up_down_qmv` — a Sirsi fused-kernel
symbol, despite the crash reports being attributed to the `com.openai.codex` coalition. Verified the
provider side simply does not exist on this machine: all 11 `libmlx.dylib` on disk export the symbol
zero times, including both venvs named `-patched`. In `sirsi-native-rebuild/native/`, only
`build-fused/` carries the undefined reference; `build-release/` and `build/` are clean. My first
hypothesis — a stale staged artifact — was wrong and is corrected in the routed item: the newest
staged dylib (`/private/tmp/sirsi-locked-native-nocore-build/`, 20:39:22) postdates the last crash
(20:37:21), so fused-linked dylibs are being actively rebuilt against a kernel nothing provides. The
crashes stopped because the binary stopped being run, not because it was fixed. Routed to
claude-nexus as `20260814-011828` with the `nm` evidence and a suggested stage-time regression gate;
changed nothing in that repo, as it is the inference lane's call.

Housekeeping: `thread reconcile` healed one claude-home thread to a successor and re-flagged the
same 3 uncommitted files on branch `fix/broker-quarantine` (not mine to commit); `thread prune` 0
(2→2); `ccd reap --apply` killed 1 leaked prior run of this task; `router prune` nothing to reclaim.
`diagnose` 94/100 🟡 — its lone priority ("1 Sirsi process using 1.3 GB") is again a phantom, `ps`
shows 0 sirsi processes. RAM 90% free, swap 0.00M/0.00M, 0 headless sessions, 0 BINARY_MISSING
sentinels (the binary heal stays FORBIDDEN — rebuilding main would ship #639, which the owner chose
to leave undeployed). Broker leak remains UNMEASURABLE: a down broker gives no signal in either
direction.

## 2026-08-15 - Pantheon SNE supervision foundation

Added the product-neutral SNE Runtime API client, strict Anubis and Ra profiles, and a shell-free supervisor plus CLI. Pantheon passes explicit executable/model/tokenizer/assistant/runtime identity, waits for fail-closed readiness, terminates gracefully, and restarts unexpected child exits. Resource-profile fields are canon but remain pre-admission until concurrency, memory-ceiling, and foreground-yield enforcement are demonstrated.

## 2026-08-15 - SNE resource governance added

- Pantheon SNE supervision now carries explicit request-concurrency and RSS-ceiling controls to the service.
- An over-ceiling child is terminated for governed restart and reprime; lifecycle behavior remains unadmitted until the fault-injection matrix is run.

## 2026-08-16 - Pantheon owns SNE supervised restart

Two real in-process native MLX close/reopen attempts had ended with empty HTTP
replies. SNE now returns structured HTTP 409 `restart_required` without touching
engine state. Added typed Pantheon client errors, serialized supervisor
stop/start with preservation of the original parent context, readiness wait,
and `ReloadModel` delegation. `sirsi-sne-supervisor` maps SIGHUP through that
same service contract rather than signaling the child directly.

Focused tests pass, including a helper-process proof that restart produces a
new PID. The real Gemma 4 12B Metal gate replaced child PID 39938 with 40085,
kept service SHA `458b4052...`, restored readiness, and preserved exact content
SHA `770fe7e4...`. This admits the architecture for continued integration, not
the product: Nexus workflow, durability100, package/sign/notarize, clean-Mac,
security, and pilot gates remain.

## 2026-08-16 - Nexus consumed Pantheon-supervised SNE

The real Nexus -> Pantheon -> SNE gate selected exact provider/model identity,
streamed native SSE, replaced child PID 40979 with 41121 through the supervised
restart contract, restored readiness, and preserved exact content SHA
`770fe7e4...`. Nexus never gained process or GPU ownership. This closes the
governed integration seam, while repeated durability and productization remain.
# 2026-08-16 — SNE process-scoped lifecycle

- In-process native MLX close/reopen proved intermittent and was removed.
- Added typed load/unload/reload client calls and Pantheon supervisor ownership.
- Real candidate6 gate used three distinct PIDs and exact official output.
- This supersedes the narrower restart-only lifecycle record; no performance or
  GA claim changed.

## 2026-08-16 — Candidate7 SNE lifecycle refresh

Pantheon's real process-scoped SNE lifecycle test was rerun against packaged candidate7 after SNE adopted the official thinking-disabled Gemma 4 prompt contract. Initial, load-after-unload, and reload operations used distinct PIDs 52111, 52118, and 52125, and every process returned the exact final-answer content SHA `fda564ba3f7a0f028106d468420f674898ed99ac5bf2765ac9586206e39d73c5`. Focused tests and vet passed; the first sandboxed unit attempt failed only because `httptest` could not bind loopback, then passed with host permission. Existing canonical, owner, Workspace, and evidence records were refreshed in place to avoid duplicate current-state documents.

## 2026-08-16 — Exact Gemma 4 tuple admission

Pantheon now validates a selected SNE catalog tuple before creating the child
process. The strict registry covers all sixteen executable Gemma 4 manifests
and binds manifest bytes, model/adapter/precision identity, declared memory,
qualification, checkpoint, and artifact set. Research requires opt-in; profile
memory ceilings, semantic duplicates, unknown IDs, and drift fail closed.
Focused tests and vet passed at the host loopback boundary, and every real
catalog manifest passed the check-only CLI gate. The feature does not select or
substitute inference frameworks and does not change runtime performance.

## 2026-08-16 - Durable Gemma 4 registry and E2B supervised restart

- Materialized the 16-entry `pantheon.sne-model-admission.v1` registry under
  `configs/supervisor` from SNE's exact manifest catalog.
- Registry SHA-256: `35b9c79acb949c60c738f4d7119cf599d6178d1d3789adc16269553cef00efeb`.
- Current supervisor admitted `e2b-nvfp4`, replaced child PID 61882 with 61917,
  restored readiness, and preserved exact `Hello.` content.
- Dynamic framework fallback remains prohibited; external product gates remain.
## 2026-08-16 - E4B NVFP4 becomes lifecycle-ready, not lifecycle-proven

SNE repaired the E4B NVFP4 service renderer on a copied candidate, preserved
exact token and terminal-logit hashes, passed bounded quality 8/8, and kept
streaming fail-closed at HTTP 503. Pantheon's durable 16-entry registry already
admits the exact `e4b-nvfp4` tuple, but the E2B package/restart proof does not
transfer: E4B relocatable packaging and a real supervisor-owned fresh-PID
lifecycle are still required. Dynamic framework fallback remains prohibited.
## 2026-08-16 - E4B and 12B exact lifecycle admission

Restored `artifacts/productization/bin/sirsi-sne-supervisor` from stale
`dc074b...` to the byte-identical current-source build `be91216e...`. The stale
binary omitted current model-registry and explicit dylib identity flags. The
SNE lifecycle harness also waited for a fresh child and HTTP readiness but not
for Pantheon's asynchronous success record; cleanup could terminate the
supervisor and create a false cancellation log. The bounded wait now requires
all three signals.

Real E4B NVFP4 supervision replaced child `74719 -> 74768`; real 12B NVFP4
supervision replaced child `82742 -> 82808`. Both preserved exact canary output
after restart. The registry now binds the qualified 12B copied manifest and has
SHA `84e1ac06...`. These are lifecycle/admission proofs, not performance or
physical-bandwidth claims.

## 2026-08-16 - E2B MXFP8 Pantheon lifecycle

Packaged E2B MXFP8 passed exact catalog admission, supervised restart, distinct child PID, post-restart readiness, and content equality under the repaired durable supervisor. Admission registry already matched strict manifest SHA a577baf680..., checkpoint SHA e370cd2f7e..., and artifact-set SHA 9886764736....

## 2026-08-16 - Future Pantheon installer experience requirement

The owner directed that Pantheon eventually ship installation quality comparable
to Tailscale, with Homebrew, GitHub Release, and Mac App Store paths wherever
Apple policy and required capabilities allow. The requirement extends the
existing signed/notarized release path rather than authorizing implementation
now. Canonical acceptance criteria were added to
`docs/PANTHEON-FEATURE-ROADMAP.md` section 5.5: preflight prerequisite detection,
consentful and explained permission/network-extension handling, actionable and
resumable recovery, installed-state verification, state-preserving upgrades,
clean uninstall, and independent clean-machine proof for every offered route.
This prevents packaging mechanics from being mistaken for a complete installer
product surface and preserves visible capability differences where App Store
policy prevents exact parity.

## 2026-08-17 - SNE acquisition and direct UI correction

Completed the owner-authorized readiness and acquisition correction set. The
real readiness overlay now passes strict decoding with typed global metadata;
the supervisor no longer mislabels stability as the registry date. Source
identity rejects revision/repository URL injection, keeps Hugging Face on its
controlled endpoint, and permits signed-catalog HTTPS Sirsi derivative origins
without broad cross-host redirects. Direct `/sne` now selects the SNE catalog
view, and empty hardware identity errors no longer wrap nil. Focused dashboard,
readiness, source acquisition, and command tests pass. Install/start UI actions
remain the next product phase and must invoke the verified transactional
acquisition/checkout/supervision chain rather than a second downloader.

## 2026-08-17 - 31B MXFP8 performance rejection finalized

SNE completed the required true post-reboot fresh100 rerun after repairing a
missing package-relative metallib link on a copied package. Exactness and
fresh-process stability passed 100/100, but encrypted swap grew from zero to
1174.38 MiB. Pantheon's readiness overlay now rejects this tuple under the
performance policy and no longer describes another rerun as pending.

## Entry 045 — 2026-08-17 00:35 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- 2026-08-17 SNE product integration: added transactional lifecycle UI endpoints and Start/Stop actions, formal runtime package resolver, strict checkout receipt provenance, authoritative service_url, and runtime catalog schema pantheon.sne-runtime-packages.v2. Before launch Pantheon now pins/verifies sned, model-specific native runtime, libmlx.dylib, mlx.metallib, and libjaccl.dylib plus admitted manifest and checkpoint receipt. Five formal tuples appear installed/startable: 26B-A4B Q4; 12B Q4/Q5/Q6/plain-Q8. Real Pantheon generations passed for 26B, 12B Q4, and 12B Q5 with clean Stop. Tests passed outside sandbox; sandbox-only listener failures were environmental. Next: MTP assistant checkout/lifecycle, 31B/E2B/E4B package entries, signing/notarized installer UX, clean-host and M1 pilots.
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-11T04:28:50Z
- last Claude read: 2026-06-16T15:30:16Z
- pending: none

---

## Entry 046 — 2026-08-17 00:51 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- 2026-08-17: Pantheon SNE lifecycle now supports exact assistant-bound MTP packages. resolveLaunch derives the sole assistant artifact from the admitted manifest and formal checkout, verifies SHA identity, and passes AssistantSafetensors to the supervisor. Plain manifests reject assistants. Runtime catalog v2 now includes formal candidate9 and pins sned, native runtime, MLX dylib, metallib, and JACCL. Unit tests pass outside the sandbox; dashboard candidate b390b005 passed a real Start/serve/Stop using only formal package/model-store roots and returned exact pantheon mtp ready content. SNE formal gate passed but swap contamination prohibits a new performance claim.
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-11T04:28:50Z
- last Claude read: 2026-06-16T15:30:16Z
- pending: none

---

## 2026-08-17 - Pantheon SNE resource caretaker

- Added one shared pre-launch resource gate to the SNE supervisor; CLI and dashboard launch paths inherit it.
- Gate reads live available RAM, swap readability/usage, memory-death state, kernel/bootstrap pressure, and the manifest-bound measured footprint before every start/restart/load.
- Fixed agent-RSS double accounting: live available RAM already excludes resident agents; the SNE reserve retains OS baseline plus node-scaled safety margin.
- Installed immutable SNE v2 shared-wide package under a formal Application Support root and repointed the exact 12B affine-8 MTP catalog tuple with rollback catalog copies preserved.
- Real supervised launch passed at required=16 GiB, available=28.94 GB, reserve=10.20 GB, swap<1 MiB, normal pressure; exact response SHA 271973d... and clean stop.
- Read-only expansion preflight rejected 26B-A4B (20 GiB declaration) and 31B (32 GiB declaration) for insufficient current headroom. Do not lower those declarations until physical-footprint evidence exists.

## 2026-08-18 - SNE package-boundary admission hardening

The prior reboot-only Metal failure demonstrated that artifact hashes do not
prove package durability: a correctly hashed executable can still depend on a
temporary build location, escaping symlink, or uncontrolled Mach-O rpath.
Pantheon now runs `VerifyRuntimePackageBoundary` both when calculating model
availability and immediately before launch. The gate rejects all package
symlinks, verifies the governed runtime hashes, parses Mach-O imports and
`LC_RPATH`, and allows only explicit package-bound libraries plus Apple system
libraries under `/System/Library` and `/usr/lib`. Absolute `/private/tmp` or
developer-build dependencies, unknown `@rpath` names, and relative escapes fail
before process creation. Focused tests passed for `internal/sne` and
`internal/dashboard`. The governing SNE failure-prevention rules now encode
this incident as rules 19-21.

## 2026-08-18 - Exact readiness identity and real v25 package proof

Pantheon replaced readiness-by-HTTP-200 with an exact tuple gate over service
version, API v0, supervisor profile, runtime SHA, loaded model, the sole
advertised model, and model-manifest SHA. Drift is terminal and adversarial
tests cover wrong runtime/model/manifest/profile/API and multiple models.

The real copied v25 package initialized Metal and passed exact readiness with
runtime `f967611c...`, manifest `931bc842...`, and artifact set `c6c315c9...`
covering 12,935,160,119 bytes. The first boundary run revealed an intentional
contained metallib symlink; the invariant now accepts only contained regular
file links. The second revealed that `LC_RPATH` is a directory contract, not a
file contract. A `/var` versus `/private/var` fixture also required canonical
root comparison. All corrections are executable tests. No production catalog
entry changed and no performance claim was made.

## 2026-08-18 - v26 observed-capacity readiness

SNE now captures serving cache capacity and native prefix-session maximum from
the loaded backend on the locked MLX owner thread and publishes them in
readiness. Pantheon gained optional exact expectations and rejects a degraded
cache/session contract even when runtime and manifest identities match. The new
copied v26 package retained native runtime `ef6a2a4b...`, MLX `d08f5aa8...`, and
metallib `2eb93da0...`; only the service/readiness surface changed. A real Metal
gate passed runtime `fc979e20...`, manifest `81e9332f...`,
`paged-ring-4096`, capacity 4096, and session maximum 2. Production catalogs
were not changed and no performance claim was made.

## 2026-08-18 - v26 exact restart and resource re-admission passed

Pantheon started the durable copied v26 package, validated exact runtime,
model, manifest, cache topology, 4K serving capacity, and two native sessions,
then performed a full supervised restart. PID 94803 was replaced by PID 94885.
The replacement independently reverified the 12.94 GB artifact set and
package-local MLX dependency and passed the same exact readiness contract.
Pantheon's resource gate re-admitted 17 GiB required under normal pressure with
32.38 GB available, 10.20 GB reserve, and 518.12 MB swap used. The executable
gate now rejects same-PID pseudo-restarts and any post-restart identity,
capacity, dependency, or resource drift. No production promotion or speed claim
was made.

## 2026-08-18 - stable model identity separated from runtime package identity

Pantheon gained additive `runtime_id` selection. Existing one-package model
entries continue to resolve without change. Multiple runtime packages for one
stable model now require unique explicit runtime IDs, and lifecycle requests
must name the desired runtime; catalog order can never decide. Tests reject
ambiguous implicit selection, duplicate model/runtime pairs, mixed
legacy/explicit variants, and unknown IDs. No production catalog was modified.

## 2026-08-18 - installed SNE runtime catalog signed and admitted

Pantheon gained exact-byte detached Ed25519 verification before catalog parsing.
The production default lifecycle requires signature and pinned public key paths.
A purpose-built signer preflights both key destinations, refuses overwrite,
writes atomically, and keeps the private key outside source/packages at mode
0600. The installed 11-entry sne-gemma4-v1 catalog passed both the low-level and
default-lifecycle real gates. One-byte mutation and wrong-key tests fail closed.
The public trust root is durable in configs/sne. This signature is host-scoped
because the current catalog contains absolute roots; portable release-catalog
materialization remains open and is explicitly not claimed.

## 2026-08-18 - signed release catalogs became path-portable

Runtime catalogs may now identify packages with a safe package ID. Pantheon
authenticates the exact catalog first, then materializes each ID beneath its
configured package store. Adversarial tests reject traversal, separators, mixed
absolute/portable identity, root escape, and root aliasing. The real installed
absolute-path catalog remained valid after the migration layer. Release
qualification still needs one generated portable signed catalog exercised on
clean M1 and M5 hosts.

## 2026-08-18 - portable signed catalog passed real M5 product lifecycle

The live M5 Pantheon catalog was migrated non-destructively to the signed
package-ID catalog; prior signed files remain rollback copies. The default
lifecycle authenticated the catalog, materialized the plain 12B affine-8 package
under Application Support, verified 8 governed files totaling 12.94 GB, loaded
the package-local MLX dylib, reached readiness, and stopped cleanly. A first
portable verifier run used repo-relative environment paths and failed because Go
tests run from package directories; the real gate now requires absolute paths.
A later combined suite hit known sandbox loopback denial and passed in the
approved host boundary. M1 Tailscale SSH timed out before transfer, so identical
cross-host bytes remain unproven rather than inferred.

## 2026-08-18 - signed catalog update, rollback, and removal became transactional

Pantheon now installs catalog/signature pairs into immutable SHA-256 version
directories and switches one current symlink atomically. A real copied probe
moved current from bbcdbaf9... to 6beef3da..., the default loader read the probe
catalog, rollback restored bbcdbaf9..., the loader read the original, and the
inactive probe was removed. The active version cannot be removed, unexpected
files fail closed, and bad updates leave current unchanged. Default lifecycle
now reads only the atomic current bundle.

## 2026-08-18 - signed catalog state reached the operator surface

The SNE read model and Pantheon UI now show the exact authenticated catalog,
immutable current version, retained versions, rollback availability, and runtime
identity. A real default test pins the installed catalog values. This closes the
unit-pass/product-divergence class for catalog diagnostics and records the
general prevention rule rather than relying on future operator vigilance.

## 2026-08-18 - signed catalog recovery became operator-actionable

Pantheon now lists retained immutable catalog versions and exposes governed
rollback and inactive-removal actions. Mutations require a stopped lifecycle,
same-origin request, explicit confirmation, and the full digest. Rollback still
reverifies the signed bundle and package materialization before switching.

## 2026-08-18 - catalog update acquisition became authenticated and atomic

Pantheon now checks a configured signed HTTPS feed, resolves only an exact
signed digest, verifies the downloaded catalog again, and installs it through
the immutable atomic store while retaining rollback. A manager mutation lease
prevents lifecycle races. The first feed is staged and signed but intentionally
inactive until its referenced release assets are actually published.

## 2026-08-18 — SNE support diagnostics product gate
- Added an explicit allowlisted diagnostics schema and dashboard download endpoint rather than serializing internal runtime state.
- Reused Pantheon's authoritative SNE read model and resource sampler; did not create a competing telemetry path.
- Added regression tests that inject private paths, temporary paths, DYLD state, and credential labels into omitted fields and fail on any serialized leak.
- Focused packages and full Pantheon suite passed. Real M5 product endpoint reported the exact signed catalog SHA, device identity, 11 runtime entries, 16 governed tuples, and live resource state; privacy scan passed.
- Captured Permanent Rule 35 and published repo, Desktop MD/HTML, and Google Workspace evidence.
- Process lesson retained: integration truth requires the actual downloadable product boundary after unit/full-suite success; raw errors and structs are never safe support artifacts.
- Next: project exact runtime/model/no-fallback provenance into Nexus streaming conversation and user-facing recovery.

## 2026-08-18 - Governed Nexus model control seam

Pantheon now permits explicit SNE start/stop/lifecycle requests from only its own origin or the existing exact Nexus origin allowlist, handles browser preflight, and rejects unknown origins before mutation. The lifecycle remains restricted to installed, signed, hardware-admitted tuples; downloads, license acceptance, and research opt-in remain Pantheon-local. Focused dashboard tests pass.

## 2026-08-18 - Pantheon governed OpenAI-compatible local API

Pantheon now registers loopback `/v1/models` and `/v1/chat/completions`. Model discovery exposes only the active ready tuple after signed runtime-catalog verification and includes SNE runtime/catalog/device/execution provenance. Completions reuse the existing exact-model proxy and now negotiate SSE only for `stream:true`, JSON for `stream:false`. Nexus Ask Sirsi dogfoods `/v1/chat/completions`; component tests and production build pass. Focused Pantheon origin, lifecycle, model-list, streaming, and non-streaming proxy tests pass. Live packaged-service qualification remains open.

## 2026-08-18 - One-copy SNE model residency transaction

Pantheon now removes the exact prepared model source only after the checkout helper returns a schema-valid successful result. Failed checkout retains staging for resume and diagnosis. The cleanup helper refuses the prepared root itself and targets outside it. SNE checkout stores verified checkpoint content once and hardlinks governed plain/MTP views to it while keeping assistant artifacts distinct. Focused checkout and dashboard suites pass; real large-model inode/disk proof and shared-object lifecycle/GC remain open.

## 2026-08-18 — SNE model-store recovery integrated into Pantheon

Pantheon production defaults now require `~/.local/bin/sne-model-store-recover` before model installation or lifecycle launch. Missing or failed recovery suppresses availability and runtime selections, marks lifecycle failed, and rejects install/start with the cause. Durable checkout/remove/recovery helpers were installed; real model-store recovery returned zero pending removals and removed nothing. Focused tests and the full dashboard package pass. The real signed-catalog test now passes against current `sne-gemma4-v2` with 12 entries and two retained versions; its obsolete v1/11-entry constant was replaced with signed-current internal-consistency assertions. Canon: `docs/evidence/SNE_MODEL_STORE_RECOVERY_INTEGRATION_20260818.md`.

## 2026-08-18 — Pantheon completion contract restored

Created and validated `.agents/completion.contract.json` as a platform-foundation contract spanning product, design, technical, operational, and narrative closure. Initialized the SNE model-store recovery proof and corrected the generated scaffold so only canon actually reviewed in this turn is listed; the draft proof validates structurally while contract-wide commands remain truthfully not run. Human companion: `docs/governance/PANTHEON_COMPLETION_CONTRACT_20260818.md`. Contract, companion, recovery evidence, and draft proof are mirrored to the Desktop Pantheon Owner Reading Room. Google Workspace mirror remains visibly pending.
## 2026-08-18 — SNE model-store recovery contract-wide verification passed

The first broad verification was rejected because the restricted execution
environment denied loopback/Unix sockets, hardware probes, and launchctl. An
independent host-permission rerun passed `go test ./...`, `go vet ./...`, and
the exact dashboard/SNE/snemodels focused suite. The rejected run remains
incident evidence and is not conflated with a product regression. Receipt:
`docs/evidence/SNE_MODEL_STORE_RECOVERY_VERIFICATION_20260818.json`.

## 2026-08-20 — SNE crash recovery became memory-admission truthful

- Graph-Caretaker crash correctness passed: exit 137, endpoint closure, distinct restart PID, no surviving prefix sessions, exact stateless output recovery, and readiness restoration.
- Resource recovery failed: swap grew 2.06 MiB -> 3,259.81 MiB; restarted RSS was 24,014,487,552 bytes.
- Root cause: the package declared 20 GiB while observed active high-water was 25,545,459,702 bytes, and direct lifecycle tests could omit required memory and bypass admission.
- `NewSupervisor` now hash-verifies and decodes the exact manifest, derives required memory when omitted, rejects zero or disagreement, and uses a node-relative total-RAM/12 reserve for lifecycle restarts.
- Focused `go test ./internal/sne` passes. Production remains fail-closed because current swap exceeds policy.
- SNE created an immutable-parent-preserving measured-memory descendant with manifest SHA `b90829d0793d7aaf96836dc47a5b4264d78ce944c91e42ac6a346f44f4a2cc6e`; it is not yet executed or promoted.
- Canon: `docs/evidence/SNE_GRAPH_CARETAKER_CRASH_MEMORY_ADMISSION_20260820.md`.
- Supervisor output and real lifecycle receipts now expose `lifecycle_reserve_bytes` separately; focused command/supervisor and SNE tests pass.

## 2026-08-20 - Typed lifecycle admission recovery

- Added stable, privacy-safe admission error codes and recovery instructions for missing measurements, host pressure, excess swap, and insufficient lifecycle headroom.
- The supervisor now stores the failed measured admission sample before returning, so CLI/UI/Nexus diagnostics report real values rather than zeros.
- Failed restart admission preserves the retryable parent context; no unsafe replacement process is launched.
- `go test ./cmd/sirsi-sne-supervisor ./internal/sne` passes. No model or GPU test ran.
- Multi-hour, pressure, thermal, and long lifecycle qualification is overnight-only. Daytime work is bounded.

## 2026-08-20 - SNE recovery reaches Pantheon UI and support export

- Lifecycle state now carries typed admission code, recovery action, and the failed measured resource snapshot.
- Pantheon renders memory, lifecycle reserve, swap limit, recovery guidance, and an accessible explicit retry; it never auto-retries.
- Privacy-safe support diagnostics retain the failure-time sample and continue excluding free-form private error content. `go test ./internal/dashboard` passes.

## 2026-08-20 - Stable OpenAI-compatible SNE recovery errors

- `/v1/models` and `/v1/chat/completions` now return stable OpenAI error objects plus privacy-safe SNE no-fallback and recovery metadata.
- Low-level local dial errors and arbitrary upstream bodies are not exposed; successful SSE remains byte-preserved. Focused dashboard tests pass.
## 2026-08-20 - Governed SNE support consumption

- Added strict parsing for the SNE support-matrix projection with exact status, assistant, claim-boundary, tuple, and no-fallback checks.
- Pantheon discovery now matches support evidence against the admission registry's exact architecture, precision, model, memory, and artifact identity.
- Only `release-supported` enables a new install/start action; candidate, research, unqualified, missing, and mismatched evidence fail closed and expose the next gate.
- An already active model remains stoppable. Support status and next gate are included in privacy-safe diagnostics.
- Source compilation only; no SNE service, model, Metal workload, or package operation ran.
- Tightened consumption to require a detached Ed25519 signature under Pantheon's installed SNE catalog public key. An unsigned matrix can no longer influence install/start eligibility.
- Added a versioned transactional support-matrix store. Source bytes and detached signature are verified before staging, the immutable SHA-addressed version is reverified, and only then does one atomic current-pointer switch expose it to discovery.
- Adversarial focused tests now prove valid signature acceptance, signed-byte tamper rejection, false release promotion rejection, immutable version activation, and current-pointer escape rejection.
- Pantheon's model cards now render support state and next gate directly, color states semantically, and give enabled controls exact model/runtime accessible names; disabled controls expose `aria-disabled` without pretending to be buttons.

## 2026-08-20 — OpenAI API exact support admission
- Closed a server-side bypass where `/v1/models` and `/v1/chat/completions` accepted a ready signed runtime without requiring its exact active model/runtime tuple to be `release-supported`.
- Added one shared exact-tuple admission function. Discovery and chat now require active model ID, exact runtime ID, verified signed runtime catalog, and `support_status=release-supported`.
- Pilot, research, unqualified, missing, or mismatched tuples fail closed before proxying. The typed local error preserves `no_fallback=true`, support status, and next qualification gate.
- Focused dashboard tests passed. No model, GPU, memory-pressure, swap, thermal, or reboot workload ran; disruptive qualification remains overnight-only.
- Extended the governed local API contract: `/v1/models` now publishes release support provenance; advanced OpenAI fields are proven byte-preserved; oversized requests fail before upstream; established streaming cancellation and pre-header caller deadlines are covered by bounded tests.
- Replaced one-new-transport-per-generation with one bounded shared loopback HTTP transport. This preserves streaming semantics, propagates caller cancellation, enables connection reuse, disables proxy inheritance, and avoids transport churn/GC leakage.
- All focused OpenAI/SNE dashboard tests pass under a hard 15-second ceiling. No inference or disruptive host work ran.
- Pantheon now preserves SNE's `Retry-After` header with the exact local `429 queue_full` body. Focused proxy tests prove the recovery hint survives the governed OpenAI-compatible boundary; no retry or fallback is invented by Pantheon.
- Pantheon support matrix consumer now requires schema v2 and exact serving policy. Strict tests reject v1/unbound policy, max concurrency above one, non-FIFO or missing queue/deadline semantics, signature tamper, and false release promotion. No live matrix was activated.
- Serving policy is now one fail-closed identity from signed support matrix through Pantheon supervisor launch and live SNE readiness. Corrected `anubis.yaml` from false concurrency 4 to 1/8 FIFO and `ra.yaml` from unbounded 0 to 1/32 FIFO; both use 120000 ms.
- Pantheon passes max concurrency, queue depth, and request timeout explicitly. Client decodes policy from both ready and status endpoints; supervisor rejects endpoint disagreement or drift from the governed profile.
- Centralized expected interactive/fleet policy in `internal/sne/serving_policy.go`. Focused client, supervisor, readiness, support signature, and adversarial drift tests pass.

## 2026-08-20 - SNE model license disclosure and consent

- Pantheon already required and receipt-bound license acceptance, but the UI asked users to accept unnamed generic terms.
- Projected the exact source-catalog license identity into the SNE read model and model card, with an official terms link.
- Install confirmation now names the model, license identifier, URL, and transactional acceptance receipt.
- Added a fail-closed license registry for Gemma Terms and Apache 2.0; unknown terms disable Install rather than collecting ambiguous consent.
- Focused dashboard route/license/install tests pass. No download, model, or GPU workload ran.
- Canon: `docs/evidence/SNE_MODEL_LICENSE_DISCLOSURE_AND_CONSENT_20260820.md`.

## 2026-08-20 - Governed SNE model removal

- Added a same-origin model-removal API that accepts only an exact admitted
  catalog-entry/model-ID pair and refuses every lifecycle state except stopped.
- Added a separate keyboard-accessible Remove model control; Start is not
  displaced. Confirmation explains shared-object retention and reinstall.
- Pantheon invokes SNE's native transactional helper with explicit arguments,
  never a shell or ad hoc filesystem fallback, and returns its removal receipt.
- Corrected helper paths to the one installed SNE package root. Focused
  dashboard tests pass; no model data or GPU work ran.
- Canon: `docs/evidence/SNE_GOVERNED_MODEL_REMOVAL_CANDIDATE_20260820.md`.
- SNE Product Doctor and the external lifecycle harness now verify that
  checkout, recovery, and removal remain present after update and rollback.
- Closed an inferred-capability gap: Pantheon measures all three installed
  helpers before enabling removal and reports only privacy-safe readiness plus
  repair/reinstall guidance. Focused dashboard tests pass.
- Added a real-handler UI regression gate for separate Start/Remove controls,
  keyboard activation, accessible naming, shared-object disclosure, and
  reinstallability language. This does not replace hands-on VoiceOver proof.
- Added shared-shell visible focus, reduced-motion, and increased-contrast
  behavior. The real served-page contract test passes; this remains source
  accessibility evidence rather than a clean-host VoiceOver claim.
## 2026-08-20 - Packaged SNE support bundle exposed safely

- Added a separate consent-gated, keyboard-accessible complete support-bundle action while retaining Pantheon's lightweight JSON diagnostics.
- Pantheon delegates archive composition to the installed SNE package, applies same-origin, 15-second, 4 MiB, no-store, and `nosniff` gates, then removes temporary output.
- Errors do not include helper output or local paths. Focused `internal/dashboard` suite passes.
- No service, model, inference, or GPU workload ran; signed clean-host evidence remains open.

## 2026-08-20 - Fresh-boot zero swap is readable admission evidence

- Localized `swap_measurement_unavailable` to `readSwapPct`: it equated a zero allocated swap total with telemetry failure even when `sysctl -n vm.swapusage` succeeded and parsed `total=0 used=0`.
- The parser now tracks successful parsing independently of magnitude. Zero/zero is valid and avoids division by zero; missing fields, malformed values, negatives, and used greater than total remain unreadable.
- Added direct regression coverage for zero swap, ordinary allocated swap, and malformed/impossible responses. `go test ./internal/guard ./internal/sne` passes.
- No admission reserve, pressure threshold, swap ceiling, model declaration, package identity, or SNE execution component changed.

## 2026-08-21 - SNE admission accounts for verified snapshot file cache

- Localized a macOS admission false negative after full model hashing: active file-backed snapshot pages were omitted from Pantheon's reclaimable-memory estimate even though XNU treats file-backed pages as generally reclaimable under pressure.
- `hapiFreeRAMBytes` now conservatively chooses the larger of queue-based `free + inactive + speculative` and file-cache-aware `free + file-backed` estimates.
- Required model memory, lifecycle reserve, swap ceilings, pressure gates, and emergency gates are unchanged. Focused guard/SNE tests pass; live controller-free policy-v7 lifecycle evidence remains pending.

## 2026-08-21 - Durable application and process recovery contract

- Added a registry-bound recovery manager for app-owned saved state, launchd services, and explicitly checkpoint-aware processes.
- Recovery now persists an atomic receipt after capture, stop, start, and readiness; Pantheon can resume an interrupted recovery from the last safe durable phase.
- Admission requires a replacement PID and optional loopback readiness. State paths are captured and reverified before launch.
- The macOS driver uses public relaunch/supervision mechanisms and rejects arbitrary shell execution, remote readiness probes, and undeclared targets.
- Canon explicitly distinguishes Chrome-style session restoration from impossible claims of restoring arbitrary unpersisted heap, instruction-pointer, GPU-stream, or socket state.
- Product service/UI registration and clean-host restoration evidence remain pending; no launch-grade UI claim is made yet.
- Added explicit `restore` and `fresh` restart intents. Restore requires declared durable state; fresh may clear only exact registered files and rejects symlinks/directories. Every receipt records intent, preventing cache-clearing maintenance from being misrepresented as state recovery.
- Hardware Admin audit found no older independent restart API; current authority is Pantheon's `internal/apprecovery` implementation. Corrected fresh-reset ordering so ordinary targets must prove the old PID exited before registered transient files are cleared. Generic launchd replacement clears only in-memory state unless a service-specific persistent-state contract exists.
- Wired recovery into Pantheon's CLI and resident menubar dashboards through one fixed owner-only strict JSON registry. Added a dedicated keyboard-operable Recovery view with explicit Restore, Fresh restart, and interrupted Resume controls. Browser responses omit paths, arguments, state hashes, PIDs, and raw driver errors. No target is enabled by the checked-in example; real clean-host target qualification remains required.
- Added transactional `sirsi recovery list/add/remove` enrollment so users do not hand-edit authority JSON. Registration now admits only existing non-symlink executables, valid platform identities, safe state-file types, and loopback readiness; mutation rechecks ownership/private permissions and atomically syncs a 0600 replacement. Running Pantheon must restart before acquiring changed authority.
- Real isolated macOS checkpoint-aware lifecycle gate passed for both restore and fresh modes. Each stopped a live private copied helper, produced a distinct replacement PID, and reached ready; restore verified checkpoint identity and fresh removed exactly its declared transient file. Scope remains local mechanism evidence, not clean-host `.app` or launchd admission. Canon and owner HTML were created; Workspace mirror remains visibly pending.
- Real generic launchd fresh-replacement gate passed using a uniquely named temporary user LaunchAgent: `kickstart -k` produced a distinct ready PID, then the fixture was booted out. No installed service was touched. The 10.314s package wall time includes launchd bootstrap/cleanup and is not a recovery-latency claim.
- Temporary real `.app` saved-state restoration and new-manager interrupted-receipt resume now pass. The app consumed the same session before/after exact-bundle relaunch with a distinct PID; a replacement manager completed a durable stopped receipt without prior memory.
- Full focused recovery/dashboard gate passes after two failure-derived corrections: exact executable identity accepts registered `/var` or canonical `/private/var`, and BSD pgrep alternation uses POSIX capture syntax. App launch uses exact bundle path, avoiding mutable LaunchServices bundle lookup. These rules are canonical to prevent recurrence.
- Added opt-in startup reconciliation. Pantheon resumes only already-pending durable receipts for targets registered with auto-resume; it never initiates a restart and ignores ready, failed, absent, and non-opted-in state. Full focused recovery/dashboard gate remains green.
- M1 clean-host recovery attempt: read-only preflight reached arm64 macOS 26.6.1 with no repo/registry; SCP closed and subsequent SSH streaming timed out before transfer or execution. No M1 claim. The M5 temporary artifact was removed.
- Recovery capability/latest phase now appears in the privacy-safe SNE diagnostics allowlist. Tests positively require target/class/auto-resume and retain negative exclusion of paths, hashes, arguments, PIDs, and raw errors. Dashboard and apprecovery suites pass. One later M1 SSH retry timed out; no polling loop remains.

## 2026-08-21 - SNE host memory ceiling fails closed

- M1 evidence exposed a copied 40 GiB SNE process ceiling on a 16 GiB host while the admitted E2B NVFP4 model required 6 GiB.
- Added launch-boundary `ValidateHostMemoryCeiling`; impossible profiles now fail with stable code `memory_ceiling_exceeds_host_capacity` rather than being silently clamped or launched.
- Preserved independent live admission for model footprint, available RAM, reserves, pressure, and swap.
- Focused SNE resource and host-ceiling tests pass.
- Canon: `docs/product/SNE_HOST_MEMORY_CEILING_ADMISSION_20260821.md`.
- Remaining launch work: unlocked clean-host 8K Direct-Paged qualification, M1/M5 performance isolation, model matrix, Nexus, durability, signing/notarization, and distribution gates.

## 2026-08-21 - SNE loopback authorization and DNS-rebinding boundary

- Registered SNE, OpenAI-compatible, and recovery routes now reject every non-loopback or ambiguous HTTP Host before a handler runs, closing the DNS-rebinding gap left by Origin equality alone.
- Added a constant-time bearer capability for inference and state-changing routes plus a restart-stable 256-bit private token store with strict mode, symlink, and malformed-state rejection.
- Pantheon Menubar now provisions the capability and opens Nexus through a fragment handoff; storage failure makes protected operations fail closed with an unexported random credential.
- Authenticated rotation atomically replaces both durable and in-memory credentials; the prior token is rejected immediately.
- Full internal/dashboard and menubar package tests pass. Live packaged Pantheon-to-Nexus launch, rotation reconnect, and stronger signed-client/Keychain same-user isolation remain open.
- Canon: `docs/product/SNE_LOOPBACK_CAPABILITY_BOUNDARY_20260821.md`.

## 2026-08-21 — Canonical Pantheon engine and live SNE authorization

- Found release-channel identity drift: DMG selected SwiftUI by file presence;
  standalone release selected the Go control engine.
- Made `cmd/sirsi-menubar` canonical in both release paths and added
  `scripts/verify-menubar-release-contract.sh` to CI.
- Closed empty-token bypasses in `sirsi dashboard` and `sirsi-gui`.
- Focused suites passed. Live matrix on isolated port 19119 returned 401 for
  missing bearer, 403 for invalid bearer, and 503 `sne_not_ready` for the valid
  durable bearer, proving authentication crossed without hidden fallback.
- Signed-native Data Protection Keychain/XPC remains the GA boundary. Do not
  claim it complete until a Developer-ID-signed Pantheon/Nexus pair is tested.
- Certificate audit found both issued public certificates but no matching
  private key on M5 or M1. A `.cer` alone cannot sign; create a new key+CSR.
- Used Apple `certtool` to create a new 2048-bit key directly in login Keychain
  and a verified CSR (SHA256 `a980e404...b34373`). No loose private key remains.
  Apple portal upload is pending owner sign-in and has not been performed.
- Closed the embedded-dashboard auth regression with a loopback-only HttpOnly,
  SameSite=Strict session cookie. Live cookie action=200; hostile Origin=403.

## 2026-08-21 - SNE post-admission recovery closes false-readiness gap

- Confirmed from the Sirsi Hardware Admin task that no separate Hardware Admin restart API exists; Pantheon's registered recovery/supervisor substrate is canonical.
- Added generation-scoped post-admission SNE readiness monitoring. It remains dormant before exact `WaitReady` admission, requires three consecutive production failures, and signals only the currently admitted child.
- Existing memory-gated monitor performs the fresh-process replacement; the replacement must pass complete runtime/model/manifest/policy/cache/session identity admission.
- Forced-failure and integrated SNE/dashboard/entrypoint tests passed. Clean unlocked model-backed M1/M5 recovery proof remains required.
- Closed a readiness admission race: a late response from process generation A cannot admit replacement generation B. The focused stale-generation and forced-recovery tests pass.
- Durable corrected candidate: `artifacts/candidates/pantheon-sne-post-admission-recovery-v2-generation-guard-20260821/sirsi-menubar`, SHA-256 `bbb6245b20d71953573658f9961e8cb36fc8b96275100385b49d59a26d061cf8`.
- M1 audit: locked, AC/100%, 128.25 MiB swap. Installed legacy SNE `0.1.0-dev` reports only generic ready identity and is inadmissible under current Pantheon; no M1 performance projection or promotion.
- Apple Developer session is authenticated at Developer ID Application / G2 CSR selection. CSR remains local and unuploaded pending action-time confirmation.

## 2026-08-21 - SNE profile-scoped cache and prefix capability
- Joined signed runtime catalog cache topology, serving capacity, and prefix-session maximum to exact model+runtime selections.
- Exposed those fields through Pantheon catalog and governed `/v1/models`.
- Derived prefix support only for execution mode `plain` with maximum > 0; MTP modes remain unsupported even with shared cache infrastructure.
- `go test ./internal/dashboard ./internal/sne -count=1` passed.
- Built copied candidate `artifacts/candidates/pantheon-sne-profile-capabilities-v1-20260821/sirsi-menubar`, SHA `5b50c61edad33556c6d08c04f5731d4bbc67ef6d1607a1925d3e02a54896e9a4`.
- Installed Pantheon unchanged. Next runtime gate requires unlocked host and exact selected SNE profiles.

## 2026-08-21 — SNE API contract is an admission identity

An M1 SNE 1.1.8 service proved that coarse `api_version=v0` can span incompatible
request schemas. Pantheon now defaults every governed launch to exact contract
`sne.openai-chat.v2`, reads it independently from readiness and status, and
rejects absence, drift, or disagreement before service admission. Focused SNE
supervision tests pass; copied binary SHA is
33defc5b43570e9530ad78e4d39bb558f1eec644937d99a5375a4f4d81f8498d.

## 2026-08-21 - SNE recovery ownership boundary

- Re-audited Hardware Admin, generic application recovery, and the SNE
  supervisor before registering a restart target.
- Confirmed Hardware Admin has no independent restart implementation.
- Confirmed SNE already has exact-admission post-start recovery under
  `internal/sne`; the generic `internal/apprecovery` contract cannot express
  SNE's model/runtime/precision/memory identity atomically.
- Rejected duplicate generic SNE enrollment and canonized the SNE supervisor as
  sole lifecycle owner. Shared UI may project its status, but cannot launch a
  second restart controller.

## 2026-08-21 - Cryptographic SNE response identity
- Propagated resolved runtime SHA-256, model-manifest SHA-256, and profile into lifecycle and /v1/models.
- Admission now rejects missing, malformed, or uppercase hashes and missing profile.
- Focused dashboard and supervisor tests pass.
- Canon: docs/product/SNE_OPENAI_CRYPTOGRAPHIC_RESPONSE_IDENTITY_20260821.md

## 2026-08-21 - Signed SNE service version contract
- M1 audit exposed that supervisor omitted --version, causing GA package to report 0.1.0-dev.
- RuntimePackage accepts signed service_version or strict SNE-X.Y.Z package derivation.
- Lifecycle fails closed without canonical version; supervisor passes exact value.
- internal/sne and internal/dashboard tests pass.
- Canon: docs/product/SNE_SIGNED_SERVICE_VERSION_CONTRACT_20260821.md

## 2026-08-21 - Supervised SNE capability propagation

Pantheon now reuses its existing durable SNE local capability across the supervisor boundary. The private file path is passed to `sned`, Pantheon's internal client presents the bearer capability, and launch fails closed on unsafe capability files. Focused Go tests pass; live package integration remains pending.
# 2026-08-21 - SNE commercialization and support provenance

- Added the missing `docs/COMMERCIALIZATION_GATE.md` with explicit user, pain, workflow, value, trust, ownership, and done-evidence boundaries for Pantheon/SNE.
- Added runtime SHA, model-manifest SHA, profile, cache topology/capacity, and prefix capability to privacy-safe SNE diagnostics.
- Kept prompts, generated text, credentials, environment values, private paths, network configuration, and machine identity excluded.
- `go test ./internal/dashboard ./internal/sne ./internal/snemodels` passes.
- Canon: `docs/evidence/SNE_COMMERCIALIZATION_AND_SUPPORT_PROVENANCE_20260821.md`.
- Draft proof `.agents/proofs/sne-v2-api4096-product-integration-20260821.json` now passes structural validation while truthfully retaining live host and Workspace blockers.
# 2026-08-21 — SNE dashboard exact-tuple readiness projection

- Audited the Pantheon read-model projection after confirming the supervisor already enforces exact service identity.
- Found that the dashboard independently called the service readiness endpoints and projected `ready` without binding the response to Pantheon's lifecycle snapshot.
- Replaced that permissive projection with `sneReadinessMatchesLifecycle`: API contract, profile, runtime SHA, model ID, manifest SHA, one-model cardinality, queue policy, concurrency, and timeout must all match the supervised lifecycle state.
- Added adversarial tests for unsupervised state and runtime/model/manifest/profile/API/multi-model/queue/timeout drift.
- `go test ./internal/dashboard ./internal/sne ./internal/snemodels` passes.
- Claim boundary: this is a product-integrity/readiness repair, not API4096 Metal qualification or a performance result.
# 2026-08-21 — Actionable SNE identity-mismatch recovery

- Preserved the exact-tuple fail-closed readiness gate.
- Added a distinct `identity-mismatch` service state when a live endpoint reports ready but does not match Pantheon's supervised lifecycle tuple.
- The recovery instruction tells the operator to stop the unverified local service and restart the installed model through Pantheon.
- Generic start guidance is used only when no more specific recovery exists.
- Focused dashboard/SNE/model suites pass.
# 2026-08-21 — SNE evidence three-home publication

- Regenerated the Owner Reading Room HTML from current canonical Markdown after connector readback exposed a stale HTML import.
- Imported and verified native Google Doc `1kUgEvjTuG9oabxzlRCqUOAkG2QwzQoJOsJnLCm8fiSI`.
- Readback confirms the exact-tuple readiness and identity-mismatch recovery sections are present.
- Permanent deletion of the stale first import was rejected as irreversible; it was safely renamed with a `SUPERSEDED` prefix.
- This closes the Workspace mirror blocker for this evidence document only.
# 2026-08-21 — SNE commercialization/support three-home closure

- Created native Google Workspace document `1b-uVcZsWJ9tfjiti2ZSqiNQHCEWZdJJJtMA_efswfJY` for Pantheon's SNE commercialization and support provenance.
- Replaced the obsolete Workspace-pending blocker in repo canon and both Owner Reading Room Markdown mirrors.
- This closes publication provenance only; clean-host signing, notarization, installation, and performance qualification remain open.
# 2026-08-21 — SNE Developer ID signing boundary (corrected)

- The initial query targeted the wrong/obsolete release keychain. The governed private key and canonical SNE release keychain were found; their public-key identity matches the August Developer ID certificate, and the canonical keychain now exposes valid Developer ID Application identities.
- Direct Developer ID and App Store signing remain distinct lanes, but the direct-distribution identity is no longer blocked on a certificate producer.
# 2026-08-21 — Native Pantheon SNE control surface

- Added `macapp/Sources/SirsiMenubar/SNEControl.swift` as a SwiftUI client of Pantheon's authoritative Go SNE API.
- Added Home navigation for **SNE — Models & Engine** and registered the screen in snapshot QA.
- Surface covers discovery, exact lifecycle identity, license consent/install, start, stop, safe retry, model removal, signed-catalog rollback/removal, and actionable recovery.
- Mutation requests read only the canonical private regular mode-600 capability file and send bearer authorization. Swift does not reimplement tuple admission, memory policy, catalog verification, or model-store rules.
- First sandboxed SwiftPM invocation failed before compilation because nested `sandbox-exec` was denied. Escalated identical build reached source, exposed three styling errors, those were corrected, and the build passed.
- Live screen, assistive-technology, clean-host, signed-package, and M1/M5 integration evidence remain open.
- Three-home publication completed at Google Doc `1Ts1kz9kJpLsN3P3Mq_2-wsyaI-xSFd1BAs7dKguQ9T4` with repo canon and Owner Reading Room Markdown/HTML.
# 2026-08-21 — Native SNE deterministic visual QA closure

- Accepted minimum and wide populated-fixture renderings after visual inspection.
- Preserved accepted images and SHA-256 checksums in `docs/evidence/artifacts/sne-native-mac-control-20260821/`.
- Recorded three rejected iterations: absent `.task` fixture state, AppKit `GroupBox` content loss under `ImageRenderer`, and unsupported Link/progress glyphs plus clipping.
- Updated repo canon, Owner Reading Room Markdown/HTML, and native Google Doc `1Ts1kz9kJpLsN3P3Mq_2-wsyaI-xSFd1BAs7dKguQ9T4`.
- Claim remains bounded to deterministic layout. Live lifecycle, VoiceOver/accessibility, signed package, clean-host, M1/M5, and inference performance proof remain open.
# 2026-08-21 — Authenticated secured-SNE readiness projection

- Found `sneReadModel` created an unauthenticated client while secured SNE requires bearer authorization for `/v1` status/models.
- Routed the read projection through the existing rotation-safe Pantheon capability snapshot; no new credential store or authority was introduced.
- Added a secured fake-service regression proving a rotated capability reaches readiness, status, and model calls.
- `go test ./internal/dashboard ./internal/sne -count=1` passes.
- Live secured API4096/Metal proof remains pending the unlocked graphical session.
- Canon: `docs/evidence/SNE_AUTHENTICATED_READINESS_PROJECTION_20260821.md`.
# 2026-08-21 — Authenticated SNE queue transparency

- Added authenticated `/v1/sne/metrics` consumption through Pantheon's current rotating capability.
- Dynamic counts are projected only when queue limits, FIFO policy, and timeout match admitted readiness identity.
- Missing or drifted telemetry remains visibly unavailable rather than fabricated as zero.
- Dashboard/SNE focused suites pass; live contention remains pending API4096 execution.
- Canon: `docs/evidence/SNE_QUEUE_TRANSPARENCY_TO_NEXUS_20260821.md`.
# 2026-08-21 — Native SNE accessibility semantics

- Added stable accessibility identifiers and explicit spoken labels for engine readiness, progress, active runtime, model identity/actions, recovery, signed-catalog state, and lifecycle-tool availability.
- Hid the decorative readiness color from assistive technology and made model-specific action context explicit.
- The sandboxed SwiftPM invocation was denied before compilation by nested `sandbox-exec`; the identical escalated build compiled, linked, and passed.
- Live VoiceOver and lifecycle proof remains truthfully open because the M5 graphical-session gate still reports the console locked.

# 2026-08-21 — Actionable locked-session SNE recovery

- Classified the known locked graphical-session Metal start failure as `metal_session_locked` rather than an opaque runtime failure.
- Pantheon projects `waiting-for-unlock`, retains the exact tuple, and leaves healthy already-running services unchanged.
- Native SNE control says `Waiting for unlock`, exposes an accessible `Retry after unlocking`, and retries the same tuple when macOS reports the session active while the exact error still persists.
- Retry re-enters normal admission and never clears caches, changes model/precision, or permits fallback.
- Focused dashboard lifecycle/readiness tests and current-toolchain Swift build pass. Live lock/unlock, Metal startup, and VoiceOver evidence remain open.
- Updated the existing three-home native-control record; Google Doc `1Ts1kz9kJpLsN3P3Mq_2-wsyaI-xSFd1BAs7dKguQ9T4` readback confirms the new recovery section.
- Moved unattended recovery ownership into the Go lifecycle manager using one cancellable native `IOConsoleLocked` watcher. Exact retry and owner-stop cancellation tests pass; the UI listener remains an immediate user-facing companion, not the sole caretaker.
- Rejected the initial full-tree ioreg poll after a bounded 20-sample measurement: 1251.682 ms mean versus 9.418 ms for root-depth-1 (99.25% reduction). Production now uses the narrow standalone-property parser at a five-second interval; missing/invalid/spoofed values fail closed.
# 2026-08-21 — Native runtime identity correction

- Cross-repo API4096 integration audit found Pantheon verified service/native hashes correctly but passed the service executable hash to `sned --runtime-sha256`.
- `resolveLaunch` now selects only `NativeRuntimeSHA256`; service identity remains independently verified against `bin/sned`.
- Added an explicit LaunchConfig semantic invariant and adversarial regression with distinct hashes.
- `go test ./internal/dashboard ./internal/sne -count=1` passes. No Metal/model/performance run occurred.
- Current signed catalog remains pre-API4096 by design; promotion requires accepted API4096 evidence and a new signed tuple.
- Three-home publication completed at native Google Doc `1b5aulkcj3dn0Tz03FVsBzitPGB2Or-TTBaWUPaQZYJA`; readback confirmed defect, correction, evidence, and catalog boundary.

# 2026-08-21 — API4096 catalog-candidate boundary and v26 lineage recovery

- Added `sirsi-sne-catalog-candidate`, an unsigned exclusive-output transaction that requires the signed current catalog, accepted API4096 v2 receipt, promoted product pointer, API4096 parent ancestry, actual package identities, complete gate set, and package dependency-boundary verification.
- Added focused Go and real-current-state negative tests. Missing admission exits 67, creates no output, and leaves the signed catalog unchanged.
- The negative gate exposed a pre-existing mismatch: repository v2 had been replaced by 11-entry staging bytes `36073d1a...` while the signature and canon described admitted v26.
- Recovered exact admitted 12-entry v26 catalog `aca14182...` and signature `ed649deb...` from Pantheon's immutable catalog store; recovered matching v26 model-admission registry from the durable live store. Preserved all staging regressions as explicitly superseded files.
- Full signed lineage now passes with predecessor immutability, one active tuple, and v26 present. No API4096 successor, signature, install, activation, model run, or performance claim occurred.

# 2026-08-21 — Pantheon install-suite contention repair

- A repository-wide Go sweep exposed two SNE installation tests that assumed asynchronous acquisition and subprocess checkout would finish within exactly one second.
- Under full-suite CPU/process contention, both jobs remained legitimately active beyond that artificial window; this was a test-harness timing defect, not a failed production state transition.
- Replaced the fixed 100-by-10ms polling loops with explicit ten-second deadlines while retaining immediate assertions for installed and failed terminal states.
- Repeated `go test ./... -count=1` passed every Pantheon package, including dashboard, app recovery, cleaner, SNE, model acquisition, mobile, and end-to-end tests.
- Durable lesson: asynchronous lifecycle tests must express a bounded wall-clock contract and may not encode host scheduler speed as product correctness.

## 2026-08-21 - SNE dashboard and CLI integration revalidation

- `go test ./internal/dashboard ./cmd/sirsi` passed.
- Dashboard package passed from cache; CLI package completed in 32.138 seconds.
- Existing duplicate `-lobjc` linker warning remains non-failing.
- Focused SNE diagnostics and support-bundle tests passed. Diagnostics preserve identity, resource admission, and recovery without forbidden-value leakage; support export remains bounded, same-origin protected, and downloadable.
- Focused locked-session recovery tests passed: narrow fail-closed lock parsing, `waiting-for-unlock` projection, exact tuple preservation, automatic retry after graphical unlock, and stop cancellation. Real host transition remains pending.

## 2026-08-21 - Focused SNE model-delivery qualification

- Ran the bounded `internal/snemodels` acquisition/source suite with `-count=1`.
- Accepted resumable exact-source verification and first-party derivative resolution.
- Rejected corrupt content, unsafe transport, and revision URL-path injection.
- Ran the bounded dashboard install/removal suite with `-count=1`.
- Accepted transactional verified install, prepared-source recovery semantics,
  exact-identity removal receipts, cross-origin/active-runtime protection, and
  keyboard-accessible removal controls.
- Both package commands exited 0. No model, GPU, network checkout, immutable
  package, or performance path ran. Live acquisition qualification remains open.

## 2026-08-21 - Governed live model checkout runner

- Added `scripts/run-sne-live-model-checkout.zsh` (SHA
  `b3dd71eb1945a7d4c418f4a9243de5212df8d60a0af6dee9b138d34e2e919cd9`).
- Added `scripts/test-sne-live-model-checkout-contract.zsh` (SHA
  `c809871ed00369dabbb883a32e236a28cd01b8fefae04fa5fe1abb2f6069dd73`).
- The first test caught zsh's read-only `status` special variable. Renamed it to
  `exit_status` and reran the unchanged contract successfully.
- Accepted proof: complete result, no partial artifact, hash-bound receipt.
- Rejection proof: wrong catalog hash exits 67 before destination/evidence residue.
- The authoritative 26B source catalog totals about 15.6 GB. A real qualification
  must fetch/verify that full tuple; no metadata-only shortcut will be credited.
- No immutable SNE release, live network, model, Metal, or performance path changed.

## 2026-08-21 - Real 26B resident checkout accepted

- Ran the governed live-checkout runner against the exact resident
  `26b-a4b-affine4-runtime-complete` source tuple.
- Verified exact signed catalog SHA `e19253c6d58e0bddbcc43c3dedb88b37fa76b63eb06a9fa2f5c4caf147a3870c`.
- Production Go acquisition verified 8/8 files and 15,641,238,190 bytes.
- `resumed_bytes=15,641,238,190`: every artifact was already exact; network
  transfer was zero and no duplicate model was created.
- Evidence result SHA is `8d7dffa4907d81a5e6d1545571c0be1cd5e32a79ddeff3f1379943603c86b1f9`;
  receipt SHA is `a8933d07f3cde90cae591e0ae5f7d88928324269f53cfdbe07a95be8a38ea949`;
  stderr is empty.
- Storage trace found one incomplete older prepared source and one distinct
  rejected checkout. No direct receipt/path dependency was found, but cleanup
  remains a governed lifecycle operation; no ad-hoc deletion occurred.
- This is source acquisition/reuse proof only, not serving or release proof.

## 2026-08-21 - Gemma license enforcement qualification

- Focused dashboard license/install tests passed with `-count=1`.
- Canonical `gemma-terms` resolves to Google's terms URL.
- Unknown terms fail closed; absent acceptance conflicts; hostile origin is
  forbidden; accepted terms remain part of checkout/install identity.
- This proves backend contract enforcement only. Clean-host user interaction and
  accessibility evidence for terms review/acceptance remain required.

## 2026-08-21 - Governed retained-source cleanup

- Root cause: failed model checkout intentionally retained prepared bytes, but
  Pantheon exposed no governed operation to discard them later.
- Added authenticated same-origin `POST /api/sne/prepared/discard`.
- Request accepts only `catalog_entry`; signed catalog resolves exact revision.
- Cleanup rejects unknown/path-like identity, active acquisition/checkout, absent
  or invalid source, and any target outside PreparedRoot.
- Installed model-store objects and rejected-checkout evidence are outside the
  endpoint's authority.
- Focused tests and full dashboard package (`-count=1`) pass.
- Production stale source remains untouched until UI/receipt integration is live.

## 2026-08-21 - Accessible retained-download recovery UI

- Failed installations now expose `[Discard retained download]` rather than
  leaving model-scale bytes with no product recovery path.
- Confirmation explicitly limits scope to failed prepared source and states that
  installed models/shared objects are unchanged.
- Control is keyboard-operable, exact-catalog-entry bound, and presents the
  returned revision receipt or actionable rejection.
- Focused cleanup tests and the full dashboard package pass with `-count=1`.
- No production bytes were deleted automatically.
## 2026-08-21 - Package-bound SNE support privacy verification

Pantheon now requires the verifier shipped beside the exact installed SNE
exporter, executes it against the newly generated archive, and streams bytes
only after acceptance. Successful responses carry
`X-Sirsi-Support-Privacy-Verified: true`; missing verifier or rejected archive
returns no download and actionable repair/update guidance. Focused and broader
dashboard support suites pass. Existing signed SNE bytes remain unchanged; the
next copied package inherits the verifier.

The UI gate renders the real served dashboard and requires consent,
Enter/Space keyboard activation, privacy-verified success language, and visible
failure recovery. The corrected focused suite passes.

Added an opt-in real-package integration test and ran it against the exact
copied SNE support-privacy candidate. Pantheon's production Go wrapper invoked
the package exporter and verifier and returned a valid ZIP within 2.062 seconds.
Normal suites remain hermetic when no package root is supplied.

Transferred exact SNE candidate `5899e19f...91fc` and compiled Pantheon test
binary `177ac4e1...40a2` to M1 macOS 26.6.2. After target SHA checks and isolated
install, Pantheon's production Go wrapper invoked the installed exporter and
verifier and returned a valid ZIP in 2.75 seconds. Packaged uninstall left zero
residue. Log SHA is `b7a4f6f1...3383`. No model or Metal ran.

Canon: `docs/evidence/SNE_PACKAGE_BOUND_SUPPORT_PRIVACY_VERIFICATION_20260821.md`.

---

## 2026-08-21 - SNE dual runtime identity closure

- Repaired Pantheon's launch-time collapse of separately cataloged service and native hashes.
- Supervisor, client, lifecycle, dashboard, and CLI now carry service SHA, native SHA, and exact package dylib path separately.
- Both status and ready endpoints must return both expected identities; focused tests pass.
- Live Metal/API4096 admission remains pending.
- Canon: `docs/evidence/PANTHEON_SNE_DUAL_RUNTIME_IDENTITY_20260821.md`.

## 2026-08-21 - E2B API-v2 Pantheon-supervised M1 admission

- Created copied model-admission, readiness, runtime-catalog, and M1-profile
  candidates. Production catalogs and immutable r5 remained unchanged.
- Signed the copied runtime catalog with Pantheon's protected Ed25519 identity;
  independent verification against the pinned public key passed.
- Pantheon rejected three real version-skew defects before model admission:
  installed supervisor lacked current flags, installed profile lacked serving
  policy, and old readiness IDs did not match the current admission catalog.
- Rebuilt the current supervisor, generated readiness from exact admission IDs,
  and required explicit service version `1.2.2`.
- Repaired compact-node reserve scaling: the 8 GiB floor remains on 32+ GiB
  nodes and is capped at 25% of smaller nodes. Focused guard/SNE suites pass.
- Found the old installed supervisor left a 2.75 GB r5 child after bootout. The
  isolated gate reaped only that child before launch and restored r5 afterward.
- Final M1 launch passed Pantheon identity admission, package/model load, dual
  runtime identity, secured HTTP 200, and exact `M1-READY`.
- No clean AC/performance, correctness-policy, clean100, Nexus, or release claim.
- Canon: `docs/evidence/SNE_E2B_API_V2_PANTHEON_M1_ADMISSION_20260821.md`.
## 2026-08-21 - Governed SNE streaming semantics

- Extended the signed runtime package contract with bounded streaming semantics.
- Only `incremental-sse` and `buffered-compatibility-sse` are accepted; arbitrary or promotional labels fail closed.
- Projected the capability through Pantheon's SNE read model and support diagnostics for Nexus and operator use.
- Existing signed catalogs and immutable packages were not modified; absent metadata remains explicitly unreported.
## 2026-08-21 - SNE process-group containment

- Rooted the prior 2.75 GB orphan risk in direct-PID-only supervision and cancellation ordering.
- Admitted launches now receive a dedicated process group; graceful stop precedes context cancellation, and every forced path cleans the group.
- No process-name scavenging or unrelated PID selection was added.
- Packaged launchd/crash requalification remains open.
- Real process-descendant containment passed on M5 and M1 (macOS 26.6.2) with identical test artifact `2948b46a...b746e`; the remote transient binary was removed.
- Real E2B child-crash testing exposed a second defect: restart admission failed correctly on compact-node headroom, but the foreground command did not exit after monitor failure, preventing launchd recovery. Added post-readiness `Supervisor.Wait` propagation to close that lifecycle gap.
- Supervisor `SIGKILL` then proved launchd does not automatically reclaim the separately grouped `sned`. Added a copied-candidate child-side parent PID watchdog and made Pantheon pass exact ownership on every supervised launch.
- Canon: `docs/evidence/SNE_SUPERVISOR_PROCESS_GROUP_CONTAINMENT_20260821.md`.

## 2026-08-21 - Real M1 launchd crash recovery accepted

- Built copied parent-bound `sned` (`37ba074e...8709`) and Pantheon supervisor (`b20991e8...459f3`); focused suites passed.
- Rejected the first watchdog run because the harness launched its stale canonical-stage supervisor while the repair existed only as a suffixed sibling; the child command exposed the missing `--parent-pid`.
- Replaced only the transient canonical executable with the hash-locked repair; immutable packages remained untouched.
- Accepted normal launch, child-crash recovery via fresh launchd supervisor, supervisor `SIGKILL` recovery through child parent-loss shutdown, and final empty group.
- The harness now requires/verifies the exact supervisor SHA before launchd mutation and seals supervisor path/SHA into the receipt. All-zero SHA failed closed with exit 65 while r5 retained the same PID and no test listener appeared.
- Strengthened receipt lineage: supervisors `33648/33786/33887`; services `33673/33806/33913`. Receipt SHA `d3ce5589...d362`; installed r5 was restored.
- This is M1 lifecycle evidence, not correctness/performance or M5 release evidence. Raw artifacts live under `docs/evidence/artifacts/sne-launchd-process-group-m1-20260821/`.

## 2026-08-21 — Native candidate registry verification
- Added `cmd/sirsi-sne-registry-verify` to enforce admission/readiness identity and optional manifest/artifact-set pins.
- Exact E2B v8 registry v3 passes identity; correctness and wrong-artifact controls fail closed.
- Exact package passed isolated signed catalog-store install; tampered signature failed. No production catalog/service mutation.

## Entry 047 — 2026-08-21 18:26 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- Pantheon SNE install UX: replaced browser confirm license acceptance with explicit keyboard-accessible terms dialog. Dialog identifies exact model, opens governed terms URL, requires unchecked explicit consent before enabling Accept and install, separates cancel, and preserves transactional release-supported backend request. Focused SNE route/license/install API tests pass.
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-11T04:28:50Z
- last Claude read: 2026-06-16T15:30:16Z
- pending: none

---

## Entry 048 — 2026-08-21 18:30 — Session Compact (COMPACT)

> Persisted via `thoth compact` before context compression.

**Decisions**:
- Pantheon native menu-bar SNE/Nexus handoff drift fixed. Menubar no longer hardcodes sirsi.ai root fragment; it calls shared dashboard.BuildNexusCapabilityURL with governed https://sirsi.ai/local-ai route, fragment-only capability, no query credential, and missing-capability rejection. Focused cmd/sirsi-menubar test passes. Evidence: docs/evidence/PANTHEON_MENUBAR_NEXUS_GOVERNED_HANDOFF_20260821.md.
- Router snapshot:
- active topics: ra-horus-router-hypervisor-canon, finalwishes-tier1-ga, finalwishes-dependabot-sweep, finalwishes-owner-readiness, finalwishes-lob-google-photos, finalwishes-rag-architecture, finalwishes-mobile-architecture, pantheon-mac-native-cli-pivot, lean-af-cross-repo-cleanup-sweep
- completed topics: 41
- last Codex read: 2026-06-11T04:28:50Z
- last Claude read: 2026-06-16T15:30:16Z
- pending: none

---
## 2026-08-21 - Pantheon SNE unlock-recovery installation audit

- Focused lifecycle tests pass for locked-session publication, exact preserved-
  tuple retry after graphical unlock, stop cancellation, narrow fail-closed
  IOConsoleLocked parsing, and owner-facing waiting-for-unlock projection.
- The M5 launchd domain has no loaded `ai.sirsi.pantheon` service and no canonical
  Pantheon plist is installed in user/system LaunchAgents or LaunchDaemons.
  The label appears only in launchd's enabled override map, which is not proof
  of installation or execution.
- Therefore current API4096 qualification uses a dedicated security-preserving
  watcher. Pantheon product behavior still needs real model-backed lock/unlock
  proof after the exact API4096 tuple passes parent, varied20, and signed catalog
  admission. No runtime, package, catalog, or release artifact changed.
## 2026-08-21 - Pantheon caretaker clean-host launch repair

- Distribution audit found that shell installation installs binaries but leaves
  LaunchAgent creation to interactive `sirsi setup`; this host therefore had an
  enabled label override but no installed or loaded Pantheon caretaker.
- Generated setup used deprecated `launchctl unload/load`. The bundled release
  plist searched four paths through a login shell and wrote logs to `/tmp`.
- Repaired generated and bundled caretaker contracts to execute an exact binary
  directly, rely on unified logging, and avoid ephemeral log paths.
- Added reusable modern registration: enable the user-domain label, best-effort
  bootout, then bounded bootstrap retries across asynchronous teardown.
- Regression tests verify exact direct-path plist structure and launchctl
  argument vectors. Focused tests, full `internal/setup`, and plist lint pass.
- No app was installed or launched on the locked owner host; clean-host package,
  signed build, upgrade, rollback, uninstall, and live unlock recovery remain.
- Added `sirsi surface install gui|menubar` as the narrow idempotent caretaker primitive. Noninteractive GitHub installation invokes only this command when the menubar binary exists; failure remains visible with an exact retry command.
- An isolated temporary-home test proves bundle copy, direct-path plist, and enable/bootout/bootstrap registration without touching the real launch domain.
- Closed one-copy residency for signed Pantheon.app installs: setup validates the existing bundle ID/executable with `/usr/bin/plutil`, points launchd to that exact app, and does not create a duplicate home-directory bundle. Loose standalone binaries retain stabilization into a proper app bundle.
- Homebrew cask caveats and GitHub DMG README now use the same `sirsi surface install gui` command; shell installs use `surface install menubar`. Full setup tests and both shell syntax gates pass.
- Repaired clean uninstall to use label-aware `launchctl bootout gui/$UID/$LABEL` and `disable` before removing caretaker/supervisor plists. An isolated temporary-home test proves both jobs are addressed and both plists removed; dry-run remains shell-free. Full setup suite passes.
- Made caretaker registration transactional with same-directory staging, fsync, and atomic rename. A failed first bootstrap removes the final plist; a failed successor bootstrap restores and re-bootstraps the exact predecessor. Focused injected-failure tests and full setup suite pass.
- Hardened caretaker verification: bootstrap success is followed by exact user-domain `launchctl print`; failed registration enters the transactional rollback path. `sirsi surface` now distinguishes missing, installed-but-stopped, and installed-plus-loaded states. Focused and full setup tests pass.

## 2026-08-21 - FileVault restart truth and version-correct RC2 package

- Confirmed automatic login is unset. Ordinary FileVault restarts stop at preboot login; only a pre-authorized `fdesetup authrestart` can perform one-time restart continuity without disabling FileVault.
- Confirmed `ai.sirsi.sne-api4096-v3-gui-continuation` survived restart as one RunAtLoad LaunchAgent and remains running/fail-closed while the GUI is screen-locked; no duplicate watcher was launched.
- Repaired `scripts/build-dmg.sh`: environment and flags now resolve before linker metadata; app Info.plist receives the same VERSION and numeric BUILD_NUMBER; the CLI uses CGO because current native vitals require it.
- Built and read-only mounted local ad-hoc Pantheon 1.0.0-rc2 build 2026082101. Bundle ID, app version, CLI version, and signature structure agree. DMG SHA-256: `066860c9944568ce887020568f5e85bc030861beeae64885058009ea394a6d8e`.
- External signing/notarization was denied for lack of exact-payload authorization. No credential import, upload, publication, or immutable SNE mutation occurred. The candidate is non-distributable pending explicit authorization.

## 2026-08-21 - Artifact-level package identity gate

- Added `scripts/verify-pantheon-package-identity.sh` and wired it into every DMG build before image creation or notarization.
- It fails closed on missing embedded CLI/control engine, malformed build number, bundle ID/version/build disagreement, CLI/app version disagreement, invalid signature, ad-hoc signature in Developer ID mode, wrong authority, or wrong team.
- Extended `scripts/verify-menubar-release-contract.sh` to require native-vitals CGO, plist identity embedding, and artifact verification.
- Bash syntax and both release gates pass against local Pantheon 1.0.0-rc2 build 2026082101.

## 2026-08-21 - Deterministic fail-closed release CI

- GitHub tagged releases now pin `BUILD_NUMBER` to `${{ github.run_number }}.${{ github.run_attempt }}` instead of inheriting a wall-clock default.
- Added optional secure preparation of an App Store Connect API key from repository secrets; Apple ID credentials remain fallback-only.
- Tagged release builds set `REQUIRE_RELEASE_SIGNING=1`; `build-dmg.sh` exits before image creation if no Developer ID identity is supplied.
- Extended the static release contract to require deterministic CI identity and mandatory signing. Bash syntax, contract verification, and workflow YAML parsing pass.

## 2026-08-21 - API4096 catalog evidence binding reverified

- `test-sne-api4096-catalog-candidate-admission.zsh` passes.
- Negative path: missing admission exits 67, creates no output, and does not alter the signed catalog.
- The candidate tool cannot sign, install, or activate. Its required gate values must come from the strict hash-bound SNE admission receipt.

## 2026-08-21 - Real M5 caretaker deployment and installer hardening

- The preinstalled Pantheon 0.23.7 CLI did not contain `surface install`; it
  interpreted the command as surface selection and installed nothing.
- Current 0.23.8 source-built `sirsi` + `sirsi-menubar` siblings exercised the
  transactional installer. Sandboxed launchctl returned EIO; the identical
  narrow command outside the Codex sandbox succeeded without sudo.
- Launchd now proves `ai.sirsi.pantheon` running as PID 84269 with one exact
  RunAtLoad+KeepAlive direct executable:
  `~/Applications/Sirsi Menubar.app/Contents/MacOS/sirsi-menubar`, SHA-256
  `b291f4ee6e006be65c2ec170e578e16ff95830525e92cbc933c07cfc577eed57`.
  The bundle passes strict code-signature verification and the job has never
  exited.
- Fixed two installer observability/trust defects: launchctl failures now retain
  stderr, and a release app carrying signing metadata is canonical only when
  strict deep signature verification succeeds. Full `internal/setup` and
  `cmd/sirsi` suites pass.
- Pantheon SNE ownership remains deliberately `not-installed`. The immutable
  API4096 tuple is accepted by current evidence but is not yet a signed catalog
  entry; no stale research LaunchAgent was promoted around that gate.
- The host subsequently screen-locked. Fresh20/8K Metal tests remain correctly
  blocked by session admission, not by caretaker deployment.

## 2026-08-21 - Transactional Pantheon package candidates

- A copied 0.23.8-beta build first proved app identity but hdiutil was denied in
  the sandbox; the identical build outside the sandbox produced and read-only
  verified a 14 MiB development DMG.
- The Developer ID Application identity was recovered in the existing dedicated
  `Sirsi-SNE-Release-v4.keychain-db` (SHA-1 identity
  `4BE5346FCC67C3240A5288D1B959D269A9DA812C`). The user keychain search list had
  malformed path composition and was repaired to explicit login + release
  keychains. The release keychain remains locked; signing fails closed with
  `errSecInternalComponent`. No password was guessed or exposed.
- Repaired `scripts/build-dmg.sh` so every attempt builds in an isolated
  temporary directory, retains a build-numbered app candidate, and moves the
  public versioned DMG into place only after the full signing/notarization path
  succeeds. A failed successor can no longer mutate the last accepted package.
- Positive ad-hoc build 20260821.4 passed package identity and release-contract
  checks. Its promoted DMG SHA-256 is
  `5a15fbfa2f1e9246e5b4b5d575b0ea1ff5d80e2c689312c6dc48b40c39e0fb96`.
- Deliberate build 20260821.5 with a nonexistent signing identity exited 1 and
  preserved that DMG SHA exactly. This is packaging durability evidence, not a
  distributable release claim.

## 2026-08-21 - Resident SNE product-manager parity repair

- Audit found `sirsi dashboard` configured both governed SNE installation and
  lifecycle managers, while `sirsi-menubar` omitted both fields when creating
  the same dashboard server.
- The omission left the normal resident product surface read-only: model
  acquisition and supervised lifecycle backend code existed but was unreachable
  through the installed caretaker.
- Menubar now supplies `DefaultSNEInstallConfig()` and
  `DefaultSNELifecycleConfig()` exactly like the CLI dashboard. Focused
  `cmd/sirsi-menubar` tests pass.
- This is source evidence only until a copied successor package is built and
  the live caretaker is transactionally replaced; immutable SNE was untouched.

- Copied package 0.23.8-beta build 20260821.6 passed package identity and DMG
  construction, then replaced the live caretaker through the same transactional
  installer. Launchd reports running PID 90770, runs=1, no prior exit, bound to
  the exact build-numbered app candidate. Resident UI wiring is therefore live.
- This remains an ad-hoc development deployment. It does not satisfy Developer
  ID, notarization, signed-catalog API4096 admission, or public distribution.
## 2026-08-21 — Pantheon control plane no longer waits for AppKit readiness

- Live audit disproved PID-only readiness: launchd had a resident caretaker,
  but the dashboard was absent until `systray` invoked `onReady` after graphical
  availability.
- Extracted one `controlPlane` owner and initialized it before `systray.Run`.
  The visual callback now consumes that state rather than constructing service
  ownership. Headless mode and all shutdown paths share the same object.
- Added startup-order and idempotent-stop regression coverage. Focused menubar
  and dashboard suites pass.
- Built copied candidate `20260821.7`. The sandboxed `hdiutil` attempt failed
  closed and preserved the prior candidate; an identical outside-sandbox build
  completed transactionally.
- Installed through `sirsi surface install gui`. Launchd reports one exact .7
  process (PID 96074, first run, no exit); that PID listens on loopback 9119 and
  returned live dashboard/stats responses.
- Candidate is ad-hoc and remains development-only. A true locked-session
  restart proof, Developer ID/notary closure, and absolute RAM-byte telemetry
  repair remain required. No model or performance claim changed.
## 2026-08-21 — Apple-silicon RAM telemetry repaired in copied candidate .8

- Traced zero absolute RAM fields to an intentional `CollectStats` placeholder.
- Found a deeper shared-vitals defect: `vm_stat` pages were always multiplied by
  4096 even when the kernel declared 16384-byte Apple-silicon pages.
- Shared vitals now parses the declared page size, clamps impossible used bytes,
  and exposes exact total/used/free bytes. Menubar stats copy that contract.
- Added 4 KiB and 16 KiB regression coverage. Focused vitals, menubar, and
  dashboard suites pass.
- Built and transactionally installed copied candidate .8. Live launchd state:
  PID 2534, runs=1, no exit, exact .8 executable. Live stats: 51,539,607,552
  total, 10,911,121,408 used, 40,628,486,144 free, 21.1704%, low pressure.
- .8 remains ad-hoc and development-only; no SNE model or benchmark ran.
## 2026-08-21 — True locked-session Pantheon continuity accepted

- Captured one host-level sample while `IOConsoleLocked = Yes`.
- Exact copied .8 caretaker remained running as PID 2534, `runs = 1`, with no
  prior exit.
- The same locked session served fresh structured `/api/stats` telemetry over
  loopback 9119. This closes the AppKit-independent service-continuity gate.
- Full FileVault reboot/login restoration remains separately open; it is not
  inferred from a screen-lock result.

## 2026-08-22 - SNE lifecycle dependency restored

- Live `/api/sne/lifecycle` exposed `failed` because the formal SNE recovery helper was absent.
- A verified, data-preserving tooling repair installed the immutable API4096 helper set into Application Support without changing models or package history.
- Restarted launchd-managed Pantheon; lifecycle now initializes as `stopped`, proving recovery passed.
- Metal execution remains separately gated by the currently locked console.
## 2026-08-22 — Legacy SNE serving-policy migration

Post-restart lifecycle evidence found the installed `sne-profile.yaml` retained the pre-policy `max_concurrent_requests: 4` shape. Added `LoadOrMigrateSupervisorProfile`, which atomically backs up and upgrades only that exact known interactive legacy shape to one native request, eight FIFO waiters, and a 120000 ms deadline. Unknown policy drift remains fail-closed. Dashboard launch resolution now uses the migration-aware loader. Focused `internal/sne` and `internal/dashboard` tests pass.
## 2026-08-22 - Strict v3 API4096 catalog candidate generated

- `sirsi-sne-catalog-candidate` now preserves legacy v2 fixture compatibility
  while requiring the exact full model-manifest schema for every v3 promotion.
- Focused generator and SNE tests pass; generator SHA is `9b6e31b4...e2e8d`.
- Exact v8/v22 lineage produced unsigned candidate `9d8ace96...2387` with no
  signing, installation, or activation capability. Existing signed catalog was
  authenticated and left unchanged.
## 2026-08-22 - Seshat repeated Google authentication repaired

- Root cause was Seshat's access-token-only implementation: no refresh, no
  atomic persistence, and a retired out-of-band OAuth URL.
- Added PKCE loopback desktop OAuth, proactive and one-time-401 refresh, atomic
  `0600` token storage, and secret-free auth status. Focused Seshat and CLI
  suites pass; fixed CLI SHA `3ce510a7...eb4c4`.
- Firebase is independently healthy and refreshed four projects without browser
  login. Existing gcloud ADC lacks Drive scope. No Seshat token/client exists;
  the sole downloaded web client belongs to FinalWishes and was rejected.
- Operational remainder is one Sirsi-owned desktop OAuth client and one consent;
  thereafter renewal is automatic.

## 2026-08-22 - M1 transport and lock-independent SNE continuity accepted

- M5 proved M1 reachable by direct Tailscale ping (7 ms), TCP 22, TCP 5900, and authenticated SSH while the M1 graphical console was locked.
- M1 supervised plain E2B NVFP4 SNE remained ready with exact model identity, zero swap, and 68% free memory.
- Permanent classification: successful bounded transport probes establish reachability; idle Tailscale `Active=false`/empty `CurAddr` cannot override them. GUI lock, agent session, and SNE readiness remain separate dimensions.
- Canon: `docs/evidence/PANTHEON_M1_TRANSPORT_AND_LOCK_INDEPENDENT_SNE_CONTINUITY_20260822.md`; Owner Reading Room MD/HTML mirrors created. Native Workspace mirror is visibly pending because Seshat is Drive-readonly.

## 2026-08-22 - Pantheon installer/updater and Seshat activation repair
- Reproduced owner-visible Homebrew failure: the app recommended `brew upgrade sirsi-pantheon` even though the cask was not installed and the tap required trust.
- Replaced drift remediation with Pantheon's signed application updater: `sirsi update --app`; `sirsi fix` now invokes that same executable contract.
- Corrected a second defect where PATH/app pathname inequality was treated as binary drift even when version and commit matched. Added focused accepted/rejected identity tests.
- Installed the repaired local 0.23.9-beta app at `/Applications/Pantheon.app`, switched `ai.sirsi.pantheon` to that durable path, installed the liveness watch, and proved diagnosis reports both checks healthy.
- Proved Seshat Google Workspace authorization is already durable and refreshable. The Cloud Console screen was not an activation step and no owner browser action is required.
- Developer-ID distribution remains correctly blocked: `Sirsi-SNE-Release-v4.keychain-db` rejects unattended private-key access with `errSecInternalComponent`. The working local app is ad-hoc signed and is not a publishable artifact.
- Preserved Sirsi Admin containment: no disabled experimental SNE launch agents were restarted.

## 2026-08-22 - Ruthless persistence containment and Pantheon memory repair

- Executed the owner's exact containment authorization: deleted ten disabled
  experimental SNE LaunchAgents and stale Sirsi launch files marked bak, OFF,
  quarantined, retired, miswired, superseded, or reconciled; removed Adobe
  Collaboration Synchronizer from login items; disabled and removed the
  Pantheon liveness supervisor during containment; removed the dedicated SNE
  release keychain from automatic search without deleting its file; restarted
  only Pantheon. The sole remaining Sirsi LaunchAgent is
  `ai.sirsi.pantheon`. No experimental SNE agent was restarted.
- Startup tracing localized Pantheon's 1.2-1.4 GiB footprint to
  `guard.StartBridge -> StartWatch -> stele.Inscribe -> stele.Open`.
  `stele.Open` read the entire lifetime JSONL ledger, converted it to a string,
  and split every line merely to recover the final sequence and hash. This
  allocated about 749 MiB of live Go heap and 1.446 GiB of heap arenas.
- Replaced whole-ledger loading with a bounded tail reader that inspects at
  most the final 1 MiB, discards a partial leading record, validates entries
  backward, and restores the exact chain tip. A 16 MiB regression ledger proves
  bounded recovery. Focused Stele, Guard, and menubar tests pass.
- Identical initialization after repair measured about 28.5 MiB RSS, 3.3 MiB
  live heap, and 15.9 MiB heap system allocation. The installed local Pantheon
  process measured 12.5 MiB physical footprint, approximately a 99% reduction.
  The rebuilt CLI completed diagnosis in 1.4 seconds rather than expanding the
  ledger into gigabyte-scale memory.
- Also removed the automatic startup full-disk Jackal scan. Architectural law:
  Spotlight owns broad filesystem discovery on macOS. Pantheon may query its
  metadata index and maintain governed Sirsi deltas or explicit product
  manifests; it must not operate a competing broad automatic index. Replacing
  the remaining manual Jackal crawler with a Spotlight-backed provider is the
  next Pantheon implementation phase.
- Removed the four-hour timer as well: resident Pantheon now performs no hidden
  Jackal crawl at startup or on a schedule. It hydrates the persisted governed
  manifest and watches explicit Scan/Clean publications. Manual deep-forensic
  Jackal scans remain user-invoked. Built and installed the repaired local
  menubar binary, SHA-256 `69b2c685...019358`, and restarted only Pantheon.

## 2026-08-22 - Menubar unified-action regression localized

- Owner review correctly identified that many Pantheon menubar signals ended in
  informational Terminal windows or prose rather than resolution.
- The remediation machinery was not lost: the dashboard retains a typed action
  registry, runner, event stream, two-phase confirmation tokens, notifications,
  and SNE/recovery APIs. The defect is duplicated surface wiring: the menubar
  hardcoded commands and never consumed the canonical registry, allowing drift.
- Repaired immediate safe bypasses in the installed local candidate: Gemma
  start/check, Router Doctor safe repair, Compute Profile, Diagnostics, and
  Guard now execute natively with progress and drill-down; Horus opens the real
  dashboard instead of Terminal. Installed menubar SHA is `6f9cbb40...98cce`.
- Canonized the full release-blocking closure matrix at
  `docs/product/PANTHEON_MENUBAR_ACTION_CLOSURE_20260822.md`. Remaining work is
  one shared registry, finding-to-remediation projection, native destructive
  confirmation, and complete SNE/recovery/update/permissions/service controls.

## 2026-08-22 - Menubar unified-action closure accepted locally

- Exported the canonical dashboard action registry and corrected stale verbs
  that did not exist in the installed CLI (`doctor`, `quality`, `dedup`, and
  `guard --once`). The menubar now submits typed ActionRequests to the same
  loopback runner instead of hardcoding execution semantics.
- Added server-issued, single-use prepare/hash/token/commit rows for Ra fleet
  mutations, system repair, network repair, signed updates, and SNE stop or
  quarantine. Safe services execute natively and publish drillable results.
- Added SNE, Repairs & Recovery, Horus, Fabric, process relief, ghost cleanup,
  and Vault entrypoints. Authenticated restart remains the sole automatic
  Terminal exception because macOS owns credential collection.
- Added terminal runner receipts and made menubar success contingent on a
  matching key with `status=success`; idle no longer means success.
- Fixed repeated Desktop permission dialogs: resident disk-access detection had
  opened Desktop/Documents/Downloads/Mail/Safari on every refresh. It now reads
  only the TCC authorization database, whose denial is silent. Protected folders
  are touched only by explicit user operations.
- Final installed local pair: CLI `d4888a68...c8b37`, menubar
  `1bc6c205...1fa89`. Focused tests pass. Installed diagnosis is 100/green with
  no actionable finding; safe action receipt is exact; destructive proof was
  prepare-only. Resident observation found no TCC request or scanner process.
# 2026-08-22 — Portfolio-wide macOS permission silence

The repeated Desktop consent prompt exposed a broader architectural defect:
resident code was using protected-resource access as permission detection. The
repair now covers every TCC category and every core Sirsi product. Pantheon is
the sole human authorization broker; CLI/TUI/MCP/helpers/SNE/Inference and
Hypergraph remain non-prompting. Swift startup TCC registration, automatic
notification authorization, Go TCC database probing, and the duplicate Swift
probe were removed. The release pipeline executes the six-repository static
gate. Tests: portfolio gate accepted; `go test ./internal/platform
./cmd/sirsi-menubar` passed with host access; Swift package built successfully
(it has no test target). No installed app was re-signed because the visible
Login keychain currently has zero code-signing identities. Hidden release
keychains remain outside automatic search. This is a truthful release blocker,
not a reason to ad-hoc replace the installed application.

## 2026-08-22 - Zero-open local-AI replacement goal activated

- Replaced the narrow launch objective with the owner-ratified zero-open-work
  goal spanning Pantheon, SNE v2, Nexus local AI, total-chip observability,
  packaging, distribution, and publication.
- Converted the closure inventory from six coarse groups to 36 individually
  owned and evidenced obligations. Initial disposition is 8 complete, 26
  runnable, 2 external-blocked, and zero unknown/stale/hung inventory records.
- Closed four abandoned coordination routes with exact evidence: three retired
  SNE v1 article-review requests and one liveness request that would have
  re-enabled intentionally contained research agents.
- Repaired the Homebrew source contract by removing the ambiguous same-token
  formula, retaining `sirsi-pantheon` as the app cask, introducing the distinct
  `sirsi-pantheon-cli` formula, and validating tap readability and style. Remote
  publication and clean-host behavior remain runnable and are not overclaimed.

## 2026-08-22 - Public Homebrew contract and task-ledger reconciliation

- Published isolated Homebrew repair commit `1ea137f` while preserving unrelated
  owner/agent files. Public refresh resolves `sirsi-pantheon` only as the app
  cask and `sirsi-pantheon-cli` only as the headless formula.
- The GitHub DMG digest exactly matches the cask. The M1 clean target has no
  Homebrew, proving DMG must remain the primary clean-user path. M1 transport
  disappeared before install; no remote bytes changed.
- Fenced and completed four exact false-open inference records with distinct
  evidence. Partial, failed, and active work remains open. Added a lineage map
  from legacy SNE/AppleStack families into the active 36-item inventory.
