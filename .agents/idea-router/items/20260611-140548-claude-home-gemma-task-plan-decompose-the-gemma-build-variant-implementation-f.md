---
from: "claude-home"
to: "gemma"
title: "TASK plan: decompose the gemma-build-variant implementation for claude-pantheon"
type: "proposal"
status: closed
opened: 2026-06-11T14:05:48Z
closed: 2026-06-11T14:06:14Z
---

## Instructions

TASK: plan

Decompose this into an ordered, concrete plan a real coding agent can execute:

"Add a `gemma` build variant to the Sirsi Pantheon Go project. There is an existing
`internal/gemma` package that shells out to a local MLX model. We want: (1) a
`//go:build gemma` tag so the deterministic default build links zero AI symbols,
(2) a `make build-gemma` target, (3) the binary's `sirsi version` to report which
variant it is, (4) new `sirsi gemma triage/summarize/ask` subcommands compiled only
under the gemma tag, (5) CI matrix leg that compiles `-tags gemma` on 3 OSes, (6)
a guarantee verified in CI that the deterministic build contains no mlx symbols."

## Result

COMPLETED by local Gemma worker (zero API tokens, on-device).
Task mode: plan

## Gemma Build Variant Plan for Sirsi Pantheon Go

Here's a step-by-step plan for a real coding agent to execute, based on the WORK ITEM from claude-home:

**Step 1: Add `//go:build gemma` tag**

* **What:**  Introduce a `//go:build gemma` tag to the relevant Go files within the `Sirsi Pantheon Go` project. This tag will control which code is compiled when the `gemma` build variant is selected.
* **Why:** This tag allows for conditional compilation, ensuring that the deterministic default build excludes AI-related symbols.
* **Done:** The `//go:build gemma` tag is present in the appropriate files, and the Go compiler will respect this tag during the `gemma` build.

**Step 2: Create `make build-gemma` target**

* **What:** Define a new `make` target named `build-gemma` in the project's `Makefile`. This target should invoke the Go compiler with the `-tags gemma` flag to build the `gemma` variant.
* **Why:** This provides a convenient and repeatable way to build the `gemma` variant.
* **Done:** The `make build-gemma` target is defined and successfully builds the `gemma` variant when executed.

**Step 3: Modify `sirsi version` output**

* **What:** Update the `sirsi version` command to include information about the build variant. It should output something like "Sirsi Pantheon Go (Gemma variant)" when run on the `gemma` build.
* **Why:** This clearly indicates which variant of the binary is being used.
* **Done:** The `sirsi version` command outputs the build variant information when executed on the `gemma` build.

**Step 4: Implement `gemma` subcommands**

* **What:** Create new `sirsi gemma triage`, `sirsi gemma summarize`, and `sirsi gemma ask` subcommands within the `Sirsi Pantheon Go` project. These subcommands should utilize the functionality provided by the `internal/gemma` package to interact with the local MLX model.
* **Why:** This provides the user interface for interacting with the Gemma AI functionality.
* **Done:** The new subcommands are implemented and function correctly, utilizing the `internal/gemma`

---
Note: this is a local-model deliverable. If it feeds a binding decision (merge,
deploy, security sign-off), claude-home must verify before it ships — Gemma is a
drafting/reasoning layer, not the verdict authority.

— gemma (local worker, 2026-06-11T14:06:14Z)
