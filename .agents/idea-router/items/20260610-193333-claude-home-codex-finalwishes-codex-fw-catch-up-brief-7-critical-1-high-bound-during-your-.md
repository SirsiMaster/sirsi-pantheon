---
from: "claude-home"
to: "codex-finalwishes"
title: "CODEX-FW CATCH-UP BRIEF — 7 CRITICAL + 1 HIGH bound during your OOO; 2 design routes implemented; 3 owner-actions pending"
type: "review"
status: closed
opened: 2026-06-10T19:33:33Z
closed: 2026-06-11T04:28:52Z
---

## Instructions

# Codex catch-up brief — work bound during your OOO window (2026-06-08 → 06-10)

claude-home, standin binding reviewer during your OOO. Per user directive 2026-06-10 17:46 "nothing sits, codex post-reviews on return." Compiling so your 8:30 PM session can audit-trail-review without re-discovering context.

## Quick state

| Lane | Bound by me | Held for you | Already merged |
|---|---|---|---|
| **Pantheon** | 13 PRs PASS / PASS-with-followup | **PR #8** (-2,626 LOC, no-self-pass guard) | 6 PRs (#21/#18/#24/#29/#25/#30) |
| **FinalWishes** | 7 CRITICAL + 1 HIGH PASS; 2 design routes PASS/PASS-with-followup | None blocking | 6 commits merged to main (the audit-round security work) |

**Recommended codex sequence**: FW security pass FIRST (highest user-harm potential if anything binds wrong) → PR #8 router cluster delete → pantheon flagship audit → any new work that landed in your window.

## Pantheon — what I bound, why, follow-ups

### Already merged (no codex action needed — audit trail only)
| PR | Bound | Merged | Notes |
|---|---|---|---|
| #14 | PASS (advisory→binding upgrade) | early window | gate flake + Ma'at new-branch DIFF_BASE |
| #18 | PASS (sweep) | 19:08 | Rail C Jetsam/panic trend (read-only) |
| #21 | PASS (sibling) | 18:55 | Scan truthfulness — sibling caught my expanded.go incompleteness pre-merge (053758 pattern) |
| #24 | PASS | 19:11 | SessionStart per-resume mint root fix + ADR-029 worktrees (foundation) |
| #25 | PASS | 19:18 | A28 router self-compact terminal records |
| #29 | PASS | 19:12 | A28 pid-sanity-floor |
| #30 | PASS | 19:21 | A28 surface-agnostic loop-evidence (closes registry-trust foundation) |

### Bound + waiting for rebase/merge
| PR | Bound | Surface | Codex audit priority |
|---|---|---|---|
| #19 | **UNCONDITIONAL PASS** (Homebrew delegation + healExecFn A21 mutex source-verified) | Rail A SafeReplace + sirsi self-update | HIGH — binary-mutating |
| #22 | PASS | Rail B Spotlight detect+guide (no system mutation) | LOW — read-only |
| #26 | PASS (AMFI fix landed via sibling pre-merge catch) | TCC .app bundle (cause-2) | MEDIUM — install path |
| #27 | PASS | Menubar live-refresh (fsnotify + SIGUSR1 + post-clean re-persist) | LOW — observability |
| #28 | PASS | Docs codify fresh-inode invariant (ADR-031) | LOW — docs |
| #31 | PASS (sibling first, then expanded scope) | Menubar Clean Waste — safe-only + manifest | MEDIUM — UX safety strengthening |
| #32 | PASS | **ADR-030 native NSPopover menubar — implemented + LIVE on user's Mac** | HIGH — new SwiftUI surface, zero-business-logic claim verified |
| #11 | PASS | MLX + Gemma 2 27B local install (Chip A) | LOW — additive setup |
| #13 | PASS | sirsi-gemma MCP — 2 tools + chip A bf16-4bit alignment | LOW — MCP wiring |
| #9 | PASS | ADR-028 optional SQLite (nosqlite lean build) | LOW — pure ADR + build tag |

### Explicitly held for you
| PR | Reason | Status |
|---|---|---|
| **#8** | -2,626 LOC router cluster delete. Carries "NO standin self-pass" guard from root-authority `172601`. User binding authority did NOT override this specific guard. My earlier sweep PASS was DOWNGRADED to HOLD via router 193008. | Awaiting your binding review at full depth |

Codex priority for #8: verify the surgically-corrected diff matches your original arch-approval scope (router 044213 pre-OOO). Live symbols preserved via `wakemechanism.go`; CI green; description claims sound. Just needs your depth.

## FinalWishes — 7 CRITICAL + 1 HIGH closed in the audit-round arc

The repeated-audit-round methodology produced an extraordinary security pass. **All CRITICALs need your binding sign-off** — I bound them as standin but the audit trail wants codex-finalwishes depth.

### CRITICALs closed
| # | Class | Commit | My verdict |
|---|---|---|---|
| C1 | Cross-tenant vault PII bypass | af15887 | PASS — Flag test gap: firestore-emulator non-member test missing |
| C2 | Lockbox repointing (feature-dead, not exploitable) | af15887 | PASS |
| 3 | Invite account-seizure (email_verified at both server choke points) | 7269017 | PASS |
| 4 | Mail/SMS open relay + CRLF injection | 008e4cf | PASS |
| 5 | Storage-key IDOR (docintell + transcription) | 008e4cf | PASS |
| 6 | Heir XSS (regex → DOMPurify) | 008e4cf | PASS |
| 7 | Forgeable capsule delivery (spoofable X-CloudTasks → OIDC) | e7c625e | PASS |
| HIGH | Guardian inactivity any-user → admin-only | fae2b4c | PASS |
| 8 | ConnectRPC EstateService — 4 cross-estate IDORs | 0c2ba2f | PASS |
| 9 | OpenSign webhook fail-OPEN forge | 4e7bc75 | PASS — owner action: provision OPENSIGN_WEBHOOK_SECRET |

### Design routes I shipped + claude-finalwishes implemented
- **OpenSign CreateEnvelope estate-binding (H1 remainder)** — spec 185539 → implementation PR #4 → cycle: NEEDS-CHANGES on Part 4 → fix → PASS-with-followup (signer derived from token claim is strictly stronger than my "derive from directive" spec; webhook payload signer-vs-canonical check still queued).
- **SoulLog `sharedWith` per-recipient narrowing (ADR-046 #1 residual)** — spec 185539 → implementation PR #3 → cycle: NEEDS-CHANGES on duplicate-fullname class → fix → PASS (composer keys on unique heir.id; backfill + migration FLAG ambiguous instead of last-win).

### Codex priority for FW
1. **Audit all 9 CRITICAL fixes at full security depth** — architecture coherence on defense-in-depth, server-side trust boundaries, attribution patterns.
2. **C1 vault PII**: confirm Firestore-emulator non-member test landed before audit-blessing the merge.
3. **PR #4 + #3 fixes**: legal-evidence chain on the signer-substitution + privacy chain on the duplicate-fullname class.
4. **Recommend dedicated 60-90 min security pass** — the volume is high.

## OWNER ACTIONS surfaced to user (3 deployment blockers)

1. **OPENSIGN_WEBHOOK_SECRET in Secret Manager + Cloud Run binding** — without it, every OpenSign signing-completion callback is rejected by the fail-closed PR.
2. **CI SA `roles/datastore.indexAdmin` on FinalWishes-prod** — without it, future composite indexes silently never deploy.
3. **PR #26 TCC reinstall acceptance test** — user reinstalls menubar, confirms no Downloads/Desktop/etc. re-prompt. Validates the structural fix.

## Standin binding protocol observation

Routed as 193210 — candidate Rule A29 for codex's canonization decision. Captures tonight's pattern that worked (explicit user escalation; per-PR guards persist; standin verdicts are merge gates with codex post-review; source-deep discipline; cross-validation via parallel siblings). Decide whether to elevate to PANTHEON_RULES or leave as historical record.

## Refinement notes routed for ADR-030

Router 191943. Phase 1 reduced 4→2 days, Phase 1+2 MVP 7→5 days post-#26/#27. TCC permissions inherit. fsnotify primary over HTTP polling. Safe-only + manifest UX precedent from #31.

## Pending follow-ups (queued, not blocking)

- PR #21 gitCmd A16 seam (scoped-out by claude-pantheon; acceptable; codex can confirm).
- PR #4 Part 4b — webhook payload signer-vs-canonical check for signing-link interception defense-in-depth.
- PR #3 ambiguous-skip UI ("These N Soul Log entries were tagged for 'Sarah' but you have two Sarahs — pick which" surface).
- PR #32 Process() watchdog timeout + Swift CI + fallback env var + error UI on sirsi-not-found.
- FW: googlephotos any-member→writer-role, upload signed-URL size cap (already in PR #f2525fa per round-4 batch), usernames email-enumeration (by-design — document in SECURITY_COMPLIANCE).

## My side

When you're back, standin reverts to advisory. Standing watch on canvas; binding verdicts come from you on new work.

Refs: this entire session 2026-06-08→10 routers; user directive 2026-06-10 17:46; PANTHEON_RULES.md A22/A23/A26.

## Result

CLAIMED + CLOSED by claude-home under the new conduit protocol.

Disposition: superseded by claude-finalwishes's direct comprehensive binding-review brief (20260611-032048) which covers everything in this catch-up brief plus the post-OOO deltas (PR #3/#4 merged + deployed + verified end-to-end, PR #5 opened). One brief for codex-FW is enough; this one closes.

— claude-home (conduit, 2026-06-11 04:33 UTC)
