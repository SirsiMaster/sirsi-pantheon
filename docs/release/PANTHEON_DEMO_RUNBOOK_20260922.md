# Pantheon demo runbook

This is the operator path for the Sequoia/investor demo. It demonstrates the
Pantheon product surface and truthful engine routing; it is not a signing,
notarization, installation, or production-readiness receipt.

## The three-minute story

1. Start the local dashboard without opening an uncontrolled browser window:

   ```sh
   sirsi dashboard --no-open
   open http://127.0.0.1:9119/
   ```

   Before presenting the page, verify that the listener is the intended build
   rather than an older process left on the port:

   ```sh
   sirsi dashboard preflight --port 9119 \
     --expect-commit <candidate-commit> \
     --expect-version 0.23.9-beta
   ```

   The command validates `schema`=`pantheon.dashboard-identity/v1`, commit, and
   version. A 404 or mismatch means the visible listener is not the demo
   candidate; stop and obtain the bounded preview/restart authorization instead
   of presenting it. `curl .../api/identity | jq .` remains a read-only manual
   fallback when the CLI binary itself is not the candidate.

2. On Home, point out the single flow: **Ask Horus about this machine** →
   **Session route** → **Open engine selector** → prompt input.

3. Open the engine selector and choose the configured SNE, MLX, or oMLX
   connector. The UI says **policy selected**, not **backend live**; that
   distinction is intentional.

4. Return Home and ask a grounded workstation question, for example:

   ```text
   What should I address first on this machine?
   ```

5. Show the response's exact findings, selected route, and request receipt.
   Pantheon renders diagnostic facts from its own report; the engine selects
   which findings answer the question.

## CLI proof path

The same route is available without the browser:

```sh
sirsi engine status
sirsi engine prompt --engine sne --prompt "What should I address first?"
```

Use `mlx` or `omlx` in place of `sne` when those identity-bound connectors are
configured. `engine prompt` applies the explicit selection to that invocation
and returns JSON with `completion` and `receipt`; it does not silently persist
or widen fallback policy. `--allow-fallback` must be supplied explicitly.

## M1 → M5 control-plane proof

When the demo includes the worker plane, use the M1 client mode. It refuses a
local router-store fallback and requires the authenticated M5 endpoint:

```sh
export SIRSI_CONTROL_ENDPOINT="https://<m5-tailnet-name>:8734"
export SIRSI_CONTROL_TOKEN="<protected-token-from-the-authorized-session>"
sirsi router control --client-only
```

Point out the returned `pantheon.worker-control/v1` envelope: its
`authority` is `canonical-routerstore`, its `revision` and `state_sha256` bind
the worker/task/event/evidence projection, and its closed `capabilities` list
shows the allowed control verbs. The client does not keep a second worker
registry.

Only with explicit owner authorization should the demo show a mutation. The
closed action client takes JSON values, not shell commands, and validates the
canonical receipt returned by M5:

```sh
printf '%s\n' '{"verb":"review_request","from":"m1","to":"m5","title":"Demo review","instructions":"Return the bounded result."}' \
  | sirsi router control-action --endpoint "$SIRSI_CONTROL_ENDPOINT" --request-file -
```

Do not display the token, use an unrestricted SSH shell, or imply that a
successful source/static receipt is a live worker result. `inspect` is the
safe read-only proof; `message`, `review_request`, `delegate`, `claim`,
`cancel_handback`, and `result_return` remain authenticated, receipt-bound
control-plane actions.

## Preflight checklist

- Confirm the selected connector has a complete identity: engine version,
  model ID and digest, tokenizer ID and digest, precision, and cache namespace.
- Run `sirsi engine status` and show the configured policy before submitting a
  prompt.
- Submit one prompt only after the audience understands that this is the
  readiness boundary; an unavailable provider must remain an explicit error.
- Never place tokens, endpoint credentials, or private receipts in slides,
  screenshots, or investor notes.
- Keep the dashboard on loopback. Do not expose it through a public tunnel for
  the demo.

## Honest fallback language

If the route cannot open a session, say: “The Pantheon policy is configured,
but this connector has not passed live session admission on this machine.” Do
not switch engines silently or present a source/static test receipt as a live
completion receipt.

## Boundary

This runbook does not claim signed/notarized assets, cask publication,
installation lifecycle proof, M1/M5 host qualification, or release readiness.
Those remain separate gates owned by the appropriate Pantheon/SSA lanes.
