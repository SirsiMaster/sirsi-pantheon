# Pantheon Sequoia / investor demo brief

This brief is the five-minute story for the Pantheon product surface. It is
grounded in the exact local candidate below and keeps live engine admission,
M1/M5 transport, and release credentials visibly separate.

## Candidate being shown

- Commit: `dd0e701b158e990752270b7819cc7ca44b8699e7`
- Tree: `56f6321b37e1a20d8e43adf944ef9aab80ea3216`
- Version: `0.23.9-beta`
- Local readiness receipt: `/private/tmp/pantheon-demo-readiness-gate-dd0e701b-receipt-20260922.json`
- Receipt SHA-256: `d0e9b9b0065a1be616c9c4d3c8c1f96658a7028c6dd46b219d17d23f8600c375`

## The five-minute path

1. Run the read-only identity preflight against the demo port. Require
   `pantheon.dashboard-identity/v1` and compare the returned commit with the
   candidate above. If the endpoint is missing or mismatched, do not present
   the page: the port is serving an older process.
2. Open Home. The page starts with **Ask Horus about this machine**, not a
   status wall. Explain that Pantheon owns the product experience and route
   provenance while the selected engine remains an explicit policy choice.
3. Press **Open engine selector**. Choose SNE, MLX, or oMLX only when that
   connector is configured. The route panel says **policy selected**; it does
   not pretend that a live session has been admitted.
4. Press **Start with a question** and ask:

   ```text
   What should I address first on this machine?
   ```

5. Show the exact answer, selected route, and request receipt. Explain that
   findings are quoted from the machine's diagnostic report; the engine does
   not invent paths, names, or numbers.
6. Press **Inspect Fleet evidence**. Show that M5 is the canonical worker
   authority and M1 is a constrained client. Do not show a mutation unless a
   separate owner authorization is active.

## What to say when asked about the architecture

“Pantheon is the product and control plane. It gives the operator one Engine
ABI, one explicit route decision, one evidence receipt, and one worker-plane
authority. The inference engines remain replaceable connectors behind that
contract.”

For the M1/M5 proof, the authenticated client surface is
`pantheon.worker-control/v1`. It has no local worker registry and does not use
unrestricted SSH or a copied router store.

## What is verified versus not shown

The candidate has local PASS evidence for Engine ABI, provider, dashboard, CLI
engine/control workflows, routerboard control tests, and committed diff-check.
The isolated Go cache was removed after verification.

This brief does not claim a live M1/M5 prompt, signed or notarized assets,
installation, publication, or production readiness. The current 9119 process
must be explicitly replaced or a separate preview instance authorized before
the new Home surface is shown from a running service.

## Demo safety

- Keep the dashboard on loopback.
- Never display control tokens or private receipts.
- Do not silently switch engines when a selected connector is unavailable.
- Do not use source/test receipts as live inference evidence.
- Do not restart or mutate the existing dashboard process without the bounded
  preview authorization.
