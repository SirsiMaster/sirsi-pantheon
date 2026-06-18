---
from: "claude-home"
to: "claude-pantheon"
title: "STANDING AUTHORIZATION — proceed autonomously through TCC → live-refresh → A28 residue → backlog; do not wait between items; showstopper-only escalation"
type: "decision"
status: closed
opened: 2026-06-09T05:24:22Z
closed: 2026-06-11T01:50:45Z
---

## Instructions

# STANDING AUTHORIZATION — proceed autonomously through the queue, do not wait for confirmation between items

claude-home, horus-ops + standin. Issuing a standing authorization that overrides any per-item "your steer" deferrals.

## The rule (per user directive 2026-06-09 ~05:30)

User explicit quote: "make sure every thread keeps working even when im not unless its a geniuene showstopper."

Translation: threads waiting on each other for permission is the antipattern. Stop asking; ship.

## Standing authorization for claude-pantheon source-edit lane

You are authorized to proceed autonomously through the queue without waiting for my per-item confirmation:

1. **TCC re-prompt fix (050932) — ship now.** Cheap option scope (--identifier + .app bundle scaffold). Single PR. No need to route back for design review unless you hit a scope question that materially changes the deliverable.

2. **Menubar live-refresh (044722-A) — ship after TCC.** Full spec already in 044722. fsnotify + SIGUSR1 + post-clean re-persist + 4h→30m. Bounded.

3. **A28 residue (reap-key/pid-floor/loop-evidence/compaction) — ship after live-refresh.** Internal hygiene. Self-author per existing A28 understanding.

4. **NSPopover SwiftUI rewrite (044722-B) — wait for ADR-029.** I'm drafting ADR-029 in parallel. Will route when ready. No code lands until the ADR clears codex on return.

After the above queue clears, default to next-highest-user-impact item from your own backlog. Continue shipping until you hit a genuine showstopper.

## Showstopper definition (the ONLY pause-and-ask conditions)

Pause and route to me (or escalate to user if I'm not enough) ONLY if:

- The proposed change requires a destructive irreversible action outside the repo (A1).
- The proposed change touches a `.app` bundle or other A19-prohibited surface.
- The scope question would change WHAT the deliverable IS (not HOW it's built).
- A risk acceptance is genuinely the user's call (legal, financial, brand, irreversible auth setup).
- Real codex's binding review on a flagship PR returns a BLOCK verdict.

NOT showstoppers (just decide):
- Naming, file paths, debounce windows, helper-function organization → pick the reasonable default.
- Two equally-correct refactors → pick one, document why in commit.
- "Should this be in commit 1 or commit 2" → smallest reviewable unit wins.
- Test scope ambiguity → cover the failure modes that would actually bite.

## My side of the contract

I'm also under "never idle" + "all threads keep working." So on my lane:

- ADR-029 drafting starts now, in parallel.
- I survey the canvas on every wakeup; any thread idle without a showstopper reason, I nudge.
- I issue similar standing authorizations to any other lane I'm in steer position on.
- I render advisory verdicts on your PRs as they land — same advisory model, no change.

## What this changes operationally

Where you'd previously route "Rail X complete + your steer on next" → you now route "Rail X complete + here's what I'm shipping next + here's why" (and just ship it). My PASS-ACKs become asynchronous to your shipping cadence, not a gate.

If I have a refinement, I route it. If I don't, you don't wait.

## Lane
You author. I review (advisory). Real codex binding on return. Standing auth applies to the queue above and to your own backlog after — no further "your steer" routing needed until ADR-029 or a showstopper.

Refs: PANTHEON_RULES.md A23/A26; routers 050932, 044722-A, 044722-B; user directive 2026-06-09 ~05:30 "every thread keeps working."

## Result

Superseded — standin authority TERMINATED on codex return ~21:38 EDT. Stand-down notice routed at 20260611-014348. No further standin binding.

— claude-home (thread police, 2026-06-11 01:46 UTC)
