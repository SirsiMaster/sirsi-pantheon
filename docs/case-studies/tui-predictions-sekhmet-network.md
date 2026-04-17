# Case Study 020 — From Memorization to Prediction: Making the TUI Usable

**Date**: April 5, 2026
**Scope**: TUI input system + network security hardening
**What changed**: Users no longer need to memorize Pantheon's 15 deities, 40+ subcommands, and 80+ flags. The TUI predicts what they want as they type. A new Sekhmet network audit protects users on public WiFi.
**Version**: v0.13.0

---

## The Problem

Pantheon had 15 deities, each with subcommands and flags. A user who wanted to deploy a scope had to know: `ra deploy --scope assiduous --dry-run`. A user who wanted to check their hardware had to know: `hapi profile`. Nothing in the TUI told them what was available.

The TUI launched a beautiful deity roster grid. Gold glyphs, active indicators, a universal input bar. But the input bar was a blank prompt with a placeholder that said "scan my dev environment for ghost processes." If you didn't already know the command vocabulary, you couldn't use it.

This is the gap between a demo and a product. A demo impresses. A product serves.

The second problem was travel security. The founder was working from public WiFi — hotel lobbies, airports, coffee shops. Claude Code sends conversation context to `api.anthropic.com` over TLS 1.3, which is secure. But DNS was unencrypted (ISP default, spoofable), the macOS firewall was off, and no VPN was active. A motivated attacker on the same network could see DNS queries, attempt MITM with rogue certificates, or probe open ports.

---

## The Grid Overflow (A Detour Worth Documenting)

Before building predictions, we had to fix the grid. The deity roster rendered in a 3-column layout, but "Agent Orchestrator" (Ra's role) was 18 characters — 2 more than the column budget. The role text wrapped onto a second line, breaking the entire grid alignment.

Three attempts to fix it:

1. **lipgloss `MaxWidth()`** — Truncates content instead of wrapping. Didn't work. Egyptian hieroglyphs (U+13000–U+1342F) have unpredictable terminal widths. lipgloss uses `go-runewidth` to measure, but these characters are categorized as narrow (1 cell) while most terminals render them as wide (2 cells). The width budget was always wrong.

2. **Shorter role names** — "Agent Orchestrator" → "Orchestrator". Worked visually but dodged the underlying bug. If we added a new deity with a longer name, it would break again.

3. **Manual measure-then-pad** — Render each cell part (dot, glyph, name, role) with colors but no width constraints. Measure the styled string with `lipgloss.Width()`. Pad with real space characters to the target width. The key insight: if `lipgloss.Width()` miscounts the glyph width, the padding error matches the measurement error, and the columns still align. The error model is self-consistent.

This third approach is permanent. It works regardless of which terminal renders the glyphs, because the measurement and the padding use the same (possibly wrong) width model.

---

## Fish-Shell Predictions

The inline prediction system has one function that does all the work: `buildSuggestions(input string, history []string) []string`.

It parses the current input into tokens and returns full command strings that the bubbletea textinput component prefix-matches against. The component renders the untyped suffix as dim ghost text after the cursor — identical to fish shell's autosuggestions.

The prediction tiers:

| Input state | Suggestions |
|---|---|
| Partial first word (`ra`, `an`, `se`) | All deity names, aliases, intent keywords |
| Deity + partial subcommand (`ra d`) | That deity's subcommands (`ra deploy`, `ra dashboard`) |
| Deity + subcommand + partial flag (`ra deploy --d`) | That subcommand's flags (`ra deploy --dry-run`) |
| Any prefix matching a previous command | History entries (most recent first) |

The command tree is a static `map[string]deityCommands` with 15 entries covering all subcommands and flags. It's roughly 100 lines of data, compiled into the binary. No runtime introspection, no cobra reflection, no network calls.

Key binding changes:
- **Right-arrow** accepts the ghost text prediction (not Tab — the user explicitly requested this, matching fish shell muscle memory)
- **Up-arrow** recalls previous commands from session history
- When the cursor is mid-input (not at end), right-arrow moves the cursor normally — it only accepts when at the end

The edge case that matters: `AcceptSuggestion` is checked before `CharacterForward` in the bubbletea input handler. When the cursor is at the end of input and the user presses right-arrow, the suggestion is accepted and `CursorEnd()` is called. The subsequent `CharacterForward` finds `pos == len(value)` and is a no-op. When the cursor is mid-input, we temporarily unbind `AcceptSuggestion`, forward the key event, then rebind.

---

## Sekhmet Network Audit

The network audit runs six checks in ~130ms:

| Check | Method | What it catches |
|---|---|---|
| DNS Configuration | `networksetup -getdnsservers Wi-Fi` | ISP defaults (unencrypted, spoofable) |
| WiFi Security | `networksetup -getairportnetwork` + known networks plist | Open networks with no encryption |
| TLS Verification | Go `crypto/tls` dial to `api.anthropic.com:443` | Downgrade attacks, certificate failures |
| CA Certificate Audit | `security find-certificate` on system keychain | Rogue certificates (>200 = suspicious) |
| VPN Status | `ifconfig` utun interface scan | Missing VPN on public networks |
| Firewall | `socketfilterfw --getglobalstate` | Disabled firewall accepting inbound connections |

The `--fix` flag auto-applies two safe remediations: setting DNS to Cloudflare (1.1.1.1) and enabling the macOS firewall. Both require admin privileges; the command reports what it couldn't fix and tells the user the exact sudo command to run.

An early version used `system_profiler SPAirPortDataType` for WiFi info. It hung for 10+ seconds in some environments — the command scans all network interfaces and historical data. Replaced with `networksetup -getairportnetwork en0` (instant) plus the known-networks plist for security type.

The scoring model reuses the existing `DiagnosticFinding` and `calculateScore()` from Doctor. Each CRITICAL finding costs 20 points, each WARN costs 10, each INFO costs 2. Starting from 100, the founder's machine scored 48/100 before fixes and 88/100 after.

---

## The Build Path

| Step | What | Time |
|---|---|---|
| 1 | Fix grid overflow (3 attempts) | ~20 min |
| 2 | Create `suggestions.go` — static command tree + `buildSuggestions()` | ~5 min |
| 3 | Create `network.go` — six network security checks | ~10 min |
| 4 | Wire `sekhmet` parent command + `network` subcommand into cobra | ~5 min |
| 5 | Modify `tui.go` — enable predictions, rebind keys, add history | ~10 min |
| 6 | Fix `system_profiler` hang, gofmt, rebuild, test, push | ~10 min |

Total: ~60 minutes from first grid fix to all features pushed and passing Ma'at's pre-push gate.

---

## What We Learned

1. **Unicode width in terminals is not a solved problem.** Libraries like `go-runewidth` maintain tables of character widths, but Supplementary Multilingual Plane characters (like Egyptian hieroglyphs) are often miscategorized. The only reliable approach for grid layout is to use the same measurement function for both sizing and padding, so errors cancel out.

2. **Built-in components are often enough.** The bubbles `textinput` already had `SetSuggestions`, `ShowSuggestions`, and `CompletionStyle` in v1.0.0. We almost built a custom suggestion overlay before discovering the native API. The entire prediction feature required zero new dependencies.

3. **System commands lie about their speed.** `system_profiler SPAirPortDataType` is documented as a simple query. In practice it can hang for 10+ seconds. Any command called from a TUI or interactive tool needs a timeout or a faster alternative. `networksetup` does the same job in milliseconds.

4. **Security hardening is a product feature, not a deployment concern.** A CLI tool that sends conversation context over the network has a responsibility to help users verify their network security. `sirsi sekhmet network` makes this a 130ms check instead of a manual audit that most users would never do.

---

## Files Changed

| File | Lines | Change |
|---|---|---|
| `internal/output/suggestions.go` | +165 | New: command tree + buildSuggestions() |
| `internal/guard/network.go` | +230 | New: six network security checks |
| `internal/output/tui.go` | +75 | Predictions, history, key rebindings |
| `cmd/pantheon/main.go` | +95 | Sekhmet command + network subcommand |
| `CHANGELOG.md` | +15 | v0.13.0 release notes |
| `VERSION` | 1 | 0.12.4 → 0.13.0 |
