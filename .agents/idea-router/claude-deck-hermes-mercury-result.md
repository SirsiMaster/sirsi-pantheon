# claude-deck: Hermes → Mercury alignment

Scope checked: `packages/sirsi-portal-app/`, `docs/investor-deck/`, `docs/market-materials/`,
`artifacts/investor-deck/`, `marketing/`, `docs/DECK_CLAIM_TRACEABILITY.md`.

## Updated (active deck/UI copy)
- `packages/sirsi-portal-app/public/deck-tight-embed.html`
  - "How Hermes syncs across the fabric" → "How Mercury syncs across the fabric"
  - Qualification-plan gate description "Kernel TCP and Hermes over Thunderbolt 4..." →
    "Kernel TCP and Mercury over Thunderbolt 4..."
- `packages/sirsi-portal-app/src/routes/apollo-workbench.tsx`
  - Route option label "Network · Hermes" → "Network · Mercury"

## Preserved as compatibility identifiers / historical evidence (not rewritten)
- `docs/DECK_CLAIM_TRACEABILITY.md` — sha256-pinned receipt citing `SirsiMaster/sirsi-hermes`
  release `v1.1.0` (hash `0642f3ab...`), binary name `sirsi-hermes`. Repo/binary rename is the
  protocol owner's migration, not deck's.
- `deck-tight-embed.html` sleeve-src citation "Hermes scaling model, Sept 2026 (Photon · Hermes ·
  Apollo — Scaling to 256 Macs)" — literal title of a dated source document; left as published.
- `artifacts/investor-deck/2026-09-19/` and `.../2026-09-22/` released deck snapshots — immutable
  historical artifacts, unchanged.
- `docs/apple-stack-lab/hardware-admin/**` (HERMES_* docs + evidence/*) — Hardware/Stack Lab
  protocol-owner territory per the directive, not claude-deck's lane; left untouched.

## Also updated
- Agent memory `project_sirsi_system_names.md` — canon now reads Mercury (formerly Hermes) for
  the Thunderbolt transport rail; Apollo and Photon unchanged.

No pre-existing hermes wire/schema/router IDs were touched. No commit/push performed — the touched
deck-tight-embed.html already carried unrelated uncommitted changes at session start; these two
text edits ride on top of that working tree, left for the deck owner's normal commit flow.
