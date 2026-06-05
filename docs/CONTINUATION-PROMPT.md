# Continuation — Pantheon canonical-setup wizard + open safety items

## Resume name: "Pantheon setup wizard"

## DONE this session (committed/pushed)
- `sirsi fix` heuristic resolver — answers every finding; safe PPID-narrowed
  orphan-kill (`guard.KillTrueOrphans`, PPID<=1 only, `--yes` never kills, 4
  regression tests). Commits b9d86ea…42588a9.
- Menubar zsh fix: `read -n 1 -s -r` (bash) → portable `read _` (baad20c) — was
  the recurring `zsh: not an identifier: -s`.
- This machine canonicalized: ONE `~/.local/bin/sirsi` (v0.22.0-beta, stable
  self-signed "Sirsi Pantheon Code Signing"), homebrew dupe removed, zsh
  completion installed to /opt/homebrew/share/zsh/site-functions/_sirsi.

## OPEN — do these next
1. **Build the canonical install WIZARD** (user ask: "wizard them during install").
   Enhance `cmd/sirsi/setup.go` (`sirsi setup`) into a guided first-run wizard:
   - Detect DUPLICATE `sirsi` binaries on PATH + version DRIFT (`which -a`,
     compare versions) → offer to consolidate to ONE canonical (Homebrew is the
     product canonical per CLAUDE.md §3; `~/.local/bin` for source builds).
   - Verify install dir is on PATH; if not, add ONE idempotent marked line to the
     correct rc file (zsh→.zshrc, bash→.bash_profile). Never duplicate.
   - Install shell completions for the DETECTED shell (`sirsi completion zsh|bash`
     → fpath dir). zsh completion WORKS (212 lines) — just wire it.
   - Keep existing deps + FDA + agent-registration checks; FDA stays guided
     (macOS forbids self-grant).
   - End with a clean summary of the canonical state.
   - A16-injectable side effects + tests. codex re-review (A12) before land.
2. **codex orphan-kill near-pass gap**: `ScanOrphans` doesn't populate
   `ProcessInfo.User`, so root/system protection can't fire in `KillTrueOrphans`.
   Fix: populate User in the ScanOrphans ps path (orphan.go `defaultOrphanPs` /
   `orphanPsEntry`), so isProtectedProcessWith's root check works. Then codex
   passes → unblock the diagnose→fix + menubar funnel (still BLOCKED).
3. **mds_stores storm**: user must run `sudo mdutil -i off -d ~/Development`
   (Spotlight write-amplification from agent file bursts). Agent can't sudo.

## Identity note
This session's thread is mislabeled `claude-home` but is functionally the
claude-pantheon resolver lane (authored fix.go/guard). thr-fb73 is the separate
pantheon ops/ctr-drain watcher. Reconcile to one pantheon owner.
