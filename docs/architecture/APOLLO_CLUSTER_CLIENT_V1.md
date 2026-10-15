# Pantheon Apollo cluster client — platform groundwork

The client projects Apollo's `internal/clusterapi` types and handlers at exact
service commit `950c778d`. It reconciles preparation from Pantheon `aa3f6a37`
with the authentication refusal and lifecycle/recovery safeguards from the
`f6144e54` correction branch. It contains no engine implementation.

## Availability and trust boundary

Remote CLI operations currently refuse **before network access** because no
enrolled endpoint/session authentication provider is installed. HTTPS alone does
not authenticate the caller or authorize a destination for private text. The
library accepts an injected HTTP client, rejects nil clients, requires HTTPS and
refuses redirects. A non-nil client is not proof of enrollment. No router token,
secret flag, locally minted key, SSH/subprocess route or fallback is supplied.
Ra's retained-source inspection of c078d592 and21f7f216 also found no approved
Apollo front-door provider. An actual provider/configuration and authenticated
positive/negative installed proof remain prerequisites to enabling commands.

## Intended conversation flow

```text
sirsi apollo chat --endpoint https://ENROLLED-SERVICE \
  --model EXACT --machines auto --mode plain --precision Q8 \
  --prompt "Explain this incident." --max-output-tokens 512
sirsi apollo status --endpoint https://ENROLLED-SERVICE --job-id JOB --job-receipt admission.json
sirsi apollo events --endpoint https://ENROLLED-SERVICE --job-id JOB --job-receipt admission.json --after 42
sirsi apollo cancel --endpoint https://ENROLLED-SERVICE --job-id JOB --job-receipt admission.json
```

These commands remain disabled, not qualified installation instructions. `chat`
sends role/content text to service-owned `/v1/apollo/prepare`. Apollo owns the
verified tokenizer/template and returns a short-lived preparation with exact
model/checkpoint/tokenizer/template/plan identities, tokens and rank_count.
The job request is derived from that preparation. There is no user token-ID,
model-path or manual immutable-hash override. Plain and Flash remain separate;
exact950c778d advertises only Plain. Single-host v3 and offline planning remain
separate; no silent substitution is allowed.

The chat renderer updates completion/failure state before selecting JSON or
human output. Both require observed `completed` followed by final `released`;
failed/cancelled/orphan outcomes and incomplete streams are errors.

## Admission and lifecycle predicates

Preparation binds request/session/model/mode/precision/output extent and four
lowercase SHA-256 identities. New admission requires exactly prepared rank_count
fences, contiguous ordered rank IDs and unique hosts, nonempty fence/generation,
positive memory and live RFC3339 expiry. **Preparation does not expose expected
host placement.** Exact950c778d `validAdmission` enforces host/budget equality
against its private frozen plan; client rank-count/topology checks do not
independently verify absent plan contents, signatures or trusted generation data.
That service enforcement is conditional on real Auth/planner/backend integration.

Status/cancel/events take the original admission tuple. Every response/event must
match job/execution/request/plan/epoch. Reconnect never adopts fresh status as
original identity. Original expired fences remain structurally valid for
recovery queries; fresh admission and live response fences still require expiry
in the future. Recovery does not renew resources or authorize computation.

Streams bound individual events and retain no transcript. There is no cumulative
4 MiB live-stream cap. Final release/orphan evidence is required; completed or
failed alone does not prove teardown. An empty/early EOF, transport failure or
oversized frame returns `IncompleteStreamError.LastSequence` for explicit
reconnection against the same receipt. Events after a terminal event are errors.
Authorized typed token IDs/positions/text/error code and raw event content are
emitted only to the caller's conversation/terminal, never general host telemetry.
The caller owns private persistence of the frozen receipt and accepted cursor;
a local JSON file alone does not authenticate anything.

## Cancellation mutation gate

The existing service accepts only a contract body for cancellation. Client
response checks occur after mutation and cannot protect a rebound target.
Before enabling the client, the service owner must provide enforced immutable,
nonreused owner/session-bound job IDs or an agreed atomic tuple precondition.
A stale/rebound-target test must prove zero worker mutation. No unilateral
precondition fields are added to the wire here.

## Verification and remaining delivery

The complete `internal/apollo` package must pass with `go test -race` (no excluding
regex). CLI tests exercise JSON/human success and failure through the same
stream-rendering function used by chat. Socket-free injected fixtures retain
HTTP routing/body validation without depending on host listener permission.

Native enrolled authentication and foreign-endpoint/unauthorized refusal,
production planner/backend/all-rank execution, stale-target mutation protection,
private durable receipt/cursor integration, current-main CI, independent review,
signing/install/rollback and actual useful streamed conversation remain open.
This document does not claim a launchable or installed feature.

Architecture authority: SIRSI_MANIFESTO.md; PANTHEON_RULES.md;
ADR-055-SNE-LICENSING-ANUBIS-RA-SEAM.md. Service source is read-only reference;
no engine source crosses into Pantheon.
