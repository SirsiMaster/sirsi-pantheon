# Pantheon Sequoia / investor demo brief

This brief is the five-minute story for the Pantheon product surface. It is
grounded in the exact local candidate below and keeps live engine admission,
M1/M5 transport, and release credentials visibly separate.

## Candidate being shown

- Commit: `937aba9d86a66862344374ea602c9d0909a09e63`
- Tree: `ff5781fcb893b7e20ad96717163df9aa68cabc27`
- Parent: `35314f3b788aa6a3426ca10c7acc70cea399346f`
- Version: `0.23.9-beta`
- Accepted cumulative source reviews:
  - Variant routing: `/private/tmp/pantheon-variant-routing-df22b212-independent-source-review-20260922.json`
    (SHA-256 `e2903f3852e0259f2a8fc265373684f4f0ee80e4acd5a7bb5cb32c817a8c040f`).
  - CLI bootstrap: `/private/tmp/pantheon-cli-variant-bootstrap-35314f3b-independent-source-review-20260922.json`
    (SHA-256 `4a578a2272f2d630dd8750d90d0474c81fa82ab4475312e54033d10e14980da0`).
  - First-question layout: `/private/tmp/pantheon-first-question-layout-937aba9d-independent-source-review-20260922.json`
    (SHA-256 `cc2ed54b365acb9e4f4850ef59862ca0e130433bff0fe9633f2f618cbccdb7c9`).
- The review accepts source/layout only; it is not a test, build, preview, or
  live-session result.

## The five-minute path

1. After a fresh SSA admission, set `CANDIDATE_SIRSI` to the CLI built from
   this exact commit and start it on loopback port `9120`. Do not use the
   ambient `sirsi` on `PATH`. In a second terminal run the read-only identity
   check:

   ```sh
   "$CANDIDATE_SIRSI" dashboard preflight --port 9120 \
     --expect-commit 937aba9d86a66862344374ea602c9d0909a09e63 \
     --expect-version 0.23.9-beta
   ```
   It requires `pantheon.dashboard-identity/v1` and compares the running
   commit/version with the candidate above. If the endpoint is missing or
   mismatched, do not present the page. Open the verified 9120 URL in Chrome;
   keep any existing installed-app process on 9119 untouched.
2. Open Home. Start with **Ask Horus about this machine**. The primary path is
   **Session route** → **Open engine selector** → **Use sample question**.
   Use **All tools** only if the audience wants a health or infrastructure
   workflow. Explain that Pantheon owns the product experience and route
   provenance while the selected engine remains an explicit policy choice.
3. Press **Open engine selector**. Choose a configured engine variant:
   `mlx-raw`, `mlx-patched`, `omlx-public`, `sne-plain`, or `sne-mtp`. The
   route row says **policy selected**; it does not pretend that a live session
   has been admitted.
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

## CLI backup path

If the dashboard preview is unavailable, use the same exact candidate CLI and
an explicitly configured route:

```sh
"$CANDIDATE_SIRSI" engine prompt --engine sne --variant sne-plain \
  --prompt "What should I address first on this machine?"
```

The command returns completion and receipt JSON. It is not a substitute for a
live dashboard rehearsal, and an unavailable connector must remain an error.

## What to say when asked about the architecture

“Pantheon is the product and control plane. It gives the operator one Engine
ABI, one explicit route decision, one evidence receipt, and one worker-plane
authority. The inference engines remain replaceable connectors behind that
contract.”

For the M1/M5 proof, the authenticated client surface is
`pantheon.worker-control/v1`. It has no local worker registry and does not use
unrestricted SSH or a copied router store.

## What is verified versus not shown

The exact candidate has independent `ACCEPT_SOURCE_ONLY` review for the
variant-aware route selector, first-question structure, and responsive route
status row. Focused tests, build, browser preview and live dashboard have not
been run for this candidate. The runbook's preflight step is still required to
bind a future preview to the exact candidate.

This brief does not claim a live M1/M5 prompt, signed or notarized assets,
installation, publication, or production readiness. Keep any existing 9119
process untouched. A separate preview on 9120 requires SSA's exact-candidate
observation/admission first. The prior `HOLD_SNE_ACTIVE` receipt
`/private/tmp/pantheon-5115015a-sne-active-preview-hold-20260922.json`
(SHA-256 `c1e153050c671efbe64c6e5c9f6a1355c5788d2dcfdc432d91164108253c6ac1`)
binds older ancestor `5115015a`, is nonreusable, and grants no authority for
`771293e0`. SNE task `01a0932e-2ad2-7fa3-b884-0550bd287043` was active at the
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
