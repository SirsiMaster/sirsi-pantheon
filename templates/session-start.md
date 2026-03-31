---
description: How to start a new session using the Thoth knowledge system
---

# 𓁟 Thoth Session Start

// turbo-all

## Step 1: Read the Thoth memory file (ALWAYS do this first)
Read `.thoth/memory.yaml` in the project root. This is the compressed project state:
- Identity, version, stats
- Architecture quick reference
- Critical design decisions
- Known limitations and recent changes
- File map of important paths

This replaces reading thousands of lines of source code.

## Step 2: Read the engineering journal (when reasoning matters)
Read `.thoth/journal.md` for timestamped decision entries:
- What was happening (context)
- What we discovered (insight)
- What we chose and why (decision)
- What happened (result)

## Step 3: After making significant changes — update Thoth
1. Update `.thoth/memory.yaml` with new version, stats, decisions, limitations
2. Add a journal entry to `.thoth/journal.md` with context, insight, decision
3. Commit Thoth files with your code changes

## Step 4: For deep dives — check artifacts
Look in `.thoth/artifacts/` for benchmark results, audits, design documents relevant to your current task.
