# Mail Ops — Design Note

Router request: `20260925-134524-claude-deck-claude-pantheon-...` (owner, via
claude-deck). Full ADR: [ADR-067](../ADR-067-MAILOPS.md). This note answers
the four asks in the request directly, one section each.

## 1. A governed mail module (Ma'at-gated, dry-run default, signed receipts)

**Shipped in this PR.** `internal/mailops` + `sirsi mail {poison,senders}`.

- **Accounts**: `~/.sirsi/mailops/accounts.yaml` (label → email → token
  path, no secrets). Example shape: `configs/mailops.yaml`.
- **Poison detector**: `mailops.PoisonScan` — same empty-`text/plain`-part
  logic as the Python prototype, ported to Go against a `Client` interface.
- **Sender census**: `mailops.SenderCensus` — top-N inbox senders over the
  last year, flags `List-Unsubscribe`.
- **Archive-only apply**: `--apply` is required to mutate; every batch
  writes a JSONL receipt to `~/.sirsi/mailops/receipts/` (local-only, never
  uploaded — Rule A11).
- **Dry-run by default**: `sirsi mail poison <label>` without `--apply`
  only reports; matches every other mutating Pantheon command's contract
  (`reap-sessions`, `guard slay`, etc.).
- **Filter proposals**: not yet built — the prototype has no filter-rule
  code to port, and "owner approves filter rules before apply" (ask #4)
  implies a proposal/approval flow with no existing analog in this repo to
  follow. Flagging as a follow-up ledger item rather than inventing the
  shape now (Rule 0 — don't build ahead of a real usage pattern).

## 2. Outlook health watcher (Osa log parsing, duplicate-send detector)

**Deferred — owner decision needed, see ADR-067 "Deferred".** The request
describes the log family (`Osa/OutlookServiceApiLogs_*`) and the symptom
(same-UID looping, duplicate sends within 10 minutes) but this session has
no sample of the actual log format to parse against. Building a parser from
a paraphrase risks exactly the failure Rule A35 (Scope The Check To The
Claim) names: a check that claims "we watch Outlook's logs" while actually
matching a guessed shape that silently misses the real one.

**Next step**: owner (or a session with disk access to the M5 Outlook
container) attaches one real `OutlookServiceApiLogs_*` file excerpt showing
a poison-loop occurrence; the watcher gets built against that fixture, with
the regression test being "does it fire on this real excerpt," per Rule
A35's guidance to verify both directions before shipping a check.

## 3. Headless token handling + 7-day Testing-mode expiry

**Shipped**: token storage design (file-based, 0600, auto-refresh via
`oauth2.Config.TokenSource` — see `internal/mailops/gmail.go`). This is
headless-safe already: no login keychain dependency, works over SSH/cron.

**Deferred — owner decision**, because it's a security-posture choice:
- **(a) Publish `sirsi-mail-ops` internally for `sirsi.ai`** — removes the
  7-day Testing-mode refresh-token expiry, no external Google review needed
  for an internal-only Workspace app, keeps the `gmail.modify` +
  `gmail.settings.basic` scope as-is. Lowest-privilege option.
- **(b) Domain-wide delegation via a service account** — no per-user
  consent screen at all, but requires Workspace admin access to `sirsi.ai`
  and technically permits scope beyond what's requested unless carefully
  restricted per-API. Higher-privilege, harder to audit later.

Recommendation: (a). It solves the stated problem (7-day expiry) without
expanding what the credential can reach. (b) is the more powerful lever and
should only be taken if a future need requires it.

## 4. Hard rules — how they're enforced, not just stated

| Rule | Enforcement |
|---|---|
| Never delete | `mailops.Client` has no delete method — a caller cannot call what doesn't exist. |
| Never empty trash | OAuth scope is `gmail.modify` only; trash-empty requires a scope never requested (`auth.go`). |
| Never send mail | No send method on `Client`; scope list excludes `gmail.send` entirely. |
| Owner approves filter rules before apply | No filter-apply code shipped yet (see §1) — there's nothing to approve around until that flow exists. |

## Verification
- `go build ./internal/mailops/... ./cmd/sirsi/` — clean.
- `go test ./internal/mailops/...` — 6/6 pass, including a dry-run-never-
  mutates test and an apply-archives-only-matched test.
- Not verified: an actual Gmail account round-trip (requires the owner's
  token files, which stay local to `~/.sirsi/mailops/` per Rule A11 and were
  not read or transmitted by this session).
