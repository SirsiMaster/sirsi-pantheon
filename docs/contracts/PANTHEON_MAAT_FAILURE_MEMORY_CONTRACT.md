# Pantheon Ma'at failure memory and operational preflight

Status: repository contract adopted from the SNE policy; executable integration and
publication remain pending. This document does not certify installed enforcement.
Owner: codex-pantheon. Classification: platform-foundation. Date: 2026-09-11.
Governing decision: ADR-004, extended here for operational incident prevention.
Source request: 20260911-063933-codex-inference-codex-pantheon-canonize-ma-at-shared-sne-pantheon-contract.

## Authority and purpose

SNE policy: `/Users/thekryptodragon/Development/sirsi-native-rebuild/docs/engineering/MAAT_FAILURE_MEMORY_AND_RECIPE_PREFLIGHT_POLICY.md`.
The adoption receipt records its mechanically computed SHA-256 and observation time.
A later source revision requires explicit comparison; this document does not claim
that the observed working file is a published Git object.

Pantheon adopts the loop `observe -> normalize -> hash -> scope -> preflight ->
construct -> attest -> remember` for its operational actions. Here, construction
means preparing a Pantheon operation, never constructing an SNE candidate.
Mac operators should see what failed, what was prevented, the supporting evidence,
and a specific safe recovery. The value is fewer repeated incidents and lower
support cost; this foundation is not independently launch-qualified or priced.

codex-inference retains exclusive authority to create, modify, compile, execute
engineering tests, benchmark, qualify, and promote SNE source, runtime, recipes,
and candidates. Pantheon contributes immutable incident evidence, requests SNE
guards, and consumes SNE-owned immutable results. Authorized installation and
lifecycle use of a producer-qualified distribution does not grant engineering
or promotion authority. A Ma'at pass cannot substitute for SNE parity, serving,
performance, release, signing, or installed-operation evidence.

## Shared vocabulary and identity

Both sides use: normalized failure signature, SHA-256 incident key, originating
recipe or work item, immutable evidence digest, failure class, affected
component/profile scope, deterministic prevention guard, and incident status
`active`, `superseded`, or `retired`. Superseded and retired entries retain a
durable successor reference; an old signature never silently disappears.

The SNE policy does not yet specify a byte-level interchange schema. Pantheon
must preserve producer bytes and producer keys. It must not recompute an SNE
incident key using an independently invented normalizer. The following is a
Pantheon-local proposed encoding to converge with codex-inference before shared
wire-format enforcement:

- Versioned UTF-8 JSON signature with sorted keys, no insignificant whitespace,
  strings only, and no implicit case folding. Version is part of the signature.
- Signature fields: namespace `pantheon`, signature version, failure class,
  operation, component role, violated invariant, and normalized stable error code.
- Volatile timestamps, PIDs, absolute host paths, prompts, and secrets are excluded.
  Normalization rules are versioned, not inferred by a language model.
- Incident key is SHA-256 of those exact normalized bytes. Evidence SHA-256 is
  independently computed over immutable evidence bytes; it is not the incident key.
- Component, ABI, artifact, model-profile, host-profile, and operation predicates
  specify applicability. Exact identities and explicit version ranges must remain
  distinguishable; no empty value silently becomes a wildcard.

Each entry carries its origin, producer/namespace, evidence references and digests,
applicability predicates, guard id/version/digest, status, and successor reference.
Each preflight receipt binds the exact action manifest, registry snapshot digest,
selected incidents, evaluated guards, measured checks, outcomes, timestamp, and
recovery reference. Receipts describe only checks actually executed.

## Immutable evidence and registry protocol

Use append-only events for introduction and status transitions. A local SQLite
projection may index hashes, but it is rebuildable from those events and cannot
rewrite their evidence. This extends existing Ma'at governance; it does not create
another router home or a second SNE registry.

Evidence is stored create-only by computed digest. Readers verify bytes before
use. Catalogs derive artifact digests from regular-file identities; manually
transcribed artifact digests are not canonical catalog inputs. A same-key repeat
is idempotent, while changed evidence creates a new event and retains the prior
receipt. Failed append or unreadable selected evidence prevents attestation.
Concurrent writers must commit event and index updates transactionally. Recovery
must distinguish a missing registry from a verified empty registry.

Reject symlinks and nonregular catalog objects, contain paths to authorized roots,
and bind checks to the same opened object or staged immutable object used by the
operation. Revalidate identities immediately before mutation to prevent a changed
file or host state from inheriting a stale pass. Imported results are data, never
commands: producer trust and authorized provenance must be verified separately
from digest integrity. Hash equality alone does not authenticate a producer.

## Deterministic prevention and narrow blocking

Evaluate active incidents whose declared scope matches the exact action. Dispatch
only locally registered guard implementations with typed input and typed argument
vectors. Do not run shell strings, interpolated heredocs, or commands embedded in
an imported receipt. Unknown schema, missing evidence, unsupported guard, or
indeterminate scope yields an explicit unverifiable result for that action.

Outcomes are `pass`, `reject`, and `unverifiable`. Only a pass makes an operation
eligible for its existing authorization and release gates. A rejection blocks the
matching action, not unrelated observation, review, research, or a correctly scoped
successor. No mutation occurs during preflight. Dry-run and explicit narrow consent
remain required where existing safety law requires them. No silent model, runtime,
precision, processor, account, framework, or cloud substitution is recovery.

## Pantheon scope and acceptance matrix

| Scope | Preflight obligation | Required negative and recovery evidence |
| --- | --- | --- |
| Engine ABI | Bind client/engine identities and negotiated supported ABI | Incompatible and missing ABI rejected; compatible successor rechecked |
| Installation | Verify producer provenance, package/catalog bytes, signature, authorized target and dependencies | Tampered/nonregular artifact rejected; partial staging leaves current installation intact |
| Update | Bind current/candidate identity, schema compatibility, ownership and serialized transaction | Concurrent update and unsupported migration rejected; failed commit preserves recoverable prior state |
| Rollback | Verify retained qualified artifact and data/schema compatibility before restoration | Missing artifact or irreversible schema mismatch rejected; restored identity and readiness proved |
| Model lifecycle | Consume sole SNE qualification and Studio lifecycle result; bind model/runtime/profile and permission state | Stale/wrong tuple, missing permissions and unavailable readiness remain explicit; no fallback or SNE candidate creation |
| Host admission | Bind host profile, current resource evidence, activity and operator constraints | Stale measurements, unknown activity and inadequate resources reject the scoped start; no load-bearing process termination |
| Diagnostics | Verify observation provenance/freshness and redact before any authorized export | Missing/corrupt evidence remains unknown; secrets, prompts and sensitive paths excluded from exports |
| Recovery | Bind exact incident, authorized recovery plan, prerequisites and postconditions | Failed postcondition never becomes success; interrupted recovery records durable failure and safe next action |

All eight rows require both pure guard tests and integration proof through the
existing authoritative engine entry point before being marked implemented.

## Integration plan and release evidence

/plan: retain the producer policy snapshot identity; obtain independent review of
this contract; converge byte-level incident interchange with codex-inference;
extend existing Ma'at and operation entry points; verify and publish per phase.
/goal: one deterministic Pantheon incident/preflight path used by each operational
surface, with exact incident-to-regression traceability and installed proof.
Estimated duration: contract review 1 working session; implementation estimated
3-5 working days after a verified integration base, subject to acceptance findings.

1. Contract and fixture convergence: codex-pantheon owns the local adapter;
   codex-inference owns SNE producer bytes. Agree golden signatures, status
   transitions, evidence references, scope boundaries, and incompatibility receipts.
   Acceptance: byte-identical replay, cross-producer isolation and unknown-version
   rejection. Do not invent active incidents without verified source evidence.
2. Ma'at core: extend `internal/maat` with normalization, append-only storage,
   immutable evidence verification and typed guard dispatch. Reuse existing
   operation identity and authorization types. Tests cover deterministic hashing,
   tamper, corrupt/missing store, concurrent append, idempotency, supersession,
   invalid successor, exact-scope isolation, symlink and object-replacement cases.
3. Operation integration: connect the eight rows through the authoritative engine
   (`internal/engine`) and existing setup, selfupdate, SNE lifecycle, host and
   recovery owners. Inspect current source before selecting exact call sites.
   Coordinate with `pantheon-stacklab-contract-integration`, installer and runtime
   continuity ledger obligations rather than duplicating their implementations.
   Use A16 injected side effects and A21 concurrency-safe seams.
4. Operator projection: CLI/JSON, MCP, TUI, native Mac, menu bar and retained
   dashboard consume the same receipt. Show freshness, what was blocked, verified
   reason, evidence and permitted recovery; missing results display unknown.
   No per-surface policy decisions or fabricated quality scores.
5. Qualification: focused table-driven and race tests, full Go test/vet/lint and
   build gates, independent exact-object review, then CI and permissioned clean-host
   install/update/rollback/interruption/recovery proof. Record source, artifact,
   installed identity and observed postconditions separately. Complete the repo
   completion proof only when these obligations actually pass.

Current delivery is the contract and plan. No executable guard, installed
qualification, benchmark, or release success is asserted by this document.

## Privacy, publication and recovery ownership

Local diagnostic paths, process arguments, model contents, prompts and credentials
remain local. A hash is not permission to transmit sensitive evidence. Sharing
requires a privacy-safe derived receipt and existing explicit export authority;
raw local audit logs are never uploaded. Mirrors carry policy text, not host data.

Canonical repository path: `docs/contracts/PANTHEON_MAAT_FAILURE_MEMORY_CONTRACT.md`.
Owner Reading Room: pending; this session cannot write that filesystem location.
Sirsi Google Workspace: pending; no mirror id has been established for this contract.
Git publication: pending; managed session denied `.git/FETCH_HEAD` writes.
These are publication blockers, not a requirement for renewed owner approval.
A writable publication lane must preserve this path, publish through Git/CI and
independent review, update existing human indexes, and record stable mirror ids.
