# 𓂀 Pantheon — Continuation Prompt
# Read this FIRST in a new session. Then read `.thoth/memory.yaml`.
# Last updated: 2026-03-26T22:18:00-04:00

---

## Session 23 — Priorities

### P0: Verify Session 22 Fixes
1. **Restart Antigravity IDE** — Cmd+Q → reopen from Applications.
2. Open Running Extensions and confirm:
   - AG Monitor Pro is GONE
   - Pantheon shows **v0.6.0** (not 0.5.0), no warnings
   - Git extension: no `title` warning
   - Antigravity extension: no missing command warning
3. Run `Cmd+Shift+P` → **"Thoth Accountability Report"** → verify webview opens.
4. Check status bar for `$(bookmark)` savings indicator.
5. If all pass → update case study status to "Verified."
6. If Gatekeeper blocks again → run recovery: `xattr -cr /Applications/Antigravity.app && codesign --force --deep --sign - /Applications/Antigravity.app`

### P1: OpenVSX Publishing
- Publish `sirsi-pantheon-0.6.0.vsix` to OpenVSX (open-vsx.org).
- Requires API token from `SirsiMaster` account.
- After publish: install from marketplace instead of sideloading.

### P2: Performance Audit
- Profile the Thoth Accountability Engine's workspace walk time.
- Target: <500ms for workspace benchmark on cold start.
- If slow: implement async walk with progress indicator.

### P3: Guardian Extension Health
- Guardian should warn about slow extensions in the status bar.
- Pull extension profile data from the Extension Host.
- Auto-suggest disabling extensions with >500ms profile time.

---

## Context Pointers
- **Thoth Accountability Engine**: `extensions/vscode/src/thothAccountability.ts` (645 lines)
- **Extension entry point**: `extensions/vscode/src/extension.ts`
- **Commands**: `extensions/vscode/src/commands.ts` (8 commands registered)
- **Package manifest**: `extensions/vscode/package.json` (v0.6.0)
- **Case study**: `docs/case-studies/session-22-accountability-engine.md` (pending verification)
- **Journal**: `.thoth/journal.md` (Entry 019 — "Give Thoth his receipts")
- **Gatekeeper fix**: `xattr -cr + codesign --force --deep --sign -`

## Extension Sideload Location
- Antigravity: `~/Desktop/.antigravity/extensions/sirsimaster.sirsi-pantheon-0.6.0/`
- Registry: `~/Desktop/.antigravity/extensions/extensions.json`
- Disabled: `~/Desktop/.antigravity/extensions/shivangtanwar.ag-monitor-pro-1.0.0.disabled/`
- Git patch: `/Applications/Antigravity.app/Contents/Resources/app/extensions/git/package.json`
- AG patch: `/Applications/Antigravity.app/Contents/Resources/app/extensions/antigravity/package.json`

## Session 22 Stats
- **Files created**: 1 (thothAccountability.ts — 645 lines)
- **Files modified**: 4 (extension.ts, commands.ts, package.json, statusBar.ts)
- **External patches**: 4 (AG Monitor, Pantheon sideload, Git title, Antigravity commands)
- **Canon updated**: VERSION, CHANGELOG, memory.yaml, journal.md, case study
- **Version**: 0.5.1-alpha → **0.6.0-alpha**
- **VSIX size**: 49.47 KB (13 files)
