# Own-task preserved evidence relocation

This finite corrective operation repairs references after their artifacts have
already been preserved elsewhere. It is not task completion, permission to
relocate files, a generic task editor, or authority to repair another lane. It
changes only selected `links` URLs and/or `result_ref`, plus reserved provenance,
in one canonical transaction. Generic `TaskUpdate` is unchanged.

## Portable inputs and offline planning

Supply two existing regular files using absolute physical paths (no symlink in
any component): a closed v1 manifest and the preservation map. Paths below are
illustrations, not host configuration or an instruction to create these files.
Artifact references accept canonical absolute POSIX paths or local `file:///`
URIs without host/query/fragment. They never fetch network content. There is no
hardcoded machine, canonical hostname, database path, or cloud credential gate.

```json
{
  "version": 1,
  "id": "owned-preservation-batch-1",
  "agent": "example-lane",
  "attestation_ref": "review://existing-independent-process-receipt",
  "preservation_map_sha256": "<64 lowercase hex characters>",
  "tasks": [{
    "task_id": "existing-task",
    "expected_sha256": "",
    "changes": [{
      "field": "link",
      "kind": "evidence",
      "label": "original receipt",
      "old_uri": "file:///evidence/retired-checkout/receipt.json",
      "new_uri": "/evidence/preserved/receipt.json",
      "sha256": "<64 lowercase hex characters>"
    }]
  }]
}
```

For `result_ref`, use `field: "result_ref"` and omit `kind`/`label`. A link is
identified by its exact original kind, label and URL; ambiguous identical links
refuse. Existing unrelated links, including previously appended preservation
links, are not removed. One manifest permits at most 64 tasks and 128 reference
changes, within a 128 KiB request. This method also caps the raw HTTP body,
including RPC envelope and whitespace, at 128 KiB before authentication/JSON
decoding; other RPC methods retain their existing limits. IDs are bounded simple slugs. Unknown/duplicate
JSON fields, alternative field casing, repeated tasks, repeated field identities,
unsupported columns and missing apply fingerprints refuse.

The existing map schema is `pantheon.checkout-preserved-namespace.v1`, with
`rows` containing `old_absolute`, `preserved_absolute`, `sha256` and
`type: "regular"`. Other preservation metadata remains allowed; duplicate keys
in any object and case aliases of decoded root/row control fields refuse. Every requested
mapping must match exactly one row. The exact map bytes are SHA-256 bound (4 MiB
limit, 4096 rows); every replacement is independently hashed (64 MiB per-file
limit). Request/map/target reads are bounded, descriptor-backed and nofollow,
reject special files, and check named identity/size/mtime continuity. This is a
cooperative point-in-time custody check, not a lease against future file edits.
The intentionally absent old artifact is never opened, recreated or required.

```sh
sirsi router task relocate-evidence --manifest /evidence/manifest.json \
  --preservation-map /evidence/PRESERVATION_MAP.json
```

Default is **offline local-plan only**: no `Resolve`, RPC, router DB creation,
fingerprint authority claim, registry change or task write. Output binds the
parsed manifest digest and states the limits. A local hash is not canonical row
validation or independent approval.

## Current authority, review, then explicit application

1. Run the same command with `--inspect` only when the configured canonical
   service/relay is reachable. It uses the existing `Resolve` route and real
   session; a configured localhost service is valid. Explicit sandbox SQLite is
   refused before it can create a database. Normal enrolled relay recovery is
   retained; there is no alternate database, endpoint flag or credential copier.
2. Copy each returned `before_sha256` into that task's `expected_sha256`. Do not
   reuse an older audit's digest if ordinary task edits have occurred since it.
   Re-run offline checks and obtain the existing independent Main/Ra/Astra review
   of the **exact** final manifest and artifact custody. This is external process
   evidence, not a new server approval protocol or answer-role grant.
3. Only the normal reviewed, published successor workflow may invoke the same
   command with `--apply`. `--inspect` and `--apply` are mutually exclusive;
   extra positional arguments refuse. Source implementation and fixture review
   alone do not authorize live application or retire a parent operational goal.

Before future apply, Main/SSA must qualify the configured canonical service's
exact served build/schema and support for this new method, with prior server
writers drained/replaced across canonical instances. Old server implementations
can still overwrite links from a pre-read snapshot; a new server cannot fence
an old binary's SQL. This is a service-first deployment/compatibility boundary,
not refusal of existing 121/126 clients: their ordinary `UpdateTask` requests
remain unchanged and accepted, with no new flags, roles or approval gate.

The typed RPC is `RelocateTaskEvidence(context, {action, manifest})`. Both actions
require a fresh, live RuleOfRa own-lane session even when an embedding's general
gate is log/off. The server derives agent/host/session/thread/runtime from actual
authentication; the manifest cannot override them. Bootstrap/operator tokens
have no foreign-task exception. Metadata repair consumes existing lane authority
without requiring, changing or bypassing a worker answering-role declaration.
Session and live-thread binding are rechecked in the transaction.

## Atomicity, fingerprints and replay

`before_sha256` is `sirsi.task.public-persisted.v1`: SHA-256 of the domain string
`sirsi.task.public-persisted.v1` plus NUL, then each of the 20 `taskSelect` public
persisted columns in order, encoded as a validity byte, uint64 big-endian byte
length and exact stored bytes. Nullable and raw JSON distinctions are retained.
Derived liveness and private lease/scope columns are never returned or hashed.
This is versioned service computation, not a client-reconstructed Task DTO hash.

SQLite acquires actual `BEGIN IMMEDIATE` on a retained connection before reading
the snapshot. PostgreSQL locks selected task rows in sorted task-ID order and
holds shared principal-row locks. All expected public fingerprints and exact old
field identities must match; failure rolls back the whole batch and provenance.
Only `links`/`result_ref` occur in the task UPDATE: status, phase, timestamps,
charter, accounting, timeline, scope and all lease columns remain unchanged.
Link JSON may be re-encoded within the selected `links` column; identities,
order and all unselected link values are preserved. Result-only edits do not
re-encode `links`. Stored link objects must contain exactly string-valued `kind`,
`label` and `url`; incomplete/null/non-string objects refuse rather than
normalizing unrelated history. No task status/claim trigger is requested.

Apply (including exact replay) checks private ownership under the retained task
row lock and refuses ANY nonempty `lease_token`, `lease_expires`, `claimed_by` or
`thread_id`, including complete valid expired custody. Expiry does not prove
quiescence: an existing completion writer may have bound its live timestamp
before waiting on SQL and can still land later. This operation never clears or
reclaims custody. The existing normal custody reconciliation must first finish;
later read-only inspection of the actual target rows must verify they are
unowned before reviewed application. This is not a new role/approval gate.
Inspect can read public fingerprints while held, but does not certify apply eligibility.
Lease columns remain excluded from public fingerprints; this separate check is
necessary even when only private ownership changed.

Existing-row links writers are the relocation operation and ordinary
`UpdateTask`; `AddTask` and requirement reconciliation insert only and cannot
replace an existing primary key. Ordinary updates capture decoded Task and raw
links from the same SELECT and pin every write to those exact links. A
reference-only conflict internally reloads/remerges the original request, up to
four attempts, without duplicating accounting increments. Status and ownership
conflicts still refuse; relabels also pin lease expiry so a delayed renewal cannot
be cleared. No new wire fields are required. `CompleteTaskLease` is the other
existing result-reference writer and remains live-token/expiry fenced; active
ownership is never bypassed by metadata relocation.

The reserved `task-evidence-relocation:v1:<agent>:<id>` record contains the exact
manifest, authenticated actor identity, timestamp, before/after fingerprints and
explicit qualification. It records an **actor attestation** of external review
reference and client filesystem hashes; the service does not remotely verify
either. Generic `SetState` and CAS cannot write any version of this prefix.
No hidden task lease capability appears in the result/provenance.

Lost-response replay must use the exact same final manifest/ID and match every
recorded current after-state. It returns `repeated: true, applied: false` without
rewriting tasks or adding another audit. Merely finding the target URI is not
idempotence. Changed manifest, later task edit, missing provenance or corrupted
record refuses; there is no force/rebase switch. Reinspect and independently
review any genuinely new correction rather than manufacturing an old state.

## Qualification boundary

Focused source fixtures: `TestTaskEvidence*` in `internal/routerstore` and
`cmd/sirsi`, including `taskevidencewriters_test.go`; retain existing task-update,
claim, accounting and lease regression suites. Backend fixtures use the existing admitted test harness (SQLite by
default; PostgreSQL only with its existing isolated `SIRSI_TEST_PG_DSN`). They
cover raw private-column preservation without exposing those columns to clients,
transaction rollback, replay, concurrency, HTTP authority and offline CLI inputs.
No tests/build/live apply were executed by the source producer. Main owns normal,
race, negative controls, independent review, publication and any subsequent
reviewed canonical application. Actual checkout-cleanup custody and remaining
parent obligations are not inferred complete from this source change.
