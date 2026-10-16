# Fabric reconciliation census

## Safe native job and observer launchers

Classification: internal-tool. Users are Sirsi operators and registered execution lanes. The pain is orphan expensive work and repeated unsigned-observer prompts; the workflow is register → admit → supervise → prove cleanup. This reduces interruptions and operational support risk, not a separate priced product. SSA/Ma’at owns the launcher; Ra owns tool-hook integration; Claude Apollo M5 Rail approves Apollo enrollment and private telemetry. No credentials, permission-mode changes, global firewall changes, model downloads or benchmark claims are part of these tools.

Install `admitted-run.py` as `~/.local/bin/sirsi-maat-run` (0700). A caller supplies its **own** registered `SIRSI_AGENT_ID` and `SIRSI_THREAD_ID`, not another lane’s IDs:

```
~/.local/bin/sirsi-maat-run --work exact-work-label --timeout 300 -- /absolute/program argument
```

The actual local host is mapped to m1/m5/studio. The registered process anchor must be in the launcher's ancestry with unchanged kernel start time. A current reservation can be borrowed only with exact resource/holder/work, full share and fresh heartbeat; it is never released by a borrower. If the resource is free, the runner requests its own reservation, refuses any floor/denial/malformed/error reply, heartbeats that own reservation, and releases only its own ID. Child exit, timeout, lost admission and covered interruption lead to process-group termination/reap. Private terminals under `~/.sirsi/evidence/maat-runs/` hash argv rather than copying its content; release failures record debt. No model is loaded by a refusal test. Registration is observation, never automatic impersonation.

These are cooperative operational controls, not an OS security sandbox against same-user tampering, deliberately hidden workloads, escaped descendant sessions or supervisor SIGKILL. A native tool hook and per-runtime adoption must be proven separately. The overall timeout includes bounded metadata-call and cleanup overhead. Do not mistake a receipt file's existence for live admission.

Install `observer-launch.sh` as `~/.local/bin/sirsi-apollo-observer-launch` (0700). Existing observer LaunchAgents prepend `/bin/sh` and this launcher to their unchanged executable/arguments. Only the enrolled executable locations, strict Sirsi Team9D382WV988/identifier requirement and lead-approved CDHash are executable; unsigned or different signed replacements refuse **before** opening a listener. `--verify-only <file>` checks staged/rollback files without executing them. The deployment preserves original binaries/plists. Rollback of the launcher means restoring its original plist while retaining the verified signed binary; never silently restore an unsigned listening service. Approval for the three unchanged private Tailscale endpoints is a separate durable lead decision, not a global firewall exception.

Verification: `python3 -B -m unittest discover -s scripts/maat -p 'test_*.py' -v`, `sh -n scripts/maat/observer-launch.sh`, actual signed/unsigned verify-only controls, actual registration/refusal/success/timeout controls, per-host binary and launcher hashes, strict signature checks, private-listener and peer health readback. Report native hook adoption gaps explicitly; source tests alone do not establish full fleet enforcement.

`python3 -B scripts/maat/fabric-reconcile.py > census.json` inspects the current host's pinned lane registry, conventional project roots, and every worktree linked by their Git common directories. Run it separately on each host. It makes no Git changes and never stages, commits, resets, cleans, prunes or deletes work.

`python3 -B -m unittest discover -s scripts/maat -p test_fabric_reconcile.py -v` verifies dirty/clean classification, unusual status names, empty-inventory refusal, and preservation of the real Git index.

Counts are **Git status entries**, not requirements, bytes, or logically distinct files: renames are deliberately represented without expensive rename detection. A clean worktree may still be detached or ahead of its upstream. Missing temporary worktree paths do not prove lost changes; reconcile their retained refs/objects before any administrative cleanup. Repeated worktrees sharing one common directory are enumerated once.

Reports contain private local path metadata and must remain in the operator's private evidence home, not public issues. They never read source contents or credentials. Ignored caches are not source work; private non-project home untracked files are deliberately excluded. A missing registry, unreadable repository, changing HEAD, unfinished Git operation or missing linked worktree prevents a global clean claim. Cloud helpers and other hosts require separate receipts.

Disposition order: observe → identify the real writer and ledger → checkpoint a coherent source unit → validate and scan → commit/publish through normal authority → repeat the census. Never manufacture a clean result by deleting work, hiding a stash, mixing unrelated changes, or closing the parent implementation request after a partial handoff.
