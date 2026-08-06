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
// evidence if its heartbeat is fresh OR a confirmed live agent PID exists for it
// OR a live watcher process (pgrep -f thr-<id> OR pgrep -f <agent>-watcher) exists.
// We do NOT bump LastSeenAt on every inbox tick (option 1) — that would add a
// threads.json write per agent per ~60s, re-introducing the exact mds_stores
// write-amplification → Jetsam storm the health surface (Rail B) exists to eliminate.
//
// PID-alive short-circuit (A27 alarm fix): when the registered PID is a confirmed
// live agent surface (PIDAlive via PIDStateOfThread's cmdline check), the session
// is running under a harness-gated heartbeat — not truly abandoned. pgrep-for-
// watcher is the fallback for surfaces that have no recorded PID.
//
// Re-registration P1 (A27): after thread re-registration the watcher script retains
// the OLD thread id in its argv. WatcherAlive (thread-id keyed) misses it; the agent-
// keyed probe in WatcherAliveByAgent finds it via "<agentID>-watcher" in the script
// name (.sirsi/watchers/<agentID>-watcher.sh), which is stable across re-registrations.
package router

import (
	"os/exec"
	"strings"
	"sync"
	"time"
)

// defaultWatcherAlive probes for a live watcher loop process.
func defaultWatcherAlive(threadID string) bool {
	// Keyed on `pgrep -f thr-<id>` — the SAME (agent_id, pid) watcher identity
	// ADR-024 §3 arms on, and ADR-022 reaps on. Each thread_id is unique, so this
	// matches only that thread's watcher, never another agent's shared loop body.
	out, err := exec.Command("pgrep", "-f", threadID).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// watcherAliveFn is the injectable watcher prober (Rule A16). Guarded by a
// RWMutex (Rule A21) so tests can install stubs without racing any concurrent
// reader — EffectiveStale is reached from goroutine-capable consumers, so a
// bare package-level assignment is a data race, not merely untidy.
var (
	watcherAliveMu sync.RWMutex
	watcherAliveFn = defaultWatcherAlive
)

func getWatcherAliveFn() func(string) bool {
	watcherAliveMu.RLock()
	defer watcherAliveMu.RUnlock()
	return watcherAliveFn
}

// setWatcherAliveFn installs a prober (nil restores the real pgrep probe).
func setWatcherAliveFn(fn func(string) bool) {
	watcherAliveMu.Lock()
	defer watcherAliveMu.Unlock()
	if fn == nil {
		fn = defaultWatcherAlive
	}
	watcherAliveFn = fn
}

// defaultWatcherAliveByAgent probes for a live watcher loop process by agent id.
// Watcher scripts follow the naming convention "<agentID>-watcher.sh" and live in
// ~/.sirsi/watchers/ (e.g. claude-home-watcher.sh). The script name is in argv
// regardless of which thread id the watcher was started with, so this probe is
// stable across re-registrations: after a thread re-registers with a new id the
// existing watcher carries the OLD id (WatcherAlive misses it) but the script name
// is unchanged (WatcherAliveByAgent finds it via "<agentID>-watcher" in the cmdline).
func defaultWatcherAliveByAgent(agentID string) bool {
	if agentID == "" {
		return false
	}
	out, err := exec.Command("pgrep", "-f", agentID+"-watcher").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// watcherAliveByAgentFn is the injectable agent-id watcher prober (Rule A16).
// Guarded by a separate RWMutex (Rule A21) so tests can install stubs without
// racing concurrent readers — same safety contract as watcherAliveFn.
var (
	watcherAliveByAgentMu sync.RWMutex
	watcherAliveByAgentFn = defaultWatcherAliveByAgent
)

func getWatcherAliveByAgentFn() func(string) bool {
	watcherAliveByAgentMu.RLock()
	defer watcherAliveByAgentMu.RUnlock()
	return watcherAliveByAgentFn
}

// setWatcherAliveByAgentFn installs an agent-id prober (nil restores the real probe).
func setWatcherAliveByAgentFn(fn func(string) bool) {
	watcherAliveByAgentMu.Lock()
	defer watcherAliveByAgentMu.Unlock()
	if fn == nil {
		fn = defaultWatcherAliveByAgent
	}
	watcherAliveByAgentFn = fn
}

// pidStateOfThreadFn delegates to PIDStateOfThread (injectable for tests,
// Rule A16 — the real PIDStateOfThread shells out, so tests stub this).
var pidStateOfThreadFn = PIDStateOfThread

// WatcherAlive reports whether a live watcher loop process exists for threadID —
// the surface-agnostic loop-evidence signal. A process surface looping under a
// harness-gated heartbeat is still alive; a non-process surface (mcp/api/webhook,
// no watcher process) returns false here and falls back to heartbeat freshness.
func WatcherAlive(threadID string) bool {
	if threadID == "" {
		return false
	}
	return getWatcherAliveFn()(threadID)
}

// WatcherAliveByAgent reports whether a live watcher loop process exists for agentID.
// It probes for "<agentID>-watcher" in process argv — the naming convention for
// watcher scripts (.sirsi/watchers/<agentID>-watcher.sh). Unlike WatcherAlive (thread-
// id keyed), this probe survives re-registration: the script name is stable while the
// thread id in argv grows stale. Call sites should union both probes so either evidence
// clears the loop-dead verdict.
func WatcherAliveByAgent(agentID string) bool {
	if agentID == "" {
		return false
	}
	return getWatcherAliveByAgentFn()(agentID)
}

// EffectiveStale is the loop-evidence-aware staleness used for the police-trusted
// `.stale` field. A thread is NOT stale when any of these hold:
//  1. Its heartbeat is fresh (IsStale = false).
//  2. Its recorded PID is a confirmed live agent surface (PIDAlive): the session
//     is running under a harness-gated heartbeat, not truly abandoned.
//  3. A live watcher loop process exists for its thread_id (pgrep -f thr-<id>):
//     thread-keyed probe covers steady-state liveness.
//
// For the re-registration case (watcher script carries a stale thread id in argv),
// use EffectiveStaleForNewest at call sites that know which thread is newest for
// its agent — it adds the agent-keyed probe scoped to that one thread (A35).
//
// All checks are read-time and write-free.
func EffectiveStale(t *Thread, now time.Time, window time.Duration) bool {
	return EffectiveStaleForNewest(t, now, window, false)
}

// EffectiveStaleForNewest is EffectiveStale extended with an agent-keyed watcher
// probe that is credited ONLY when isNewestOfAgent=true (A35: scope the check to
// its claim). Re-registration produces exactly one successor per agent; agent-keyed
// evidence (WatcherAliveByAgent) is only valid for that one newest thread — crediting
// it for every non-terminal thread of the agent produces a blanket false-not-stale:
// one live PID 961 vouching for 71 claude-home threads, 70 of which it has no
// evidence about (claude-home review of PR #614, 2026-08-06).
//
// Callers with full registry context compute isNewestOfAgent via
// NewestNonTerminalByAgent. Callers without registry context call EffectiveStale.
func EffectiveStaleForNewest(t *Thread, now time.Time, window time.Duration, isNewestOfAgent bool) bool {
	if t == nil {
		return false
	}
	if !t.IsStale(now, window) {
		return false
	}
	// PID-alive short-circuit: if the registered PID is confirmed live and its
	// command line matches the expected agent surface, the session is running.
	// This eliminates false A27 alarms for claude/codex/gemma sessions that rely
	// on harness-gated heartbeats rather than an external watch-router process.
	if t.PID >= minAgentPID && pidStateOfThreadFn(t) == PIDAlive {
		return false
	}
	if WatcherAlive(t.ThreadID) {
		return false
	}
	if isNewestOfAgent && WatcherAliveByAgent(t.AgentID) {
		return false
	}
	return true
}

// NewestNonTerminalByAgent returns a map from agentID → threadID for the newest
// (by StartedAt) non-terminal thread of each agent. Use this at call sites that
// iterate the full registry to compute the isNewestOfAgent argument for
// EffectiveStaleForNewest: only the newest thread is a plausible beneficiary of
// the agent-keyed watcher probe (A35 — scope the check to the claim).
func NewestNonTerminalByAgent(threads []*Thread) map[string]string {
	newest := make(map[string]string)
	newestAt := make(map[string]time.Time)
	for _, t := range threads {
		if t == nil || t.Status.IsTerminal() {
			continue
		}
		if prev, exists := newestAt[t.AgentID]; !exists || t.StartedAt.After(prev) {
			newest[t.AgentID] = t.ThreadID
			newestAt[t.AgentID] = t.StartedAt
		}
	}
	return newest
}
