# Owner-Gated Checklist (Cylton)

Single tracked list of items that need the owner. Threads move items to ✅ as they're cleared.
Maintained by claude-home (H3). Last swept: 2026-06-29.

## Open — needs Cylton

- [ ] **SirsiNexusApp GitHub Actions failing repo-wide since 2026-06-26.**
  - **What:** Every SirsiNexusApp workflow fails at init (1-2s, zero steps executed, timestamps show completed-before-started → jobs never reach a runner). Actions IS enabled (`allowed_actions=all`). Last green run: 2026-06-22T13:48.
  - **Not org-wide:** sirsi-pantheon CI is green today on pricier macOS runners → org Actions billing is fine. The cause is **SirsiNexusApp-scoped**.
  - **Owner action (GitHub UI — claude-home can't see billing):** check SirsiNexusApp/org → Settings → Actions for a **repo-scoped spending limit** on standard (ubuntu) runners, a recently-added **org ruleset / required-workflow** on this repo, or a repo Actions usage cap.
  - **Consequence:** also blocks the React Portal auto-deploy. F9 itself was worked around (admin-merge #97 + manual firebase deploy), so this is not blocking F9.
  - Routed to codex-home (owner conduit) + surfaced directly to owner.

## Cleared this session

- [x] **F9 OpenSign shared HMAC secret** — was thought owner-gated; claude-nexus self-served it (provisioned in both Secret Managers, verified end-to-end). No owner action needed.
- [x] **F4 old-HMAC re-issuance scope** — fact-find proved zero prod directives ever signed under any secret (`signing_envelopes`=0). Closed, no re-issuance, no owner action.

## Pending owner decision (raised by claude-home)

- [ ] **H2 / `sirsi prd sync`:** keeping every thread PRD reconciled to canon needs the unbuilt `sirsi prd sync` tool (queued to claude-pantheon in the consolidated Router-v2/self-maintenance backlog). Decision: build it now in this session, or leave it queued for the pantheon thread?
