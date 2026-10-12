# Router delivery reconciliation — 2026-10-12

Classification: platform foundation. SSA implements and qualifies access; Ra owns
the router release and fleet integration. This is evidence, not a claim that all
workers have already upgraded.

## Authority and restricted access

Both hosts use the existing system relay at
`spool:///private/var/sirsipantheon/relay`. The canonical service is
`https://sirsi-router-6kdf4or4qq-uc.a.run.app`; observation found revision
`sirsi-router-00025-mt7` at 100% traffic. No second database, token copy, security
weakening, service interruption or administrator dialog was introduced.

The old client misreported every denied exclusive slot open as saturation.
Only an existing slot now counts as occupied; other errors preserve their cause
and say the request was not published. Stale-slot recovery is bounded.

The named `sirsi-router` permission profile extends `:workspace`, grants the real
relay directory and disables direct network access. Fresh restricted sandboxes
on **both M5 and M1** successfully read the canonical inbox. The opt-in profile
exists on each host; it does not retroactively modify an existing sandbox.
The SSA source consumer now declares the same permissions inline, avoiding a
dependency on an optional per-user profile file. Other consumers are not claimed
converted by this change.

## Every historical row has a disposition

The preserved retired M5 database contains **42** rows, not merely the previously
reported 30. Every full row and its recorded result is retained in a canonical
archival coordination record, without executing embedded historical instructions:

`20261012-174750-sirsi-software-admin-sirsi-software-admin-historical-router-archive-42-row-reconciliation-20261012-583`

- 16 recent forwards independently matched the complete original instructions
  and exact recipient in the cloud history; all are closed there.
- 24 historical closed/recalled records are preserved, not replayed as new work.
- Two open historical Horus alerts about missing consumers are superseded by
  fresh wakeable-consumer readback. This does not prove those lanes' product work
  complete.

The carrier preserves original IDs; it is **not** an import of 42 individual
original-ID ledger rows. The original databases and archives remain untouched.
Full `ListAll` exceeds the existing 64 MiB response ceiling. The read-only
`tools/router-history-readback` uses date-bounded canonical `ListSince` to reconcile
the cutover history. Scalable history pagination remains a product limitation.

## Real transport and controlled retry qualification

M1 published a coordination-only record, M5 read and closed it, and M1 read the
closure; the inverse M5→M1→M5 round trip also passed. Both traverse the canonical
cloud service.

The opt-in `TestLiveCanonicalSpoolRetryAndDuplicate` passed on both hosts. A
private fixture edge denied sends six times, the message remained durably queued,
then recovered through the unchanged production relay. Sending the same request
again returned the same ID; closed-state readback passed. Only the fixture edge
was faulted. This is not evidence of a shared-relay restart, Cloud Run crash or
long-outage qualification. An earlier fixture run correctly reported failure
because its cleanup tried to reclose the already-closed item; that failed log is
retained, and the corrected fixture checks status before cleanup.

## Evidence locations on M5

`/Users/thekryptodragon/.sirsi/evidence/` contains:

- `router-retired-42-full-readonly-20261012.json`
- `router-history-mappings-20261012.json`
- `router-history-canonical-publication-20261012.json`
- `router-cross-host-proof-20261012.json`
- `router-live-retry-M5-pass-20261012.log`
- `router-live-retry-M1-20261012.log`
- `router-access-native-full-test-20261012.log`
- `router-cloud-service-20261012.json`

Native full Go tests passed; focused spool/outbox/redial race tests passed. The
restricted full-suite run failed at sandbox-prohibited operations and is retained
separately, not relabeled green. No broad release, signing or deployment proof is
implied by these tests.

## Remaining release work

Integrate these changes into the current Ra release, verify the resulting installed
engine identity on both hosts, synchronize the pinned registry, and requalify an
actual fresh SSA consumer launch. Existing workers must not be interrupted or
their task leases commandeered to manufacture a clean result. Stack Lab and
owner-readable mirrors must carry the same bounded claims.
