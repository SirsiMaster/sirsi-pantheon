# FinalWishes claim eligibility disposition

/plan: Read/acknowledge report, claim coordination task, query durable registry, inspect supported eligibility contract and expired-lease dry run, return evidence.
/goal: Distinguish dependencies from demonstrated lease recovery defects without bypassing fences. Agent codex-pantheon; thread thr-b9302c79c6136524. estimated_duration: 15 minutes.

Fresh service registry snapshot: 47 open FinalWishes tasks, 20 blocked, 25 in-progress, 2 pending. Every row has a nonempty blocked_by. For all 47, that value does not resolve to a done same-agent task. The two named internal dependencies (FW-PUBLIC-DEMO-COVERAGE-20261001 and the relevant release work) remain open; the other values are external-reason strings. Supported actionableTaskDependency is blocked_by empty OR exact same-agent dependency done; blocked status is also ineligible. Thus these 47 rows currently fail the dependency/status eligibility contract regardless of lease/session state. A no-claimable-task result cannot establish which additional lease/retry gate also applies.

Dry-run reclaim-expired returned only a claude-pantheon task, no FinalWishes rows. Public task list does not expose lease/session binding or retry counters, so I have not invented a per-row orphan/exhausted-lease diagnosis. Snapshot and dry-run JSON are retained adjacent to this response.

Do not clear customer/auth/provider/native acceptance reasons to force claims. Bounded review tasks whose actual goal is only the already-delivered review need reconciliation by the owning lane: preserve customer-release acceptance in its existing parent, then clear only the obsolete external reason on that bounded review task through task update --blocked-by ""; claim normally; use the supported lease token for evidence completion. If runtime ownership still rejects completion, preserve the exact receipt and retry after qualified repair. No active lease should be stolen or fenced completion bypassed.

PR947's proposed session-remint fix was independently reviewed and returned CHANGES REQUIRED: its fallback permits cross-host same-agent/thread ownership in default log mode. It is not yet a qualified recovery answer. None of the 47 task ids can honestly be advertised as currently actionable from this snapshot. This disposition edits only Pantheon evidence and own task records; no FinalWishes product/registry mutation or acceptance removal.

Own thread register remains sandbox-denied on /bin/ps; inherited heartbeat works. Commits 0; context healthy; product and backlog remain incomplete.
