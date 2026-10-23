# Finite service-notice reconciliation

Classification: platform foundation; Stack Lab wing `stacklab.wing.ra-horus-fabric`.
Operational owner: SSA, with Ra owning routing and future service-reply policy.
This is a source candidate, not deployed reconciliation or a staffed Horus lane.

## Purpose and boundary

Horus is an actor-only local status/hygiene service, not a place to send agent
work. Historical replies have accumulated there. A recipient-only close is
correct to refuse another actor; impersonating Horus, creating a consumer,
granting unrestricted `close:any`, or editing the database behind the service
would not constitute an acceptable recovery.

`ReconcileMailboxNotice` is a separate typed operation for a finite set of
reviewed coordination decisions. Version one accepts only recipient `horus`.
It does not dismiss owner items, transfer requests, complete ledger tasks,
reopen history, dispatch a reply, or expose foreign inboxes to a scoped token.
Existing Get/CloseItem/Complete/Reassign permissions remain unchanged.

## Approved manifest

The operator captures each exact Item through the canonical service and computes
`routerstore.MailboxNoticeSHA256`. The digest binds every retained Item field,
including delivery history and prior result. Review the full original body:
dates or titles alone cannot establish that work is superseded.

The supported `sirsi router notice-fingerprint <id>` command performs one
ordinary authenticated Get and emits the exact id/digest without the body.
It grants no read exception: use the existing authorized operator when a lane
token cannot read the historical notice. Digests use versioned length-prefixed
field bytes, not a JSON rendering that could normalize legacy invalid UTF-8.

The manifest contains an immutable id, the intended real logical actor, exact
recipient, existing owner approval reference, durable evidence reference and
its reviewed SHA256, expiry
within24hours, and at most64 unique notice id/digest/result entries. Approval
does not assert product completion. Do not include the Oct20 wedged-supervisor
correction or Oct22 SSA supervisor diagnosis merely because both are decisions:
their actual service obligations remain tracked separately.

Only the canonical service's existing bootstrap operator may provision the
manifest using action `approve`. The lane cannot approve its own access. This
requires the existing operator execution path, never exporting the bootstrap
token to a lane, browser, prompt, or request file. No new credential is minted.

## CLI and execution

`sirsi router reconcile-notice --request /absolute/request.json` prints a bounded
request digest without contacting a service or opening a ledger. Requests are
regular non-symlink files, captured once through a nonblocking descriptor; JSON
is closed and capped128KiB. No shell snippets or caller actor fields are accepted.

`--apply` uses only the canonical RemoteStore or its restricted-shell relay.
The `ack-notice` request supplies only the approved manifest id and exact notice
id. The server derives agent/host/session/thread/runtime from authentication,
requires fresh registration even if an old general gate is configured log/off,
and enforces the headless-worker answering role.

Inside one SQL transaction, the store revalidates the live unrevoked principal,
locks the approval, verifies finite approval/digest/status and an empty item lease,
and commits the coordination close with its custody audit. On PostgreSQL,
principal share locks and the manifest update lock serialize competing writes;
the item UPDATE also predicates all captured source fields. Audit failure or
approval expiry rolls back the close. Prior body, recipient, sender and previous
result remain retained. Exact retries acknowledge one existing audit only if
the resulting row is unchanged; reopened/modified history refuses replay.

## Verification, rollout and recovery

Fixtures cover actual authenticated HTTP approval/use, foreign-read/close/state
refusal before and after, invalid bootstrap/actor/registration/worker role,
bounded inputs, symlink/FIFO refusal, changed rows, live leases, expiry,
transactional rollback and idempotence. Shared backend fixtures run against
PostgreSQL when SIRSI_TEST_PG_DSN is set; local SQLite results are not PG proof.

Deploy and qualify the compatible server first, retaining the previous revision
for rollback; no schema migration is required. Then install the compatible client,
capture/approve the exact reviewed manifest, apply one notice and verify the
original body and audit before processing the remaining approved set. Unchanged
real work stays open. Preserve all originals and rollback evidence. Do not undo a
mistaken acknowledgment by deleting audit state: use a separately authorized
history-preserving reopen/correction and retain the original custody record.

Source tests do not prove deployment, live notice closure, a healthy supervisor,
or full router release. Record exact source/service/client identities and every
applied/refused notice in the release evidence before claiming reconciliation.
