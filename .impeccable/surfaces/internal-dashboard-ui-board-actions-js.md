---
version: 1
slug: "internal-dashboard-ui-board-actions-js"
primary_target: "internal/dashboard/ui/board-actions.js"
related_targets: ["internal/dashboard/ui/index.html","internal/dashboard/ui/app.css"]
---

# Verified completion in the existing shared board

Scope: one focused owner action in the established Ra board; Operate mode. The owner asked SSA to complete verified obligations, not cancel or relabel unfinished work. Keep all navigation, assets, typography, colors, privacy withholding and existing retirement behavior unchanged.

## Direction contract

THESIS: Make an already verified completion actionable without exposing private task enumeration or pretending an execution receipt exists.

OWN-WORLD: Inherit the existing quiet dark Ra board, native form labels, button classes and focused dialog treatment. No new typeface, palette, illustrations, motion or page redesign.

STORY: The signed-in owner enters one exact lane/task ID and evidence-backed completion note, confirms the work really passed/was accepted, and receives confirmed completion or an honest unconfirmed outcome. Active or retained custody refuses atomically.

FIRST VIEWPORT: Existing tools gain one clearly named Complete verified task action. Its focused dialog contains exact lane, task, evidence note and explicit confirmation, with one submit action and Close. Status is announced without losing the user's entered evidence; responsive controls stay within the viewport.

FORM: Local extension of existing task sign-off and retirement dialog patterns; no concept tournament or seed applies. Signature interaction is deliberate evidence confirmation followed by a single bounded request. No automatic retry after uncertain response.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

No shipping raster is required. No DESIGN.md rewrite or configuration-drift repair is authorized; this extension must preserve the incumbent system. Test fake responses for success, denial, lost response, double submit and scope/identity loss; real changes require the existing authenticated board owner capability, never copied credentials.
