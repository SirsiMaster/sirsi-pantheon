# Mail Ops (`sirsi mail`)

Detects mailbox hygiene problems and cleans them up — archive-only, never
deletes, never empties trash, never sends. Currently Gmail-only (OAuth via
the Gmail API). See [ADR-067](../ADR-067-MAILOPS.md) for why this exists.

## One-time setup

1. Get a GCP OAuth Desktop client credentials file (`client.json`) for a
   project with the Gmail API enabled.
2. `sirsi mail auth <label>` — opens a browser, you sign in and approve;
   writes `~/.sirsi/mailops/token-<label>.json` (mode 600).
3. Register the account in `~/.sirsi/mailops/accounts.yaml` (see
   `configs/mailops.yaml` for the shape — label, email, token path).

## Commands

```
sirsi mail poison <label>            # dry-run: list inbox messages that hang Outlook's IMAP sync
sirsi mail poison <label> --apply    # archive them (remove INBOX label), write a signed receipt
sirsi mail senders <label> [N]       # top N inbox senders over the last year, flags List-Unsubscribe
```

`poison` finds messages whose multipart body has a zero-length `text/plain`
part — the shape that hangs Outlook's IMAP sync loop for ~5 minutes per
occurrence. Dry-run is the default for every mutating command in this repo;
`--apply` is required to actually archive.

Every `--apply` batch writes a receipt to
`~/.sirsi/mailops/receipts/<label>-<timestamp>.jsonl` — the message IDs
archived and why, for audit/rollback reference. Receipts are local-only,
never transmitted (Rule A11).

## What this does NOT do

- Does not delete mail or empty trash (`gmail.modify` scope only, and the
  `mailops.Client` interface has no delete method — structurally impossible,
  not just avoided by convention).
- Does not send mail (no `gmail.send` scope, no send method).
- Does not watch Outlook's own logs yet, or propose/apply filter rules —
  both are deferred pending owner input; see
  [MAILOPS_DESIGN.md](../design-notes/MAILOPS_DESIGN.md) §2 and §1.
