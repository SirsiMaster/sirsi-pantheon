# Model-router v1 — requirement and module traceability

**Stack Lab recipe:** [`stacklab.recipe.model-router-v1`](../../contracts/stacklab/model-router-v1-recipe-v1.json)

**Owning wing:** `stacklab.wing.pantheon.pt-wing-001` (Pantheon product/CLI)

**Policy owner:** `internal/modelrouter`

**Request surface:** `cmd/sirsi/ask.go`

This is the focused module map for the original released130 model-router-v1
acceptance contract at SHA-256
`8aa5f1f73e6a049b35d9ac0c2416350809d0821cf985f1da178ba9ecb93c1e9a`. It
expands the existing narrative recipe into independently replaceable source,
test, input, output, write, and upgrade slices. It is source/test traceability,
not a runtime or empirical acceptance record.

## Requirement-to-module map

| Original obligation | Owning module and actual source | Inputs → outputs | Existing focused tests / negative controls | Boundary / upgrade rule |
|---|---|---|---|---|
| Local-only privacy, task order, context qualification, valid request classes | `model-router-request-qualification-and-selection`: `internal/modelrouter/router.go`; provider declarations in `internal/provider/provider.go` | task, privacy, latency, nonnegative minimum context, optional lane override, ordered providers/Caps → first available qualified provider or explicit refusal | `router_test.go`: `TestRoute_LocalOnly_UsesLocal`, `TestRoute_LocalOnly_FailsWhenLocalDown`, `TestRoute_Judgment_PrefersRemote`, `TestRoute_Judgment_FallsBackToLocalWhenRemoteDown`, `TestRoute_Generation_PrefersLocal`, `TestRoute_ContextGap_EscalatesToRemote`, `TestRoute_ContextGap_LocalOnly_Fails`, `TestRoute_SelectsFirstAvailableQualifiedProvider`, `TestRoute_InvalidRequestFailsBeforeProbing`, `TestRoute_CanceledContextDoesNotProbeOrRoute`, `TestRoute_CancellationDuringProbeDoesNotRoute` | Privacy is dominant; malformed inputs fail before probing; preserve current ordering and zero/unset defaults. Context is represented and qualified. |
| Existing provider/profile declarations consumed by the route | `configured-provider-profile-and-readiness-boundary`: `internal/provider/provider.go`, `resolve.go`, `local_profile.go`, `openaicompat.go` | selected configuration/profile and current provider declarations → provider ladder, capability declarations, or explicit unknown/refusal | `resolve_test.go`: `TestLadderPrefersLocalOverRemote`, `TestNoCredentialsMeansNoRemoteRung`, `TestLocalDeclaresNoToolCalling`, `TestRemoteStreamingRequiresExplicitDeclaration`; `local_profile_test.go`: `TestSelectedProfileUnknownCapabilitiesFailLoud`; `openaicompat_test.go`: `TestCompleteResolvesServedModel`, `TestCompleteFailsWhenBrokerServesNoModel` | R5 is separately owned; this mapping does not modify it and does not attribute its separate accepted evidence to 691. No new credentials/profile writes. |
| Structured remote rate/quota exhaustion falls down once to local | `structured-remote-exhaustion-and-single-local-fallback`: `internal/modelrouter/router.go`, `internal/provider/broker_error.go` | one remote completion plus structured status/code/type and original privacy/context requirements → at most one qualified local completion, with final route or both causes retained | `execution_test.go`: `TestCompleteFallsDownOnceForStructuredRemoteExhaustion`, `TestCompleteDoesNotFallbackWithoutStructuredEligibleExhaustion`, `TestCompletePreservesBothProviderCausesWhenFallbackFails`, `TestCompleteRetainsRemoteCauseWhenQualifiedLocalFallbackIsUnavailable`, `TestCompleteDoesNotRetryCanceledOrLocalOnlyRequests`, `TestCompleteJoinedCancellationIsTerminalEvenWhenParentIsLive`, `TestOverridesAreValidatedPrivacyFirstAndRecorded`, `TestLocalOverrideNeverEscalatesAfterCompletionFailure` | Only structured eligible remote 429 / recognized 402 quota-budget exhaustion; no fallback on cancellation/deadline, local-only, explicit local override, auth, malformed, or unstructured errors. No repeated remote sweep or retry loop. |
| Per-request override, privacy refusal, and outcome evidence | `model-router-request-qualification-and-selection`, `structured-remote-exhaustion-and-single-local-fallback`, and `bounded-private-redacted-decision-journal`: `router.go`, `decision_record.go` | typed request and decision → requested/effective lane, refusal or final outcome, redacted reason | `execution_test.go#TestOverridesAreValidatedPrivacyFirstAndRecorded`; `router_test.go#TestRoute_InvalidRequestFailsBeforeProbing`; `decision_record_test.go` private-sink tests | Remote override cannot defeat local-only privacy. Explicit local selection never silently escalates. Invalid override fails before provider work. |
| Redacted bounded decision evidence for selected/refused/fallback/failed outcomes | `bounded-private-redacted-decision-journal`: `internal/modelrouter/decision_record.go` | validated route and bounded provider/model/reason/outcome/category fields → bounded JSONL record or visible sink error | `decision_record_test.go`: `TestPrivateDecisionSinkUsesPrivateChildAndPreserves0755Parent`, `TestPrivateDecisionSinkRejectsUnsafeParentsAndDirectoryLinks`, `TestPrivateDecisionSinkRejectsHardlinkedJournalBeforeAppendOrTruncate`, `TestPrivateDecisionSinkRefusesJournalSymlink`, `TestPrivateDecisionSinkBoundsJournalWithIndependentLockDescriptors`, `TestAcquireDecisionLockTimesOutWhileIndependentDescriptorHoldsIt`; `execution_test.go#TestDecisionSinkReceivesRedactedProviderMetadata`, `TestCompleteDecisionSinkFailureDoesNotRepeatInference` | Private 0700 child below the existing home directory; preserve a pre-existing 0755 parent; owned regular 0600 single-link lock/journal; finite interprocess lock around bounds-check/roll/append/sync. Never store prompts, measured summaries, answers, raw errors/bodies, bearer values, or credentials. |
| One real ask execution path; alarms visible during explicit routing; final lane shown in text/JSON | `ask-routing-intent-and-final-decision-surface`: `cmd/sirsi/ask.go` | Doctor findings, ask flags, provider ladder → heuristic answer or one `Router.Complete`, visible alarms, final route/provider/model and response/refusal | `ask_test.go`: `TestAskRejectsJSONFixBeforeDiagnosis`, `TestAskExplicitRouteKeepsDoctorAlarmsAndUsesSingleFinalDecision`, `TestAskJSONShowsAlarmAndActualRoutedCompletion`, `TestAskDefaultHeuristicAlarmDoesNotInvokeModel`, `TestAskSurfacesDecisionSinkFailureWithoutRepeatingCompletion` | Alarms remain visible with `--route`; alarms remain model-free by default; existing `--json --fix` and `--route --fix` refusals remain. Do not bypass `Router.Complete` or repeat inference after sink failure. |
| Correct captured ask output without changing other global writers | `ask-command-writer-and-terminal-formatting`: `cmd/sirsi/ask.go`, `internal/output/terminal.go` | selected Cobra `io.Writer` plus existing severity/style → normal human rendering or JSON bytes on that writer | ask explicit-route and JSON tests above; `terminal_test.go`: `TestHeader`, `TestInfo`, `TestDim`, `TestSuccess`, `TestWarn`, `TestStyles_Render` | Ask output is command-scoped; existing terminal styling and non-ask process-writer behavior remain. No package-global writer/race workaround. |

## Explicitly open original criteria

The provider interface already has an optional `StreamingProvider`; the
OpenAI-compatible adapter has `Stream` SSE parsing and adapter-level tests in
`internal/provider/openaicompat_test.go` (`TestStreamParsesSSEChunksAndDone`,
`TestStreamCancellationClosesResponse`,
`TestStreamRejectsEOFBeforeTerminalMarker`). The local profile also requires an
explicit streaming declaration. Those facts do **not** make streaming a
qualified routed request: `modelrouter.Request` has no streaming requirement,
`Router.Complete` is buffered, and `sirsi ask` calls that buffered path. There
is no routed-stream fixture proving capability refusal, fallback before first
chunk, terminal failure after partial output, or bounded cancellation.

Likewise, `provider.Caps` has no JSON response-mode or determinism evidence,
and `modelrouter.Request` cannot require either. `sirsi ask --json` describes
the CLI answer envelope, not a provider JSON-generation capability. These
three requirements remain **unknown/unqualified** until request requirement,
backend evidence, the shared route predicate, and actual wire execution are
connected. Do not infer support from a profile name, endpoint shape, or adapter
implementation.

Provider-readiness R5 has separate accepted normal/race/negative source
qualification at `d0a12981a3dfba955baa32ccf6c0796f4e5be488` with accepted
evidence reference `b04b…`; the later 0.24.132 combined candidate is reported
at 199 each green. This maps that owner-boundary evidence but does not attribute
it to or rerun it on the 691-based model-router candidate. The model-router
recipe's test names identify authored fixtures; this documentation change
claims no execution of those fixtures, live provider/model call, successful
real routed-request handoff, cost, quality, latency, or performance. Main-owned
qualification for the exact model-router source tuple and any separately
authorized real handoff remain distinct evidence.

## Negative controls and safe replacement boundary

Keep the existing controls that must reject the former unsafe behavior:

- invalid request, canceled context, or cancellation during probing must make
  no provider probe/route (`router_test.go` invalid/cancellation tests above);
- unstructured/unknown failures, cancellation, local-only privacy, and explicit
  local override must not trigger a local fallback (`execution_test.go` tests
  above);
- remote override under local-only privacy and invalid overrides refuse before
  provider execution (`TestOverridesAreValidatedPrivacyFirstAndRecorded`);
- unsafe parent, symlink, hardlink, wrong journal identity, lock timeout, and
  sink failure are rejected without inference replay (decision-record and
  execution tests above);
- default alarm behavior invokes no model; explicit `--route` preserves the
  alarm and produces at most one final completion; JSON/fix combinations keep
  their refusal (`cmd/sirsi/ask_test.go` above).

Replace one named component at a time. Keep `internal/modelrouter` as the sole
policy owner, use the existing provider ladder/profile and `DecisionSink`
boundary, and preserve the actual `sirsi ask` path. Any change to component
source, tests, inputs, outputs, writes, or qualification ownership updates the
versioned Stack Lab recipe and this matrix together. Do not execute recipe
strings. This contract adds no runtime feature, provider, credential, model,
database, service, automatic review, or remote authority.
