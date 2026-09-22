# Pantheon Sequoia / investor demo brief

This brief is the five-minute story for the Pantheon product surface. It is
grounded in the exact local candidate below and keeps live engine admission,
M1/M5 transport, and release credentials visibly separate.

## Candidate being shown

- Commit: `5115015a352f80620dcf16b023ca4c6552f72c87`
- Tree: `b154806ea7f00ed4091b3cb105d674bbd8a64fce`
- Version: `0.23.9-beta`
- Current focused dashboard receipt: `/private/tmp/pantheon-accessibility-5115015a-receipt-20260922.json`
- Receipt SHA-256: `5f176438c5712d6249d9d3cdd933581d5e4779a08e1fbfba24384f874d1dc943`

## The five-minute path

1. Start the preview from the CLI built from this exact candidate, bound to
   loopback on a separate free port (use `9120` if available). Do not use the
   ambient `sirsi` on `PATH`: the currently installed CLI is older and does
   not include the identity-preflight command. Then run the candidate-built
   CLI's read-only identity check:
   `sirsi dashboard preflight --port 9120 --expect-commit
   5115015a352f80620dcf16b023ca4c6552f72c87 --expect-version 0.23.9-beta`.
   It requires `pantheon.dashboard-identity/v1` and compares the running
   commit/version with the candidate above. If the endpoint is missing or
   mismatched, do not present the page. Keep the existing installed-app
   process on 9119 untouched; the live listener there currently lacks the
   identity endpoint and is not this demo candidate.
2. Open Home. Start with **Ask Horus about this machine**, choose an engine or
   health task from the four visible actions, and open **More actions** only
   when the audience wants the secondary tools. Explain that Pantheon owns
   the product experience and route provenance while the selected engine
   remains an explicit policy choice.
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

This candidate has focused PASS evidence for the Home action hierarchy,
keyboard-accessible disclosure, dashboard identity contract, skip link,
primary-navigation landmark, and semantic main landmark. The isolated Go
cache was removed after verification. The receipt above binds the dashboard
landmark change to this exact candidate; the broader readiness receipt for
`dd0e701b` covers a predecessor and is not evidence for this candidate.

This brief does not claim a live M1/M5 prompt, signed or notarized assets,
installation, publication, or production readiness. The current 9119 process
must be explicitly replaced or a separate preview instance authorized before
the new Home surface is shown from a running service.

## Demo safety

- Keep the dashboard on loopback.
- Use the separately authorized preview instance; do not replace or restart
  the existing dashboard process.
- Never display control tokens or private receipts.
- Do not silently switch engines when a selected connector is unavailable.
- Do not use source/test receipts as live inference evidence.
- Do not restart or mutate the existing dashboard process without the bounded
  preview authorization.
