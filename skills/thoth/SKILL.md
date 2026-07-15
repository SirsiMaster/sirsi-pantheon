---
name: thoth
description: Sync project memory, update journal, and compact context. Run before /compact or anytime you need to preserve session state.
---

You are running the Thoth persistent memory sync for the current project.

Steps:

1. Determine the current working directory and check if a .thoth/ directory exists. If not, run `sirsi thoth init` first.

2. Run `sirsi thoth sync` to update memory.yaml with current codebase facts (module count, test count, line count, binary count, command count).

3. Run `sirsi thoth sync --since "24 hours ago"` to generate journal entries from recent git history.

4. Run `sirsi thoth compact` if the user is about to compress context or if context pressure is high.

5. Run `sirsi thoth status` and report the results to the user.

6. After all syncs complete, hand off to a context operation — Claude cannot run slash
   commands itself, so END the /thoth report by telling the user the exact next command:
   - **DEFAULT: `/compact`** — mid-workstream (the usual case). Warm resume: the summary,
     live watchers, and task list survive. When compacting, preserve: current task state,
     unfinished work items, key decisions this session, and any errors or blockers.
   - **EXCEPTION: `/clear`** — only at a workstream boundary or when the thread is polluted
     by a bad assumption worth forgetting. Cold but sterile; resume rides entirely on the
     continuation file /thoth just wrote.
   /thoth guarantees BOTH are safe (disk is the source of truth); the choice is warm vs sterile.

Report a brief summary of what was synced, the current .thoth/ health status, and finish with
the single recommended command on its own line (default: /compact).
