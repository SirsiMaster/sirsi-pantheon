# `changelog.d/` — one file per entry

Add your changelog entry as a **new file here**, not as a line in `CHANGELOG.md`.

```
changelog.d/YYYYMMDD-short-slug.md
```

The file holds the entry exactly as it would have appeared in `CHANGELOG.md` —
same prose, same depth, same `Refs:` discipline. Nothing about the writing
convention changes. Only the file it lands in changes.

## Why

Every PR used to prepend to `## [Unreleased]` in one shared file, so the moment
any one merged it moved `main` and re-conflicted every sibling PR on the same
lines. On 2026-08-06 that reached **nine conflicting PRs, every one of them
conflicting on `CHANGELOG.md`, and four conflicting on nothing else** — idle
49h+ while otherwise mergeable.

`.gitattributes` sets `CHANGELOG.md merge=union`, and it genuinely works: a
local `git merge-tree` against `main` resolves clean. **GitHub's server-side
merge does not honor it.** Measured the same hour: `merge-tree` said CLEAN
while the GitHub API said `mergeable=CONFLICTING, mergeState=DIRTY` for all
four. That is the worst shape a mitigation can have — it passes on every
machine where an agent verifies it and fails in the only place that decides
whether a PR merges, so the natural conclusion is "already fixed".

Two PRs can never touch the same file here, so the conflict class is gone by
construction rather than resolved once per merge.

## Release

`scripts/changelog-assemble.sh` concatenates these into `CHANGELOG.md` under
`## [Unreleased]`, newest first by filename, and deletes what it consumed.
Run it at release cut, commit the result.

`CHANGELOG.md` remains the published artifact and the historical record. It is
assembled, not hand-edited.
