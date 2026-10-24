# Model-router v1 completion and decision-evidence recipe

**Stack Lab component:** `stacklab.recipe.model-router-v1`

**Surface:** `sirsi ask`

**Owner:** Pantheon
**Status:** source candidate; native qualification and a real routed-request handoff are separate evidence.

## Authority and scope

The router in `internal/modelrouter` remains the sole policy owner. This
corrective source implements the released130 R1/R2/R4 acceptance obligations
from contract SHA256
`8aa5f1f73e6a049b35d9ac0c2416350809d0821cf985f1da178ba9ecb93c1e9a` and the
accepted SNE-side routing design. It does not introduce a provider, credential,
model, service, or second journal authority. Provider-readiness R5 remains the
separate owned change. Context qualification is retained; streaming,
determinism and JSON-mode capability requirements remain explicitly unknown
and unqualified.

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
retries remote providers or sleeps. Cancellation, authentication, malformed
or unstructured errors do not trigger fallback. If fallback is attempted and
fails, both provider causes remain in the returned error and the decision
records the final attempted lane. The surface renders the final provider/model
and the redacted fallback category, not the initial remote selection.

## Decision evidence and privacy

The injected router sink defaults to the owner-local
`~/.sirsi/model-router-decisions.jsonl` file. The directory must be private
mode 0700 and the regular journal mode 0600. The journal is capped at 256 KiB
and each JSONL event at 2 KiB; it rolls its contents when the next event would
exceed the cap. Events include validated task/privacy/latency/context inputs,
requested override, initial/final lanes, provider/model labels, reason,
outcome and a structured failure category. They do not accept or store prompts,
measured summaries, answers, raw errors, response bodies, bearer values or
credentials. Refusals and failures are events too. A write/sync/close failure
is returned to the caller and never causes inference to run again.

## Source evidence and remaining proof

Inert fixtures cover rate/quota fallback, exact-once provider calls, fallback
failure with both causes, override/privacy refusals, capability-qualified
fallback, cancellation, redacted/private bounded evidence, Doctor-alarm
opt-in, and final route rendering in text and JSON. These are authored
assertions, not a qualification result. The finite native package/command
qualification and one harmless real routed local answer must be admitted and
run by Main. No paid frontier request is required. Do not infer streaming,
determinism, JSON-mode, R5 readiness, runtime health, or release acceptance
from this source candidate.
