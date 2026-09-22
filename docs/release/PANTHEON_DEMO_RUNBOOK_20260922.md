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
