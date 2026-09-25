# internal/mailops

Mailbox hygiene: poison-message detection (empty `text/plain` MIME part
that hangs Outlook's IMAP sync), archive-only cleanup, sender census.
Gmail-only today via the Gmail API. ADR-067. User guide:
`docs/user-guides/MAILOPS.md`. Design note (all 4 feature asks, deferred
items):`docs/design-notes/MAILOPS_DESIGN.md`.

## Architecture

- `mailops.go` — pure business logic (`PoisonScan`, `SenderCensus`) against
  the `Client` interface. No network, no Gmail SDK import here — this is
  what the tests exercise.
- `gmail.go` — the real `Client`, backed by `google.golang.org/api/gmail/v1`.
  Auto-refreshing OAuth token via `golang.org/x/oauth2`.
- `auth.go` / `authflow.go` — one-time interactive consent
  (`sirsi mail auth`), writes the refresh token to disk.
- `accounts.go` — YAML account registry (label → email → token path).
- `receipt.go` — local-only JSONL audit trail for every archive batch,
  mirrors `internal/cleaner.DecisionLog`'s shape.

## Interface contract (Rule A16 injection pattern)

`Client` is the sole side-effect boundary. It intentionally has **no**
delete, empty-trash, or send method — those operations are impossible to
call from this package by construction, not merely discouraged by
convention. `PoisonScan`/`SenderCensus` depend only on `Client`, so they're
fully unit-tested (`mailops_test.go`) with a `fakeClient`, no network.

## Adding a new mutating operation

If a future op needs to mutate mail (e.g. applying an owner-approved filter
rule):
1. Add the narrowest possible method to `Client` (not a generic "do
   anything" escape hatch).
2. Gate it behind an explicit `--apply`-style flag, default dry-run.
3. Record it through `ReceiptLog`.
4. Never add delete, trash-empty, or send — those are the hard rules from
   the owner's original request and are load-bearing for why this module
   is trusted with API access at all.

## Known limitations

- Gmail only; no IMAP/other-provider backend.
- No Outlook-log watcher yet (needs a real log sample — see design note).
- No filter-rule proposal/apply flow yet (no existing pattern to follow;
  build when a first real filter-rule use case exists).
- OAuth token expires after 7 days in GCP "Testing" publish status; see
  design note §3 for the two remediation options awaiting an owner choice.
