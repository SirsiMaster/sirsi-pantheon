---
from: "claude-home"
to: "user"
title: "USER VERIFY (after PR #26 merges): the FDA re-prompt fix — grant once → reinstall → confirm no re-prompt"
type: "decision"
status: closed
opened: 2026-06-09T05:37:00Z
closed: 2026-06-17T02:17:34Z
---

## Instructions

Your complaint — "im tired of approving these [FDA/TCC] continually… i do this for no
other even when i reinstall" — is **fixed in code** (PR #26), and there's one ~2-min
verification only you can do (on the real machine, after the PR lands).

## What was wrong (2 compounding causes)
macOS TCC re-prompted for Full Disk Access on every menubar reinstall because the
menubar wasn't a stable TCC "subject":
- **Cause 1** (fixed PR #17): the ad-hoc signature used a content-hash identifier →
  new identity every rebuild.
- **Cause 2** (fixed PR #26): the menubar was a bare Mach-O binary with no bundle id,
  so TCC keyed FDA on the content hash → re-prompt every rebuild. Now it installs as a
  real `.app` (`~/Applications/Sirsi Menubar.app`) with a FIXED
  `CFBundleIdentifier=ai.sirsi.pantheon`. TCC keys the grant on that stable id → one
  grant survives reinstalls. (Chrome/Slack/Bartender work this way — stable bundle id.)

## What you need to do (after PR #26 merges + installs)
1. `sirsi setup` (or the menubar install step) — installs the new `.app` bundle.
2. Grant Full Disk Access ONCE (System Settings ▸ Privacy & Security ▸ Full Disk
   Access → add `~/Applications/Sirsi Menubar.app`).
3. Reinstall the menubar again (rebuild + `sirsi setup`).
4. **Confirm: NO new FDA re-prompt.** If the grant persisted → fixed, close this. If it
   re-prompts → note it; the menubar lane (claude-pantheon) reopens cause-3.

This operator reinstall-verify is the acceptance gate — agents deliberately did NOT
mutate your live install to "test" it (Menubar-QA: test the real flow; don't touch the
user's machine without you).

Ref: PR #26 (.app bundle, stable CFBundleIdentifier) + #17 (stable identifier),
050932 (your TCC report), 053458, A19 (writes only ~/Applications, never /Applications).

## Result

CLOSED — owner reports COMPLETED 2026-06-17: granted FDA once, reinstalled, confirmed NO re-prompt. Acceptance gate satisfied.
