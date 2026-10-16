# Fabric reconciliation census

`python3 -B scripts/maat/fabric-reconcile.py > census.json` inspects the current host's pinned lane registry, conventional project roots, and every worktree linked by their Git common directories. Run it separately on each host. It makes no Git changes and never stages, commits, resets, cleans, prunes or deletes work.

`python3 -B -m unittest discover -s scripts/maat -p test_fabric_reconcile.py -v` verifies dirty/clean classification, unusual status names, empty-inventory refusal, and preservation of the real Git index.

Counts are **Git status entries**, not requirements, bytes, or logically distinct files: renames are deliberately represented without expensive rename detection. A clean worktree may still be detached or ahead of its upstream. Missing temporary worktree paths do not prove lost changes; reconcile their retained refs/objects before any administrative cleanup. Repeated worktrees sharing one common directory are enumerated once.

Reports contain private local path metadata and must remain in the operator's private evidence home, not public issues. They never read source contents or credentials. Ignored caches are not source work; private non-project home untracked files are deliberately excluded. A missing registry, unreadable repository, changing HEAD, unfinished Git operation or missing linked worktree prevents a global clean claim. Cloud helpers and other hosts require separate receipts.

Disposition order: observe → identify the real writer and ledger → checkpoint a coherent source unit → validate and scan → commit/publish through normal authority → repeat the census. Never manufacture a clean result by deleting work, hiding a stash, mixing unrelated changes, or closing the parent implementation request after a partial handoff.
