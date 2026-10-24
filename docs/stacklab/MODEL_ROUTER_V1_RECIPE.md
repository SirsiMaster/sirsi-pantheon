# Model-router v1 completion and decision-evidence recipe

**Stack Lab component:** `stacklab.recipe.model-router-v1`

**Versioned contract:** [`contracts/stacklab/model-router-v1-recipe-v1.json`](../../contracts/stacklab/model-router-v1-recipe-v1.json)

**Requirement/module map:** [`MODEL_ROUTER_V1_TRACEABILITY.md`](MODEL_ROUTER_V1_TRACEABILITY.md)

**Surface:** `sirsi ask`

**Owner:** Pantheon
**Status:** source candidate; native qualification and a real routed-request handoff are separate evidence.

## Authority and scope

The router in `internal/modelrouter` remains the sole policy owner. This
corrective source implements the released130 R1/R2/R4 acceptance obligations
from contract SHA256
`8aa5f1f73e6a049b35d9ac0c2416350809d0821cf985f1da178ba9ecb93c1e9a` and the
accepted SNE-side routing design. It does not introduce a provider, credential,
model, service, or second journal authority. Provider-readiness R5 has its own
separately accepted normal/race/negative source evidence at
`d0a12981a3dfba955baa32ccf6c0796f4e5be488` (accepted evidence reference
`b04b…`; the later 0.24.132 combined candidate is reported at 199 each green).
That evidence is not attributed to or rerun on the 691-based model-router
candidate. Context qualification and the accepted f66 streaming path are retained.
Optional generation requirements have the bounded source contract below;
no empirical backend determinism or JSON-mode qualification is inferred.

## Operator behavior

`sirsi ask` runs Doctor first. When measured alarms exist, the default path
prints the heuristic findings and does not call a model. `--route` is an
intentional opt-in to ask for a model answer despite those alarms; the same
findings remain visible in text and JSON. Existing `--json --fix` refusal is
preserved, and `--route --fix` is refused rather than silently skipping either
requested action.

`--lane local` and `--lane remote` request an explicit lane. The request is
validated before provider work. `privacy=local-only` refuses `--lane remote`
without probing or contacting a remote provider. A requested lane that is
unavailable or below the existing context requirement is recorded as a
refusal. No override changes a profile grant or qualification declaration.

```sh
sirsi ask --route --task judgment --privacy shareable "Review this decision"
sirsi ask --route --lane local --privacy local-only "Explain the local finding"
```

## Single completion and fail-down

The command calls `Router.Complete` exactly once. The router selects under the
existing availability/context policy, completes once, and only when a remote
provider returns a structured 429 or a recognized structured 402 quota/budget
refusal may it select and complete one qualified local provider. It never
retries remote providers or sleeps. Cancellation/deadline causes are terminal
even when joined with structured exhaustion and the parent context remains live;
authentication, malformed or unstructured errors do not trigger fallback. If fallback is attempted and
fails, both provider causes remain in the returned error and the decision
records the final attempted lane. The surface renders the final provider/model
and the redacted fallback category, not the initial remote selection.

## Decision evidence and privacy

The injected router sink defaults to
`~/.sirsi/model-router/decisions.jsonl`. It preserves the existing owned
`~/.sirsi` mode, including 0755, and creates only a private 0700 child. The
lock and journal are owner-validated regular 0600 single-link files opened
without following symlinks. A finite kernel `flock` spans journal identity and
size checks, optional roll, append, and sync, so independent processes cannot
both pass the size check. The journal is capped at 256 KiB and each JSONL event
at 2 KiB; it rolls its contents when the next event would exceed the cap.
Events include validated task/privacy/latency/context inputs, requested and
effective response-format/determinism contract (allowlisted values only), requested
override, initial/final lanes, provider/model labels, reason, outcome and a
structured failure category. They do not accept or store prompts, measured
summaries, answers, raw errors, response bodies, bearer values or credentials.
Refusals and failures are events too. A write/sync/close or bounded-lock failure
is returned to the caller and never causes inference to run again.

## Source evidence and remaining proof

Inert fixtures cover rate/quota fallback, exact-once provider calls, fallback
failure with both causes, override/privacy refusals, capability-qualified
fallback, joined cancellation with a live parent, redacted/private bounded
evidence, normal 0755 parent preservation, symlink/hardlink refusals, an
independent-lock size-check interleaving, Doctor-alarm opt-in, and final route
rendering in text and JSON. These are authored
assertions, not a qualification result. The finite native package/command
qualification and one harmless real routed local answer must be admitted and
run by Main. No paid frontier request is required. Do not infer streaming,
determinism, JSON-mode, R5 readiness, runtime health, or release acceptance
from this source candidate.

## Optional generation contract — original capability-input correction

**Component:** `model-router-generation-requirements-v1`; schema/module map:
[`model-router-generation-requirements-v1.json`](../../contracts/stacklab/model-router-generation-requirements-v1.json).
**Classification:** platform-foundation corrective. Operator: existing `ask`
caller requiring structured generation without silent fallback to text. Value:
privacy and output-contract trust; paid willingness-to-pay is unvalidated.
Pantheon owns support/diagnostics; SSA owns release/install qualification.

`--response-format=json-object` requests provider JSON generation; `--json`
still means the CLI provenance envelope. The flags can be combined. The former
adds an explicit JSON instruction and typed `response_format:{type:json_object}`
only on a selected backend whose adapter explicitly declares that capability.
`--response-format=text` and unset mode omit the wire field. Format and
determinism flags do NOT consent to bypass measured alarms: without `--route`,
findings remain visible and an actionable nonzero refusal asks for that explicit
opt-in before any ladder/probe/inference work. These requirements cannot be
combined with `--fix`. Refused requested generation returns an error instead of
implying that a heuristic answer fulfilled the requirement.

Only JSON-object mode is included, not `json_schema`. JSON+stream refuses before
Doctor/provider work. Buffered JSON success requires exactly one JSON object,
finish=`stop`, no tools/refusal or incomplete result, and a complete single
provider response envelope. Invalid results never trigger inference replay.

`--require-determinism=batch_invariant` names the existing producer's narrowly
scoped contract, not seed/temperature-zero repeatability. The current published
SNE contract at `7dff30c892ee91662daaecc5b4329a45021544a4:server.go`
reports `determinism.batch_invariant_available/active` as launch-time state.
ADR-003 explicitly says launch flags are NOT wire negotiation. There is no
per-request determinism field in its `chatReq`. The existing OpenAI-compatible
adapter therefore advertises no usable determinism mode and refuses this
requirement as unknown before discovery/HTTP. No server launch changes,
invented wire flag, capability endpoint schema, or inferred positive support
are introduced. An inert typed-provider match fixture is not SNE qualification.

Caps distinguish absent evidence (unknown), explicit unsupported and supported.
The existing selected Pantheon `LocalProfile` accepts optional `json_object`:
absent means unknown, false explicitly unsupported, true declared supported for
THAT validated exact endpoint/model. `Local` and `Ladder` consume the declaration;
it is not a SNE capability endpoint field, proof of backend implementation, or
empirical qualification. Remote/unselected defaults stay unknown. Selecting a
different profile without the declaration does not inherit a prior profile's
support. No installed profile is edited by this source correction. An
OpenAI-compatible endpoint name, seed, sampling value or successful HTTP response
is not support evidence. Actual producer support remains necessary before an
operator declares a backend qualified; no current SNE support is claimed.

Router and provider request requirements merge without weakening either;
explicit conflicts refuse before probing. Route/Complete/Stream, overrides
and structured-exhaustion fallback share the effective requirements. Privacy
remains dominant. Generation-capability metadata is checked before availability
(which can perform inference) and rechecked after it. Direct adapter calls also
enforce requirements. Unset requests retain existing text/streaming behavior.

### Replaceable modules and finite proof

- `internal/provider/requirements.go`: typed modes/states, pure validation and
  output postcondition; `requirements_test.go`: direct refusal before HTTP,
  exact inert wire capture, invalid/incomplete envelope/content, cancellation.
- `internal/modelrouter/router.go`: effective request, shared prefilter and
  fallback; `requirements_test.go`: three-state selection, mismatch, privacy,
  override, fallback, malformed result/no replay, cancellation/journal once.
- `internal/modelrouter/decision_record.go`: allowlisted requested/effective
  mode evidence; no prompts, output or provider error body.
- `cmd/sirsi/ask.go`: actual Cobra flags, explicit intent and unchanged envelope;
  `ask_requirements_test.go`: parsing/adapter, refusal, heuristic compatibility,
  cancellation and one-call journal failure with owned inert dependencies.
  The selected-profile fixture runs the real `Local`/`Ladder` resolver and
  captures its model/readiness/JSON-object HTTP requests in an owned loopback
  server; only Doctor and the journal are inert. Absent/false/malformed
  declarations refuse before HTTP, while alarm/format-only cases construct no
  ladder. `internal/provider/json_profile_test.go` checks selection isolation.

Main's prepared packet must run focused normal/race, vet/lint/canonical build
and compiled production-only qualification/wire/postcondition omission controls.
No result is claimed here. Installed handoff, backend empirical repeatability,
and full original task completion remain separate, unproven obligations.
Rollback removes this optional requirement slice; it must not remove f66's
qualified streaming behavior or weaken the privacy/fallback/evidence baseline.
The repository contract is authoritative; Reading Room/Workspace mirrors are
not updated by this source-only checkpoint and remain pending publication.
