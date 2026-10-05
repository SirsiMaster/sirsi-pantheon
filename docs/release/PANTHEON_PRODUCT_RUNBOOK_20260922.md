# Sirsi Pantheon | Working product operation

Use Sirsi Pantheon through its running installation and ordinary operator
workflows. Show current state, actual results, limitations, and errors as they
occur; do not stage or rearrange product state. Keep signing, notarization,
installation lifecycle, and production-qualification claims tied to their own
evidence.

Use the normal operating mode. The live installation and configuration
determine what is available and what Pantheon reports.

## Operating guidance

Open the working Pantheon installation through its normal app or CLI entry
point. Use the dashboard already belonging to that installation; do not start
a second server, choose a special port, or replace a running process. If the
normal product is unavailable, report that state instead of improvising a
second instance.

The CLI self-preflight must recognize `--expect-self`, and the running
dashboard must return its build identity. If either is absent, the installed
app is older than this runbook's workflow. Stop before submitting a model
request; do not switch to another `sirsi` on `PATH` or start a second server.
Use the approved package lifecycle to update the installation, then repeat
the preflight.

Start from the current product state and follow the workflow that fits the
operator's task. Use the product's live results and evidence as they appear.

If an engine route is relevant, use an explicitly configured, supported
variant: `mlx-raw`, `mlx-patched`, `omlx-public`, or `sne-plain` (Apollo
(Plain)). The UI distinguishes policy selection from a session that actually
opened. A submitted prompt keeps the route policy captured at acceptance;
changing the dashboard preference while it runs affects later prompts. Use
the request receipt to confirm the route that handled it. Keep the
selection/session distinction clear. `sne-mtp` is Apollo Flash (Speculative),
a configured choice but not proof of availability: the connector verifies the
live v3 readiness mode and matching assistant identity before opening a session
and again before completion. If that proof is absent or changes, Pantheon
fails closed without downgrading to plain.

For a real request, use the normal product flow. Report the answer, route,
fallback state if any, and evidence created by that request. Describe evidence
only according to fields it actually contains.

For other available operator workflows, follow the product's normal
authorization and confirmation flow. A state-changing action is optional; do
it only when warranted and authorized, then report the live result Pantheon
returns.

## CLI path

Set `SIRSI_BIN` to the absolute `Contents/MacOS/sirsi` executable inside the
same installed Pantheon app that owns the running dashboard. Change the
example path if that app is installed elsewhere; do not use an unrelated or
stale `sirsi` found earlier on `PATH`. The self-preflight below verifies the
CLI/dashboard pairing.

```sh
export SIRSI_BIN="/Applications/Pantheon.app/Contents/MacOS/sirsi"
test -x "$SIRSI_BIN"
"$SIRSI_BIN" dashboard preflight --expect-self
"$SIRSI_BIN" engine status
OPERATOR_PROMPT="Explain what a model completion receipt records in one sentence."
"$SIRSI_BIN" engine prompt --engine sne --variant sne-plain \
  --prompt "$OPERATOR_PROMPT"
```

For other configured routes, use `--engine mlx --variant mlx-raw`,
`--engine mlx --variant mlx-patched`, `--engine omlx --variant omlx-public`,
or `--engine sne --variant sne-mtp` when the configured endpoint reports the
v3 MTP mode and the exact configured assistant identity. The CLI selection
applies to that invocation and does not silently persist a new policy or widen
fallback behavior; `--allow-fallback` must be explicit. Without `--stream`, stdout is a
single JSON object containing the completion and identity-bound receipt. With
`--stream`, stdout is newline-delimited JSON: the first record binds the
opened session and route, followed by ordered events; the terminal completion
event carries the receipt. Provider, identity, and transport failures are
emitted as terminal error events and returned as command failures. Ctrl-C or
SIGTERM requests cancellation through the prompt context, and Pantheon reports
the terminal cancellation event when stdout remains writable. The selected
route must advertise streaming and cancellation support before Pantheon opens
a session.

`SIRSI_BIN` must resolve to the executable belonging to the normal installed
Pantheon product, not an unrelated binary found by an ambiguous `PATH` lookup.
Keep endpoint credentials in the authorized local environment, never in this
runbook or screenshots.

## Connector reference

The dashboard accepts one endpoint and identity tuple per configured route:

| Route | Endpoint variable |
|---|---|
| Raw MLX | `SIRSI_MLX_RAW_ENDPOINT` |
| Patched MLX | `SIRSI_MLX_PATCHED_ENDPOINT` |
| Public oMLX | `SIRSI_OMLX_ENDPOINT` |
| SNE plain | `SIRSI_SNE_PLAIN_ENDPOINT` |
| SNE MTP | `SIRSI_SNE_MTP_ENDPOINT` |

For each endpoint prefix, provide `_MODEL`, `_ENGINE_VERSION`, `_MODEL_SHA256`,
`_TOKENIZER_ID`, `_TOKENIZER_SHA256`, `_PRECISION`, and `_CACHE_NAMESPACE`.
SNE routes additionally require `_TOKEN`, `_RUNTIME_SHA256`,
`_NATIVE_RUNTIME_SHA256`, and `_MANIFEST_SHA256`. Do not place credentials in
shared materials. Pin initial dashboard policy with both
`SIRSI_ENGINE_PREFERRED` and `SIRSI_ENGINE_PREFERRED_VARIANT`; non-default-only
routes require an explicit variant. Do not configure the same kind/variant
twice.

## M1 → M5 worker plane

If the working product has the authenticated worker plane available, use it
from the M1 client mode for ordinary authorized operations. M5 remains the
canonical worker authority; M1 does not keep a second registry. Do not expose
tokens or use an unrestricted SSH shell. Any state-changing action must go
through the product's normal authorization and be verified in its live result.

## Operating checks

- Use the product's normal installation and launch path. Do not replace an
  existing process or create a separate service for this session.
- Confirm the route and receipt behavior in the live session. A selected
  policy alone does not prove a connector opened or completed a request.
- Keep the dashboard on loopback unless the product's authorized deployment
  configuration explicitly says otherwise.
- If a connector or workflow is unavailable, say what is unavailable and
  continue with another working Pantheon workflow. Never silently switch
  engines.
- Keep signing, notarization, installation lifecycle, host qualification, and
  production readiness claims tied to their own evidence.
