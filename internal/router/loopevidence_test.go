package router

import (
	"testing"
	"time"
)

func TestEffectiveStale_LoopEvidence(t *testing.T) {
	now := time.Now().UTC()
	window := 5 * time.Minute
	freshThr := &Thread{ThreadID: "thr-fresh", LastSeenAt: now.Add(-10 * time.Second)}
	staleThr := &Thread{ThreadID: "thr-stale", LastSeenAt: now.Add(-2 * time.Hour)}

	oldWatcher := watcherAliveFn
	oldPID := pidStateOfThreadFn
	defer func() {
		watcherAliveFn = oldWatcher
		pidStateOfThreadFn = oldPID
	}()

	// PID state always unknown in baseline tests so only watcher matters.
	pidStateOfThreadFn = func(*Thread) PIDState { return PIDUnknown }

	// Watcher alive for thr-stale only.
	watcherAliveFn = func(id string) bool { return id == "thr-stale" }

	// Fresh heartbeat → never stale, regardless of watcher.
	if EffectiveStale(freshThr, now, window) {
		t.Error("a freshly-heartbeating thread must not be stale")
	}
	// Heartbeat aged out BUT a live watcher exists → loop evidence → NOT stale.
	if EffectiveStale(staleThr, now, window) {
		t.Error("a thread with a live watcher loop must not be reported stale (the false-positive fix)")
	}
	// Heartbeat aged out AND no watcher → genuinely stale.
	watcherAliveFn = func(string) bool { return false }
	if !EffectiveStale(staleThr, now, window) {
		t.Error("heartbeat-stale with no watcher must be stale")
	}
	// nil is never stale.
	if EffectiveStale(nil, now, window) {
		t.Error("nil thread should not be stale")
	}
}

func TestEffectiveStale_PIDAlivePreventsStale(t *testing.T) {
	// A27 alarm fix: a thread whose registered PID is a confirmed live agent
	// surface must NOT be counted as stale, even without an external watch-router
	// process — it is running under a harness-gated heartbeat.
	now := time.Now().UTC()
	window := 5 * time.Minute
	thr := &Thread{
		ThreadID:   "thr-claude-home",
		AgentID:    "claude-home",
		Surface:    "claude",
		PID:        12345,
		LastSeenAt: now.Add(-30 * time.Minute), // well past stale window
	}

	oldWatcher := watcherAliveFn
	oldPID := pidStateOfThreadFn
	defer func() {
		watcherAliveFn = oldWatcher
		pidStateOfThreadFn = oldPID
	}()

	// No watch-router watcher present (pgrep would find nothing).
	watcherAliveFn = func(string) bool { return false }

	// PID alive → NOT stale even though heartbeat is old and no watcher.
	pidStateOfThreadFn = func(*Thread) PIDState { return PIDAlive }
	if EffectiveStale(thr, now, window) {
		t.Error("a thread with a live PID must not be reported stale (A27 false-alarm fix)")
	}

	// PID gone → falls through to watcher check → stale (watcher is false).
	pidStateOfThreadFn = func(*Thread) PIDState { return PIDGone }
	if !EffectiveStale(thr, now, window) {
		t.Error("a thread with a dead PID and no watcher must be stale")
	}

	// PID mismatched (recycled onto different process) → stale.
	pidStateOfThreadFn = func(*Thread) PIDState { return PIDMismatched }
	if !EffectiveStale(thr, now, window) {
		t.Error("a thread with a mismatched PID and no watcher must be stale")
	}

	// PID unknown (unverifiable) → falls through to watcher check → stale.
	pidStateOfThreadFn = func(*Thread) PIDState { return PIDUnknown }
	if !EffectiveStale(thr, now, window) {
		t.Error("a thread with unknown PID and no watcher must be stale")
	}
}

func TestEffectiveStale_ZeroPIDSkipsPIDCheck(t *testing.T) {
	// PID=0 (phantom/no-PID surface) must skip the PID-alive check and fall
	// through to the watcher check only.
	now := time.Now().UTC()
	window := 5 * time.Minute
	thr := &Thread{
		ThreadID:   "thr-mcp",
		AgentID:    "mcp-server",
		Surface:    "mcp",
		PID:        0,
		LastSeenAt: now.Add(-30 * time.Minute),
	}

	oldWatcher := watcherAliveFn
	oldPID := pidStateOfThreadFn
	defer func() {
		watcherAliveFn = oldWatcher
		pidStateOfThreadFn = oldPID
	}()

	// PID check should never be called for PID=0.
	pidStateOfThreadFn = func(*Thread) PIDState {
		t.Error("pidStateOfThreadFn must not be called for PID=0")
		return PIDAlive
	}
	watcherAliveFn = func(string) bool { return false }
	if !EffectiveStale(thr, now, window) {
		t.Error("PID=0 with no watcher must be stale")
	}
}

func TestWatcherAlive_EmptyThreadID(t *testing.T) {
	if WatcherAlive("") {
		t.Error("empty thread id has no watcher")
	}
}
