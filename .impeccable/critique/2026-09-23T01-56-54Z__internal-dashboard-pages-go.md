---
target: Pantheon dashboard at the current local URL
total_score: 19
max_score: 40
na_heuristics: 
p0_count: 1
p1_count: 2
target_identity: "file:/Users/thekryptodragon/Development/sirsi-pantheon/internal/dashboard/pages.go"
target_fingerprint: "sha256:2de459bc54504c749819ec1af9dc4df18cacde58f6fcbfe42d81e564e26653d3"
target_path: /Users/thekryptodragon/Development/sirsi-pantheon/internal/dashboard/pages.go
timestamp: 2026-09-23T01-56-54Z
slug: internal-dashboard-pages-go
---
# Pantheon dashboard usability review

**Target:** `internal/dashboard/pages.go`, with a fresh read-only inspection of the already-running `http://127.0.0.1:9119/`.

Important finding: these are not the same build. The current checkout’s source says “Sirsi Pantheon,” exposes Engine/SNE/Recovery navigation, and includes running-build identity. The actual page at `9119` is titled “Horus — Horus — Sirsi,” exposes the older Horus-oriented navigation, and does not show the source’s Engine/SNE workflow. Port `9119` is held by `sirsi-men` PID 3807. I left that process untouched.

| # | Heuristic | Score | Key issue |
|---|---|---:|---|
| 1 | Visibility of system status | 2/4 | Live machine metrics update, but the served page does not expose the current engine route or Pantheon build identity. |
| 2 | Match to real world | 1/4 | The served product identity says Horus; “Deities” and “process slayer” require internal vocabulary. |
| 3 | User control and freedom | 3/4 | Navigation, command input, and direct actions are available; cancellation/undo for long or stateful actions is not evident. |
| 4 | Consistency and standards | 2/4 | The live navigation mixes Egyptian glyphs, internal names, and standard task labels. |
| 5 | Error prevention | 2/4 | Current source has explicit route/capability checks, but they are not present in the running page I inspected. |
| 6 | Recognition rather than recall | 2/4 | Actions are labeled, but nine destinations and eight command actions compete without grouping; no Engine choice appears. |
| 7 | Flexibility and efficiency | 2/4 | Users can click actions or type commands; no visible shortcuts or recent-command aid. |
| 8 | Aesthetic and minimalist design | 2/4 | The page is structurally dense. Visual score is provisional because screenshot capture found no matching Codex window; this is based on source and accessibility evidence, not pixel inspection. |
| 9 | Error recovery | 2/4 | Error behavior and recovery suggestions were not visible in the inspected initial state. |
| 10 | Help and documentation | 1/4 | “Click any command above, or type it” is minimal instruction; no contextual help explains routes, identity, or receipts. |
| **Total** |  | **19/40** | **Poor** |

## Design specificity

The current source is more Pantheon-specific than the running page: it has an explicit preferred-engine section and identity badge. The running page still presents a generic local workstation monitor under Horus branding. That source/runtime split is the first problem to fix; polishing source CSS alone would not change what the operator currently sees.

## Cognitive load

High. The live page presents nine navigation choices and eight command actions, alongside four machine-status tiles and a free-form command field. The tools are not progressively grouped; the decision point exceeds four options.

## Emotional journey

Machine telemetry gives useful immediate reassurance. The old product name and absence of route/session evidence then make it difficult to trust that the page is the current Pantheon or to know what a prompt will do.

## What works

- Machine status becomes concrete without leaving the page: RAM, Git state, active services, and platform were populated in the live view.
- Sidebar and command actions have text labels, so they are not icon-only.
- Current Pantheon source already adds route-policy visibility, build identity, and a labeled prompt; the problem is that this source is not what `9119` is serving.

## Priority issues

- **[P0] Running page is not the current Pantheon build.** This makes the actual product experience contradict the current source and hides the engine workflow. Reconcile the normal launcher/service with the canonical Pantheon binary before presenting that URL. I did not replace or restart the listener.
- **[P1] The primary model workflow is absent from the served page.** There is no visible engine selection, route policy, or receipt path. Make “ask Pantheon” and the selected engine/route one continuous, inspectable workflow.
- **[P1] Navigation and actions are an ungrouped wall of choices.** Nine destinations plus eight actions slow first-time users and make the product’s core job unclear. Group secondary system tools and keep the Pantheon prompt/engine workflow dominant.
- **[P2] Internal terms and the prompt’s accessible name need attention.** “Deities,” “Horus,” and “process slayer” are not self-explanatory. The live accessibility tree exposed the input as `term-input` without a spoken label; current source has a label, reinforcing the build mismatch.

## Persona red flags

- **Alex, power user:** Can type commands or click actions, but has no visible shortcut or recent-command affordance and must scan a long navigation list.
- **Sam, assistive-tech user:** Navigation entries are named, but the live prompt field is exposed only as `term-input`; route changes and command results need verified announcements.
- **Jordan, first-timer:** The page title and terminology do not identify Pantheon’s purpose, and nothing on the served view tells them how to choose an inference engine.

## Minor observations

The detector returned `[]`, but it does not establish coverage of HTML/JS embedded in Go strings and could not detect that the live process serves different UI bytes. Screenshot capture found no matching Codex window, so no pixel-level findings or visual overlay are claimed.

## Questions to consider

1. Should I prioritize (a) aligning the existing `9119` listener with the current Pantheon build, which requires explicit authority to replace that service, or (b) continue source-level UI work while leaving the listener untouched?
2. For the UI itself, should the first pass focus on engine/prompt flow, navigation grouping and terminology, or accessibility/state feedback?
