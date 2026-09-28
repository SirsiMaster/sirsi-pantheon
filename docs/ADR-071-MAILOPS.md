# ADR-067: Mail Ops — Poison-Message Detection & Archive-Only Cleanup

## Status
**Accepted** — 2026-09-25

## Context
Outlook on the M5 workstation was hanging for hours on inbound and outbound
mail (~200x/day per account). Forensics on Outlook's own IMAP sync logs
(`Osa/OutlookServiceApiLogs_*`) showed the root cause: Outlook's IMAP sync
loops on messages whose multipart body has a zero-length `text/plain` part
(old Microsoft 365 calendar replies, a copier scan). Each loop blocks that
account's IMAP connection for ~5 minutes, which also blocks Outbox
finalization and causes duplicate sends. GUI automation cannot fix this
headless — the screen lock blocks accessibility APIs — so the fix must be
API-based (Gmail API; the affected accounts are Gmail-hosted).

A working Python prototype exists at `~/.sirsi/mailops/` (`auth.py`,
`mailops.py`): OAuth Desktop client, `poison [--apply]` (detect + archive)
and `senders [N]` (census). The owner asked for this to become a governed
Pantheon feature rather than a standalone script: Ma'at-gated mutations,
dry-run by default, a signed receipt per archive batch, and — separately —
an Outlook health watcher and a headless-friendly token story.

## Decision
Add `internal/mailops` (Go, mirrors the prototype's logic against a
`Client` interface — Rule A16 injection pattern) and `sirsi mail
{poison,senders}` (`cmd/sirsi/mailopscmd.go`). Hard invariants, enforced in
the type system rather than by convention:

- `mailops.Client` has no delete, empty-trash, or send method — those
  operations are structurally impossible from this package, not just
  discouraged.
- `PoisonScan` defaults to report-only; `--apply` is required to archive,
  and every archive batch is recorded to a local-only JSONL receipt
  (`~/.sirsi/mailops/receipts/`, mirrors `internal/cleaner.DecisionLog`).
- Account registry (`~/.sirsi/mailops/accounts.yaml`, `configs/mailops.yaml`
  as the shipped example) holds only label/email/token-path — no secrets.
- OAuth tokens stay file-based (`~/.sirsi/mailops/token-<label>.json`, mode
  0600) with `oauth2.Config.TokenSource` auto-refresh; this is the same
  storage shape the Python prototype already uses, so no new secret store
  is introduced. See "Deferred" below for the 7-day Testing-mode expiry.

**Deity attribution**: extended **Anubis** (Hygiene Engine) in
`docs/DEITY_REGISTRY.md` to explicitly cover mailbox hygiene alongside
filesystem hygiene — both are "detect waste/poison, weigh against policy,
archive/purge under a dry-run gate." No new deity; Rule D1 (deity function
is universal) is satisfied by widening Anubis's existing domain rather than
reassigning it. Ma'at continues to own the quality/safety gate (Rule D3) —
unchanged by this ADR.

## Deferred (owner decision needed before build)
The router request also asked for (2) an Outlook health watcher that parses
Osa logs and flags same-UID looping or duplicate sends within 10 minutes,
and (3) a plan for the 7-day OAuth Testing-mode token expiry. Both are
scoped out of this ADR because they depend on owner input this session
doesn't have:

- **Osa log watcher**: needs a sample of the actual `OutlookServiceApiLogs_*`
  format from the M5 workstation to build a correct parser against — a
  guessed format would be exactly the kind of unscoped-claim risk Rule A35
  forbids ("we parse the logs" when we've only ever seen a paraphrase of
  them).
- **Token expiry**: the owner must choose between (a) publishing the GCP
  OAuth consent screen for `sirsi-mail-ops` internally-only for `sirsi.ai`
  (removes the 7-day Testing-mode cap, no external review needed for an
  internal-only app) or (b) domain-wide delegation via a service account
  (removes user-facing consent entirely, but requires Workspace admin
  access to sirsi.ai and grants broader mailbox scope than gmail.modify
  alone needs). This is a security-posture choice, not an implementation
  detail — Rule A32 gates it to the owner.

Both are tracked as follow-up ledger items, not silently dropped.

## Alternatives Considered
1. **Keep the Python prototype as-is, wrap it with a shell-out from Go**:
   rejected — no Ma'at gate, no receipt, no dry-run enforcement in the type
   system; violates Rule 0 (Sirsi builds assets, not disposable scripts) and
   the "never deletes" invariant would rely on code review discipline
   forever instead of an impossible-by-construction interface.
2. **New deity for mail hygiene**: rejected — Rule D1 says a deity's
   function is universal and fixed; "hygiene" already exists (Anubis).
   Splitting hygiene by data type (files vs. mail) multiplies registry
   entries without a functional reason to.
3. **Service-account domain-wide delegation now, skip the token-expiry
   question**: rejected — that's the more powerful, harder-to-reverse
   option and the owner hasn't chosen it; shipping it by default would be
   an unrequested privilege escalation.

## Consequences
- **Positive**: mailbox mutation now goes through the same dry-run/receipt
  discipline as `internal/cleaner` and `internal/guard`; testable without
  network access (fake `Client` in `mailops_test.go`); one command surface
  (`sirsi mail`) instead of a bespoke script per machine.
- **Negative**: adds `golang.org/x/oauth2` and `google.golang.org/api` as
  new go.mod dependencies (first external cloud-API dependency in this
  repo, pulling in gRPC/protobuf transitively). **Measured**: `sirsi`
  binary grew from 28.1 MB to 41.3 MB (+13.2 MB) on this build. The 25MB
  guard (Rule A6) was already exceeded before this change (28.1 MB
  baseline); this change roughly triples the overage. Not fixed in this
  PR — flagging per Rule A35 rather than understating it. If binary size
  becomes a gate, the Gmail API import is the first thing to isolate
  behind a build tag (mirrors ADR-028's `nosqlite` precedent).
- **Risk**: the Outlook-loop root cause is deferred, unverified in this
  ADR — this ships the cleanup tool, not the log watcher that would prove
  the loop stopped recurring. Do not claim the Outlook hang is fixed until
  the Osa watcher (or manual confirmation) closes that loop.

## References
- Router item `20260925-134524-claude-deck-claude-pantheon-...` (owner
  request via claude-deck)
- `docs/DEITY_REGISTRY.md` (Anubis domain extension)
- `docs/design-notes/MAILOPS_DESIGN.md`
- `internal/cleaner/decisions.go` (receipt/audit pattern precedent)
- `internal/guard/slayer.go` (Rule A16 injection pattern precedent)
- Prototype: `~/.sirsi/mailops/{auth.py,mailops.py}`
