---
description: Start a new Thoth session — read memory, verify build, report context
---

## Session Startup

1. Read `.thoth/memory.yaml` — this is the project state. Do NOT re-read source files the memory already covers.
2. Read `.thoth/journal.md` — understand WHY decisions were made.
3. Read `SPECIFICATION.md` — the Thoth specification.
4. Verify: `node index.js --help` must work.
5. Report context health: 🟢 Healthy / 🟡 Getting Deep / 🔴 Critical.

## Session Wrap

1. Update `.thoth/memory.yaml` with changes made this session.
2. Add a journal entry to `.thoth/journal.md`.
3. Commit and push all changes.
