# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Pantheon operators and collaborators use the dashboard and CLI to inspect their current environment, choose an inference route, perform real work, and inspect the evidence returned by Pantheon.

## Product Purpose

Sirsi Pantheon is the product and operational control plane above replaceable inference engines. It gives operators explicit routing, consistent workflows, and truthful evidence from the environment where it is running.

Present the working Sirsi Pantheon through its normal installation, live
configuration, and ordinary operator workflows. Show the product's current
state and actual behavior as they are.

Success means an operator can select a configured engine and variant, submit a real request through the ordinary dashboard or CLI, and distinguish the selected policy, opened session, completed request, fallback behavior, and route receipt from one another.

## Positioning

Pantheon owns one shared Engine ABI and route decision across supported inference providers. It preserves identity, capability, session, streaming, cancellation, error, and receipt information while keeping inference implementation and model claims with the engine providers.

## Operating Context

- The dashboard is served by the Pantheon installation already running in the current environment; use its normal entry point and live state.
- The dashboard reflects the running installation's current state. Unavailable routes and errors are shown as reported.
- The dashboard, CLI, MCP, menu-bar, and TUI are Pantheon product surfaces over shared canonical state and evidence.
- Pantheon runs on supported environments without treating a particular physical Mac as universally canonical. Authority is scoped to the current environment and its valid role receipt.
- Stack Lab terminology: Photon is the hardware system; Apollo (Plain) is the SNE plain route; Apollo Flash (Speculative) is the SNE MTP route; Hermes is the TB rail protocol, not an inference engine.

## Capabilities and Constraints

- Supported connector identifiers remain explicit: `mlx` / `mlx-raw`, `mlx` / `mlx-patched`, `omlx` / `omlx-public`, `sne` / `sne-plain`, and `sne` / `sne-mtp`.
- Apollo Flash requires live SNE readiness and exact assistant identity. A missing or changed proof fails closed; it does not silently downgrade to Apollo (Plain).
- Engine selection is not proof that a session opened or a request completed. Fallback is never silent.
- Receipts and route provenance describe only fields Pantheon actually verifies and returns.
- Pantheon consumes inference connectors; it does not own or implement SNE inference mathematics, runtime internals, model artifacts, engine benchmarks, or inference qualification.
- M1 and M5 are environment/profile labels, not universal authority identities. Worker authority and client operations must follow the applicable authenticated, session-bound role and receipt contract; a client does not create a competing worker registry.
- Signing, notarization, publication, installation lifecycle, production qualification, and runtime claims require their own exact evidence and are not implied by source acceptance.

## Evidence on Hand

- `docs/PANTHEON_PRODUCT_WALKTHROUGH.md` — ordinary live-product workflow and evidence boundaries.
- `docs/release/PANTHEON_PRODUCT_BRIEF_20260922.md` — product and Stack Lab terminology.
- `docs/release/PANTHEON_PRODUCT_RUNBOOK_20260922.md` — operational route and receipt guidance.
- Product workflows must use live product state; no customer, performance, utilization, or availability claim is implied by this record.

## Product Principles

1. Make route choice explicit and preserve it through the request and receipt.
2. Show current, verifiable product state; never substitute a polished but untrue result.
3. Keep inference ownership behind the connector contract.
4. Preserve one authenticated worker/evidence authority per valid role assignment.
5. Keep source, test, runtime, host qualification, credentialed release, and publication claims separate.

## Accessibility & Inclusion

The dashboard is an operator-facing web interface and must remain keyboard-operable, clearly labeled for assistive technology, responsive, and truthful about live/unknown/error states.
