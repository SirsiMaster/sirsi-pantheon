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

	old := watcherAliveFn
	defer func() { watcherAliveFn = old }()

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

func TestWatcherAlive_EmptyThreadID(t *testing.T) {
	if WatcherAlive("") {
		t.Error("empty thread id has no watcher")
	}
}
