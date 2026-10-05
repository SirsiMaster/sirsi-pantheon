# Sirsi Pantheon — Working Product Walkthrough

Use Sirsi Pantheon in its normal installation and ordinary operator workflows.
Show its current state, routes, responses, and resulting evidence as they are.
Explain what the product does as each workflow unfolds, including the
capabilities and limitations visible in the running product.

## Start from the running installation

- Open Pantheon from its ordinary installed app or CLI entry point. Use the
  dashboard already belonging to that installation; do not start a second
  server or select a special port.
- Work from the current machine and live configuration.
- Keep credentials, tokens, and private receipts private.
- Describe only what the product actually reports. If a route or capability
  is unavailable, record that accurately and continue with another available
  workflow without implying that the unavailable path succeeded.

## Product workflows

### Stack Lab names and route identity

Photon names the hardware context, not an inference engine or a promise that a
particular physical Mac is canonical. Apollo (Plain) names SNE's plain route;
Apollo Flash (Speculative) names SNE's MTP route. Hermes names the TB rail
protocol, not an inference backend. Pantheon keeps these product names
visibly paired with the canonical engine/variant identifiers and receipt data
below, so a label never substitutes for verified route identity.

### 1. Establish product identity and current state

Use the CLI that belongs to the installed Pantheon, then show the dashboard
already running for that installation.

Point `SIRSI_BIN` at the CLI inside the same installed app. Replace the example
with the app's actual location if it is not installed in `/Applications`; do
not substitute a copied or unrelated `sirsi` found earlier on `PATH`.

```sh
export SIRSI_BIN="/Applications/Pantheon.app/Contents/MacOS/sirsi"
test -x "$SIRSI_BIN"
"$SIRSI_BIN" dashboard preflight --expect-self
"$SIRSI_BIN" engine status
```

The preflight confirms that the CLI and dashboard identify the same running
product. If the installed CLI does not recognize `--expect-self`, or the
dashboard does not expose its build identity, stop here: the installed app is
older than this workflow. Do not substitute a different binary from `PATH` or
start a second dashboard; update through the approved package lifecycle, then
repeat the preflight. Explain the state visible on screen as it stands; do not
promise a particular finding, count, or performance number in advance.

### 2. Show an actual engine route

In the dashboard's Engine view, inspect the configured routes and select one
that is available in this environment. The UI's selected policy is not proof
that a session opened. Submit a real request through the ordinary command bar
or CLI and let Pantheon report whether the connector opened and completed it.
The dashboard's preferred route belongs to that running dashboard process;
the CLI route is selected explicitly for each command with `--engine` and
`--variant`. A dashboard prompt uses the policy captured when the request is
accepted; changing the preferred route while it runs applies to later prompts.
Use the returned receipt to distinguish the route requested from the route
that actually completed the request.

For the CLI, choose one engine/variant pair shown by `engine status` and
configured in this installation. These are the supported pairs:

| Product name | Engine | Variant |
|---|---|---|
| Apollo (Plain) | SNE | `sne-plain` |
| Apollo Flash (Speculative) | SNE | `sne-mtp` (only when live readiness and the exact assistant identity match) |
| — | MLX | `mlx-raw` |
| — | MLX | `mlx-patched` |
| — | oMLX | `omlx-public` |

Use exactly one matching pair in the command; these are alternatives, not a
fallback sequence:

```sh
OPERATOR_PROMPT="Explain what a model completion receipt records in one sentence."
"$SIRSI_BIN" engine prompt \
  --engine sne \
  --variant sne-plain \
  --prompt "$OPERATOR_PROMPT"
```

For another configured route, replace both flags together—for example,
`--engine mlx --variant mlx-raw` or `--engine omlx --variant omlx-public`.
For `sne-mtp`, Pantheon requires the live v3 readiness mode and the exact
configured assistant identity before it opens a session.

Show the returned answer and its route/receipt fields. Distinguish a chosen
route, an opened session, a completed request, and any fallback; they are
different outcomes. Do not silently change the persistent policy or enable
fallback to make a request appear successful.

### 3. Complete an operator workflow

Choose an action that matches the live state and the questions at hand.
Follow its ordinary authorization and confirmation flow. If an action changes
state, show the confirmation and the product's returned result. If no change
is warranted, inspect the available evidence instead.

### 4. Use worker coordination when it is available

If authenticated worker control is configured, inspect the canonical worker
state through Pantheon's client interface. M5 remains the worker authority;
M1 is a constrained client/execution node, not a second registry. Submit a
message, review request, delegation, claim, cancellation/handback, or result
return only when it is authorized and relevant. The Fleet view's action
composer sends through the local capability and displays M5's request-bound
receipt; refresh the view to read the resulting canonical state. Keep tokens
private. If the control plane is unavailable, identify that accurately.

### 5. Review the resulting evidence

Summarize the exact workflow performed, configured route, observed result,
receipt/evidence produced, and any limitation encountered. Separate current
working behavior from signing, notarization, installation lifecycle,
production qualification, or release claims; those require their own proof.

## Operating principle

Use Sirsi Pantheon in its normal configuration. Its live state and actual
behavior—including errors and unavailable capabilities—are the product truth.
