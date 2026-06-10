# ADR-028: Optional SQLite — `nosqlite` Lean Build Variant

## Status
**Proposed** — 2026-06-08 (claude-pantheon, codex-OOO window). Design-only; final review held for real codex on return (~06-10); cross-eyes routed to a non-standin claude. Not implemented in this ADR.

## Context
After the deity-binary removal (PR #7, −14 MB) and the dead push-model router-cluster deletion (PR #8, −2,626 LOC), the **release `sirsi` is 15 MB stripped, pure-Go** (`CGO_ENABLED=0`, the goreleaser default) — back at its historical "<15 MB" bar. Measurement of the remaining size levers (router item `175145`):

- **Metal/CoreML cgo gate: non-win.** The release already builds `CGO_ENABLED=0`, so the `.m` bridges are already excluded. CGO=0 and CGO=1 both measure 15 MB stripped. Dropped as a lever.
- **`modernc.org/sqlite` is the one real lever: ~4.4 MB.** Confirmed linked (4,346 symbols in the `CGO_ENABLED=0` build). Pure-Go SQLite + a C-to-Go-translated libc — the single largest dependency in `sirsi`.

`modernc.org/sqlite` is imported by exactly **three** packages:
- `internal/vault` — SQLite **FTS5** context sandbox + **BM25** code search (`vault.go`, `codeindex.go`). The core consumer; this is a real product feature.
- `internal/seshat` — reads Chrome's on-disk SQLite history/bookmarks DB (`adapter_chrome.go`) for the knowledge bridge.
- `internal/notify` — the notification store (`store.go`).

The naive "remove sqlite for −4.4 MB" is **not free**: it disables Vault search, the Seshat Chrome adapter, and the notify store. That trades *features* for size on a binary already at its target. So the decision is not "remove sqlite" but "make it **optional** without losing it by default."

## Decision
Keep SQLite (and full Vault/Seshat/notify) in the **default** build. Add a `nosqlite` **build tag** that produces a lean (~10.6 MB) `sirsi` for size-constrained / headless deployments, with **graceful degradation** in the three dependent packages.

1. **Default = full.** No build tag → sqlite linked, all features present, 15 MB. The shipped release stays full-featured.
2. **`nosqlite` opt-in lean variant.** `go build -tags nosqlite` (and a goreleaser variant if demand warrants) → sqlite excluded, ~10.6 MB.
3. **Build-tag split, not runtime detection.** Each sqlite-dependent package gets two files:
   - `<feature>_sqlite.go` (`//go:build !nosqlite`) — the real implementation.
   - `<feature>_nosqlite.go` (`//go:build nosqlite`) — a stub with the **same exported surface** that **degrades gracefully**: constructors return a clearly-labeled "compiled without SQLite" sentinel error; query/store methods are safe no-ops or return that sentinel. **Never panic, never silently succeed** (Rule A1 fail-visible, not fail-loud-crash).
4. **Every dependent package documents its absent-sqlite behavior** at the stub site (what the caller sees, how callers should branch). Per claude-home advisory verdict (`175145`): graceful-degrade over fail-loud; build-tag opt-in over runtime detect; explicit per-package code path.
5. **CLI honesty.** Commands that need an absent feature (`sirsi vault search` in a `nosqlite` build) print a clear "this build was compiled without SQLite — use the default build for Vault search" message, not a stack trace.

## Alternatives Considered
- **Remove sqlite outright (−4.4 MB, lose Vault).** Rejected — trades a real feature for size on an already-lean binary. Not honest to call it "cleanup."
- **Swap modernc/sqlite for bbolt or a pure-Go FTS.** Rejected for now — Vault's value is FTS5 + BM25 ranking; reimplementing that is a large project, not a build-config change. Revisit only if `nosqlite` adoption shows the lean variant is the common case.
- **Runtime detection (load sqlite dynamically).** Rejected — modernc sqlite is statically linked Go; there is no clean runtime-optional path, and it would keep the 4.4 MB in the binary anyway.
- **Do nothing (keep 15 MB).** Viable — 15 MB is at the historical bar. `nosqlite` is additive optionality, not a mandate; if no deploy needs <15 MB, this stays Proposed.

## Consequences
- **Positive:** opt-in ~10.6 MB `sirsi` for headless/size-constrained use; default stays full-featured; clean compile-time separation; no runtime cost.
- **Cost:** two files per dependent package (3 packages → ~3 stub files) + the CLI guard messages; a `nosqlite` CI matrix entry to keep the lean build compiling; test coverage for both tags.
- **Risk:** stub surface must exactly match the real one or the lean build breaks — caught by a `nosqlite` CI build. Graceful-degrade semantics must be verified per package (test under `-tags nosqlite`).
- **Reversible:** pure build-config + stubs; deleting the tag restores the status quo.

## References
- Size measurements: router item `20260608-175145` (release 15 MB CGO=0; sqlite 4,346 symbols; Metal non-win).
- Design advisory: claude-home `20260608-222434` (graceful-degrade, build-tag opt-in, per-package documented path).
- Prior footprint work: PR #7 (deity binaries, −14 MB), PR #8 (router cluster, −2,626 LOC).
- Rules: A14 (verifiable numbers), A1 (fail-visible), Rule 0 (minimal code), ADR-005 (Pantheon is the product).
- Owners: codex-pantheon (footprint/source lane — final review on return); claude-pantheon (drafted). Vault is the hard dependency to design around.
