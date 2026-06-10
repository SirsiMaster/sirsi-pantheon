// Package router — loopevidence.go
//
// Surface-agnostic loop-evidence for the CTR `.stale` determination (A28).
//
// The registry-police (`registry-police.sh`) trusts the `.stale` field from
// `sirsi thread list --json` and deliberately does NOT reinvent heartbeat math.
// That field was `thread.IsStale(now, window)` = `now.Sub(LastSeenAt) > window`,
// refreshed only by `thread heartbeat` / `RegisterThread`. But a process surface
// whose watcher loop is actively running (pulling its inbox, doing work) is
// *looping* even when its `thread heartbeat` is harness-gated and LastSeenAt has
// aged out — producing the recurring "registered-but-not-looping" false alarm.
//
// The fix is read-time and WRITE-FREE (option 2): a thread counts as having loop
// evidence if its heartbeat is fresh OR a live watcher process exists for its
// thread_id. We do NOT bump LastSeenAt on every inbox tick (option 1) — that
// would add a threads.json write per agent per ~60s, re-introducing the exact
// mds_stores write-amplification → Jetsam storm the health surface (Rail B)
// exists to eliminate.
package router

import (
	"os/exec"
	"strings"
	"time"
)

// watcherAliveFn probes for a live watcher loop process. Injectable (Rule A16)
// so EffectiveStale is testable without spawning pgrep.
var watcherAliveFn = func(threadID string) bool {
	// Keyed on `pgrep -f thr-<id>` — the SAME (agent_id, pid) watcher identity
	// ADR-024 §3 arms on, and ADR-022 reaps on. Each thread_id is unique, so this
	// matches only that thread's watcher, never another agent's shared loop body.
	out, err := exec.Command("pgrep", "-f", threadID).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// WatcherAlive reports whether a live watcher loop process exists for threadID —
// the surface-agnostic loop-evidence signal. A process surface looping under a
// harness-gated heartbeat is still alive; a non-process surface (mcp/api/webhook,
// no watcher process) returns false here and falls back to heartbeat freshness.
func WatcherAlive(threadID string) bool {
	if threadID == "" {
		return false
	}
	return watcherAliveFn(threadID)
}

// EffectiveStale is the loop-evidence-aware staleness used for the police-trusted
// `.stale` field: a thread is stale only when its heartbeat has aged past the
// window AND no live watcher loop exists for it. Read-time, write-free.
func EffectiveStale(t *Thread, now time.Time, window time.Duration) bool {
	if t == nil {
		return false
	}
	if !t.IsStale(now, window) {
		return false
	}
	return !WatcherAlive(t.ThreadID)
}
