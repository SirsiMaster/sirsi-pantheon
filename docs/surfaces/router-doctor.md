# Observer Contract — `sirsi router doctor`

> Inherits all invariants from [`router-observers-common.md`](router-observers-common.md).
>
> **Surface**: `cmd/sirsi/routerdoctor.go`
>
> **Purpose**: the router's self-check — surfaces unarmed threads, stranded
> inboxes, and stale records so an operator can act.

---

## 1. Data Sources

| Source | What it provides | On failure |
|--------|-----------------|-----------|
| `CollectNodeStatus(repoRoot, nil)` | full fabric snapshot (threads, pending, wake health) | propagate error; doctor aborts |
| `LoadRegistry(routerRoot)` | per-agent wake mechanism config (for `--fix` wake pass) | best-effort; advisory path only |
| `DefaultLaunchctlChecker("list", label)` | whether a per-agent launchd wake job is loaded | best-effort; miss = conservatively armed |

---

## 2. The Armed Predicate — Union, Not Registry-Only

The doctor's loop-dead verdict MUST implement the armed-consumer union from
the common contract §3:

> armed = `AgentArmed(routerRoot, agentID)` **OR** `launchctl list ai.sirsi.router.wake.<agentID>` exits 0

An agent is only loop-dead when **both** legs are false: no armed thread AND
no loaded wake LaunchAgent.

**Background** (D-DOC-1, claude-io 2026-07-31): the previous implementation
checked only the thread registry.  This false-stranded `claude-io` while its
`ai.sirsi.router.wake.claude-io` LaunchAgent was loaded, causing the doctor
to advise peers not to route work to `claude-io` — misrouting real work to a
reachable agent.  For an app-hosted session (CLI respawned each turn) the
LaunchAgent is the only durable consumer available; ignoring it discredits the
very remedy the doctor itself recommends.

---

## 3. Cache

Doctor is **genuinely "none"**: every read is live (`CollectNodeStatus` on
each invocation), no fallback file, no path where a stale value renders as
current.  This is the baseline `board` should be measured against.

---

## 4. `--fix` Pass

`--fix` runs two SAFE, non-destructive repairs:

1. **Reap OS-dead threads** — `ReapDeadThreads` removes only PID-dead records
   (ADR-022 OS-truth); live, suspended, and terminal records are untouched.
2. **Wake-or-declare-unavailable** — `WakePass` wakes agents with a ready,
   non-interactive adapter; records `wake-unavailable` on stranded items whose
   agent has no usable adapter.  Interactive `claude-*` agents are NEVER
   blind-spawned.

No `--fix` action removes items, closes threads, or modifies registry state
beyond the two above.

---

## 5. Compliance Ratchet

The only outstanding obligation is the armed-predicate fix (§2), which is
delivered in the same PR as this contract.  All other common-contract
invariants are met.
