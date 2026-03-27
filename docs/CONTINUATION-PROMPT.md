# Session 29 Continuation Prompt: Thoth Auto-Sync + CI Green

## 🕵️ Context: v0.7.0-alpha (Post-Recovery)
Session 28 recovered 3 lost sessions, fixed CI (Windows CGO_ENABLED, lint, test timeouts), and published Case Study 014 ("Ghost Transcripts"). The CRITICAL finding: Antigravity IDE **never writes conversation transcripts** — `overview.txt` doesn't exist across 90+ conversations.

---

## 🚀 P0: Verify CI is Green
- Push from Session 28 should trigger CI. Verify:
  - ✅ Lint passes (20+ fixes applied)
  - ✅ Windows build passes (`CGO_ENABLED` via env block)
  - ✅ Windows tests pass (`-short` flag skips live syscalls)
  - ✅ macOS + Ubuntu tests pass
- If CI still fails: check `gh run view <id> --log-failed`

## 🚀 P1: Wire `thoth sync` — Auto-Journal from Git
- `internal/thoth/sync.go` (171 lines) exists but isn't wired
- Goal: `thoth sync` harvests recent git commits and auto-generates journal entries
- This closes the "ghost transcript" gap — even if the IDE loses conversations, git commits become journal entries automatically
- Test: run `thoth sync` → verify journal.md gets new entries

## 🚀 P2: Deploy Updated Site
- Case Study 014 needs to be on `pantheon.sirsi.ai`
- Updated build-log.html with Session 28 entry
- `firebase deploy --only hosting`

## 🚀 P3: Upgrade gh CLI
```bash
brew upgrade gh  # 2.87.3 → 2.89.0
```

---

## 🛠️ Operational Reminders
1. **Rule A19**: ABSOLUTE PROHIBITION — never modify `/Applications/*.app/` bundles
2. **Rule A20**: SirsiMaster Chrome profile for all publishing
3. **CI is fragile**: Tests hit live syscalls without `-short`. Always check CI after push.
4. **Conversation logs don't exist**: Antigravity IDE never writes them. Rely on Git + Thoth + journal.

## Context Pointers
- **Case Study 014**: `docs/case-studies/session-28-ghost-transcripts.md`
- **Thoth Sync**: `internal/thoth/sync.go` (171 lines, unwired)
- **CI Config**: `.github/workflows/ci.yml`
- **Journal**: `.thoth/journal.md` (Entries 001-024, 022-024 reconstructed)
