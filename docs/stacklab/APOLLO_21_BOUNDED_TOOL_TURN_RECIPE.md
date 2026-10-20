# Apollo 2.1 bounded tool-turn recipe

**Stack Lab component:** `stacklab.recipe.apollo-2.1-bounded-tool-turn`

**Surface:** `sirsi gemma tool-turn`

**Owner:** Pantheon
**Status:** source-ready; a real endpoint qualification is separately recorded.

## Purpose

This recipe turns one selected Apollo 2.1 local response into a bounded,
inspectable tool interaction. It is not a general agent loop and it is not a
model launcher. Apollo may request one declared tool; Pantheon decides whether
to invoke it through the existing typed `reason.Registry`, then gives Apollo
the factual result for one final answer.

The recipe is designed for the adapter contract exercised by Apollo source
`3fb7b0f6`: OpenAI-compatible loopback chat, greedy-only decoding, structured
`tool_calls`, and a separate on-demand `apollo-serve-batch` lifecycle. The
adapter never executes tools itself.

## Inputs and admission boundary

The command requires all of the following before it opens a network request:

1. An explicit `--inference-profile` path.
2. A loopback local profile with exact `id: "gemma-m5-apollo21"`.
3. An explicit `tool_calls: true` declaration in that profile.
4. One or more explicit `--allow-tool` names.
5. A currently registered tool with each requested name.

The profile is only an endpoint/capability selection. It does not start
Apollo, download a model, authorize GPU use, grant router ownership, or create
a long-lived service. An admitted on-demand Apollo batch is responsible for
those concerns and must produce its own endpoint/idle-exit receipt.

## Bounded execution

```text
operator prompt
  → Apollo request with exact no-argument tool schemas
  → zero or one untrusted model tool request
  → existing reason.Registry + PolicyReadOnly
  → factual JSON tool result, correlated by provider tool-call id
  → one Apollo follow-up answer
  → caller records its own task/lease/cleanup receipt
```

The current CLI uses `PolicyReadOnly`, so only observe tools can run. Repair
and destructive tools are not enabled by this surface. Tool arguments are
rejected, multiple calls are rejected, missing call IDs are rejected,
unallowlisted tools are rejected, and a follow-up tool request is rejected.
There is no shell interpolation or model-selected command path.

## Operator shape

```sh
sirsi gemma tool-turn \
  --inference-profile /absolute/reviewed/apollo-profile.json \
  --allow-tool process.census \
  "What is the current process situation?"
```

The command is not a substitute for the fenced router worker. A worker that
uses this recipe must preserve its claimed item, tool-call correlation, final
answer, endpoint receipt, and cleanup result before it may complete the item.

## Proof and upgrade rules

- Unit coverage proves provider wire format, exact profile refusal, explicit
  allowlist enforcement, no-argument rejection, one-turn limit, and repair
  refusal under the read-only policy.
- A real qualification uses an attended approved Apollo batch; it must not
  reuse a synthetic tool harness as evidence that Pantheon executed a tool.
- The ordinary `sirsi gemma` and `sirsi gemma worker` paths remain text/draft
  paths until their own fenced integration adopts this recipe.
- Changes to profile identity, tool schemas, policy, or result correlation
  require a new Stack Lab review and qualification; compatibility of an HTTP
  endpoint alone is never sufficient.
