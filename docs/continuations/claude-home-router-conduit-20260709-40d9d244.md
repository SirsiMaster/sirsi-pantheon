<!-- THREAD-SCOPED CONTINUATION — do NOT load unless you ARE this thread's successor.
     agent: claude-home | workstream: router-conduit (binder/reviewer/dispatch discipline)
     repo: sirsi-pantheon (router home) | date: 2026-07-09
     session: 40d9d244-9f30-4202-92eb-f8aa7cedfca2
     path: docs/continuations/claude-home-router-conduit-20260709-40d9d244.md
     NOTE: owner is archiving the source thread and moving this work to the
     ROUTER CONDUIT SUPERVISOR as its permanent home. -->

# claude-home → router-conduit supervisor — wrap-up & handoff (2026-07-09)

This thread was the claude-home conduit: bind/review across all portfolio repos, router
triage, fleet CI hygiene, and the incident work of 2026-07-02 → 07-08. Owner directive at
archive time: **the router conduit supervisor is now the home of the router** — this file is
the handoff into that home.

## OPEN — pick these up first

1. **Codex verification of merged #168 (dispatch facade)** — item
   `20260708-155614-claude-home-codex-pantheon-verify-merged-168-...`, still open,
   **blocked on the Codex app being launched** (codex-pantheon is wake-unavailable;
   interactive agents are never blind-spawned). When Codex runs, the verdict lands in that
   item's **Result section on close** — `sirsi router pull` will NOT surface it; check the
   item directly. Then: relay verbatim to claude-pantheon. The bar is codex's own six points
   (atomic ClaimNext w/ lease-token fencing; ingest idempotency + per-sender rate limits;
   update-in-place escalation; circuit breaker; file-router writes disabled/dual-read-only;
   the five race tests). **The worker stays quarantined until that bar is verified met** —
   two runaway incidents (19k sessions; 11,564-item flood) came from re-arming early.
   Context softener: #162–#168 all merged via claude-pantheon's tracked incident-canon-phase2
   lane (see their continuation row), so the merge wasn't rogue — but the owner-directed
   "bounce until DEFENSIBLE" convergence was never recorded, and that's what this closes.

2. **Session-reaper supervisor duty** — proposal `20260708-154507-claude-home-claude-pantheon-...`
   open in claude-pantheon's queue. Root cause it addresses: every ScheduleWakeup leaked a
   full `claude.app` process (26 accumulated → crash). Review source-deep when they route the
   build back; the discriminators that matter are in the proposal (group by --resume UUID,
   ancestry-exclude, ADR-022 recycled-PID guard, surface-don't-kill ambiguous).

3. **Owner-gated (surfaced repeatedly, awaiting owner only):**
   - Assiduous prod `STRIPE_API_KEY` is `sk_test_` — **revenue-blocking**; needs the owner's
     live key in Secret Manager (item `20260705-041247-...to-user`).
   - SirsiNexusApp deploy CI red ~3mo — one gcloud IAM grant
     (`storage.buckets.get` for `github-action-1021883617@` on
     `run-sources-sirsi-nexus-live-us-east4`; item `20260703-002632-...to-user`).

4. **Unverified proposal** — the Ma'at pre-push fast-path for branch-deletion pushes
   (proposed 2026-07-03, item `20260703-013628-...`) was accepted in spirit but I never
   confirmed it landed in `.githooks/pre-push`. Check `NO_CONTENT_PUSH` exists there; if
   not, re-nudge claude-pantheon.

## THE TICK PROTOCOL (hard-won — do not regress)

Each loop tick, in order:
1. `sirsi thread heartbeat --thread <current thr-id from the SessionStart hook>` — **in-band,
   never a nohup/sidecar watcher** (sidecars were a misreading; they leak and read loop-dead).
2. Reap unambiguous same-session duplicates: kill `claude.app` processes whose args contain
   YOUR session UUID but whose pid is outside your own ancestry — **exclude your pid AND your
   parent pid** (the CLI runs as a parent+child pair; the parent is not a duplicate).
3. Count total claude.app processes; **>10 → surface to owner, never auto-kill** other
   sessions (distinguish by `--resume <uuid>`; other UUIDs are the owner's live windows).
4. `sirsi router pull claude-home` AND check the Result sections of items YOU sent that are
   now closed (the codex reply mechanic) — pull alone misses half the replies.
5. Thread id CHANGES on every session restart — always take it from the current hook, never
   cache it.

## STANDING DISCIPLINES (each earned by a real failure this thread)

- **Verify before claim.** Re-check actual state (gh pr view / re-hash artifacts / read
  `origin/main` via `git show`, never the local checkout — stale-checkout trap fired 3×).
  I falsely claimed #55 merged when it had conflicted; corrected same-turn. Do that.
- **Adjacent-green ≠ failing-job-works** (AGENTS.md corollary) — a green health job masked a
  3-month-dead deploy twice this week.
- **Bind discipline:** source-deep diff review, never rubber-stamp (own or peer), merges via
  `--admin` (shared SirsiMaster identity blocks cross-approve), never bind while a peer's
  adversarial review is in flight (the #147 lesson — premature merge, reverted via #154,
  re-landed correctly as #155).
- **Two asks with no action → act directly or demand a reason; never a third ask** (gitleaks
  gate on porch-and-alley was built directly after two ignored asks).
- **Local-model calls go through `sirsi gemma` / the broker only** — ADR-031-C; no raw
  `mlx_lm.*` anywhere (caused the swap-thrash incident).
- **Escalations must be idempotency-keyed** — the 11.5k flood was one stuck item × 60s
  no-marker retries.

## FLEET SNAPSHOT AT ARCHIVE (2026-07-08 last full check)

All six repos (sirsi-pantheon, FinalWishes, SirsiNexusApp, Assiduous, homebrew-tools,
porch-and-alley): **0 open PRs, main CI green**. FinalWishes e2e-nightly genuinely fixed
(5-layer forensic, first-ever greens holding on schedule). Gitleaks gates armed portfolio-wide.
Dependabot configs scoped per-directory (fixed 24h-timeout hangs in 2 repos). ~150 stale
branches deleted across the portfolio. Canon landed: ADR-031-C + case-study §6 + BUILD_LOG
2026-07-03 entry; ADR-051 (public RSVP) shipped as PASSed with kill-switch-in-v1.

## KEY MEMORY FILES (auto-load; verify still current before relying on)

- `reference_schedulewakeup_process_leak.md` — the tick protocol's origin story
- `reference_codex_review_reply_mechanic.md` — why pull misses codex verdicts
- `feedback_pid_alive_is_not_kill_evidence.md` — recycled-PID guard before any kill
- ADR-031-A/B/C + `docs/case-studies/2026-06-18-pantheon-did-not-prevent-oom.md` §6
