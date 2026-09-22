# Pantheon Sequoia / investor demo brief

This brief is the five-minute story for the Pantheon product surface. It is
grounded in the exact local candidate below and keeps live engine admission,
M1/M5 transport, and release credentials visibly separate.

## Candidate being shown

- Commit: `b642c77b7a80430f3278dab8dca85541661c000a`
- Tree: `e8ddc90ccd9ea7cf88cf0a2c0fc0956ca8cc3a8a`
- Parent: `a5fa31845b502c3db80795125238fdf2d2dc1a99`
- Version: `0.23.9-beta`
- Accepted parent source review: `4aaeba55201e6efb2a7cea319a07a92d416bf39e`
  (`/private/tmp/pantheon-home-focus-4aaeba55-independent-source-review-20260922.json`,
  SHA-256 `f25c46a7684c0a8b2f2e3e4def516b834729b5b5d6d876574e9a9827c646e46b`)
- The sample-question successor is pending exact-object source review.

## The five-minute path

1. After a fresh SSA admission, set `CANDIDATE_SIRSI` to the CLI built from
   this exact commit and start it on loopback port `9120`. Do not use the
   ambient `sirsi` on `PATH`. In a second terminal run the read-only identity
   check:
   `"$CANDIDATE_SIRSI" dashboard preflight --port 9120 --expect-commit
   b642c77b7a80430f3278dab8dca85541661c000a --expect-version 0.23.9-beta`.
   It requires `pantheon.dashboard-identity/v1` and compares the running
   commit/version with the candidate above. If the endpoint is missing or
   mismatched, do not present the page. Open the verified 9120 URL in Chrome;
   keep any existing installed-app process on 9119 untouched.
2. Open Home. Start with **Ask Horus about this machine**. The primary path is
   **Session route** → **Open engine selector** → **Start with a question**.
   Use **All tools** only if the audience wants a health or infrastructure
   workflow. Explain that Pantheon owns the product experience and route
   provenance while the selected engine remains an explicit policy choice.
3. Press **Open engine selector**. Choose SNE, MLX, or oMLX only when that
   connector is configured. The route panel says **policy selected**; it does
   not pretend that a live session has been admitted.
4. Press **Use sample question**. It only fills the prompt; review the text
   and press Enter to submit it:

   ```text
   What should I address first on this machine?
   ```

5. Show the exact answer, selected route, and request receipt. Explain that
   findings are quoted from the machine's diagnostic report; the engine does
   not invent paths, names, or numbers.
6. Press **View worker evidence**. Show that M5 is the canonical worker
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

The parent `4aaeba55` has independent `ACCEPT_SOURCE_ONLY` review for the Home
route, question and worker-evidence controls, All tools disclosure, and
accessible naming. This successor adds the sample-question fill-only
interaction; it is pending exact-object review. Focused dashboard tests ran
for ancestor `5115015a` only and do not cover `b642c77b`. Tests, build, browser
preview and live dashboard were not run for this candidate.

This brief does not claim a live M1/M5 prompt, signed or notarized assets,
installation, publication, or production readiness. Keep any existing 9119
process untouched. A separate preview on 9120 requires SSA's exact-candidate
observation/admission first. The prior `HOLD_SNE_ACTIVE` receipt
`/private/tmp/pantheon-5115015a-sne-active-preview-hold-20260922.json`
(SHA-256 `c1e153050c671efbe64c6e5c9f6a1355c5788d2dcfdc432d91164108253c6ac1`)
binds older ancestor `5115015a`, is nonreusable, and grants no authority for
`b642c77b`. SNE task `01a0932e-2ad2-7fa3-b884-0550bd287043` was active at the
latest read-thread observation; obtain a wholly fresh SSA disposition after
its terminal handback or explicit release.

## Demo safety

- Keep the dashboard on loopback.
- Use the separately authorized preview instance; do not replace or restart
  the existing dashboard process.
- Never display control tokens or private receipts.
- Do not silently switch engines when a selected connector is unavailable.
- Do not use source/test receipts as live inference evidence.
- Do not restart or mutate the existing dashboard process without the bounded
  preview authorization.
