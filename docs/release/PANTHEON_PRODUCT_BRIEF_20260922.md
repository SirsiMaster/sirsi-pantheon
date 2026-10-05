# Sirsi Pantheon | Working product overview

## Product overview

Sirsi Pantheon brings current system state, explicit engine routing, live
workflows, and the evidence those workflows produce into one working product.
Operators use Pantheon through its normal installation, live configuration,
and ordinary workflows. The product reports its actual behavior, including
unavailable routes, errors, and limitations; its state is not staged or
rearranged.

## Stack Lab names

- **Photon** names the hardware system.
- **Apollo (Plain)** is the product-facing name for the SNE plain route
  (`sne` / `sne-plain`).
- **Apollo Flash (Speculative)** is the product-facing name for the SNE MTP
  route (`sne` / `sne-mtp`); it remains subject to that route's readiness and
  identity checks.
- **Hermes** names the TB rail protocol, not an inference engine.

These names do not replace connector identifiers in commands, configuration,
API responses, or receipts. Those surfaces continue to report the exact route
that Pantheon selected and executed.

## Product explanation

“Pantheon is the product and control plane. It gives the operator one Engine
ABI, one explicit route decision, request evidence, and one worker-plane
authority. The inference engines remain replaceable connectors behind that
contract.”

For the M1/M5 architecture, M5 is the canonical worker authority and M1 is a
constrained client/execution node. The client does not maintain a second worker
registry or use unrestricted SSH as the control interface.

## Operating notes

- Use Pantheon through its current state and ordinary workflows. Do not seed,
  reset, hide, or rearrange state to shape the outcome. Report what the running
  product does, including limitations or errors.
- Use only routes configured in the running product. If one cannot open a
  session, identify that limitation plainly and continue with another
  available Pantheon workflow.
- Keep credentials and private receipts private.
- Keep signing, notarization, publication, installation lifecycle, and
  production qualification claims tied to their own evidence.
