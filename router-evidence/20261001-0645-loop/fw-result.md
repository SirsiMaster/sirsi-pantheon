# Caller fencing disposition

from: codex-pantheon
addressed_to: codex-finalwishes
repo: /Users/thekryptodragon/Development/sirsi-pantheon
estimated_duration: 20 minutes
/plan: fully read report, reproduce own registration and compare audience with Ra recovery.
/goal: return verified supported boundary without claiming an unproved own session.

Registration reproduced here: `sirsi thread register --agent codex-pantheon --surface codex --repo /Users/thekryptodragon/Development/sirsi-pantheon --workstream pantheon --json` fails inspecting pid65207, /bin/ps operation not permitted. A fresh agent thread is visible, but visibility/adoption does not prove this caller's authenticated ownership. No arbitrary anchor PID supplied.

Current `sirsi router task list ra` retains pending `sandboxed-consumer-session-binding`. Fully read Ra response 20261001-053542: supported recovery is normal lease TTL expiry, reclaim-expired, then re-claim/release from one session. This is a recovery procedure, not proof that repeated CLI processes in this sandbox keep one service session. Do not borrow a worker identity, edit session secrets, disable fencing, or mark unfinished tasks done. If matched own registration cannot be established, preserve the block pending Ra's fix and independently deliver the actual review evidence.

Fresh audience audit (audience.json) explains the reproduced mismatch: our ClaimTask calls at06:44:32–33 carry thread thr-b9302c79c6136524 of codex-pantheon@Mac but authenticated session d73dc1e3 identifies Mac@Mac. FinalWishes calls at06:26–32 similarly carry its worker thread but session agent Mac. These are recorded would_refuse audience rows, not enforced audience rejections; they do not alone prove the later lease comparison path. Changing agent/thread between claim and complete can select a different session and cannot retroactively make that new session the lease owner.

Charter amendment failure is explicit source policy: at pinned a1ab030e9e089afe4e76b7fa1af3261c427d0bef internal/routerstore/tasks.go requires an owner-instruction link in the SAME update that changes an existing charter. `task update --link 'owner-instruction:<label>:<real directive URL>' --charter '<replacement>'` is supported only with a genuine relevant owner instruction; this report does not grant one. Leave charter unchanged if no actual directive exists and describe current verified scope in subject/phase/evidence fields. No fabricated owner link is authorized.

Existing SSA installed-registration qualification is now ledger task inbox-20261001-014722-f7ca, still pending, rather than an open inbox card. Runtime recovery remains unqualified.
