package router

import (
	"sync"
	"testing"
	"time"
)

func TestEffectiveStale_LoopEvidence(t *testing.T) {
	now := time.Now().UTC()
	window := 5 * time.Minute
	freshThr := &Thread{ThreadID: "thr-fresh", LastSeenAt: now.Add(-10 * time.Second)}
	staleThr := &Thread{ThreadID: "thr-stale", LastSeenAt: now.Add(-2 * time.Hour)}
	oldPID := pidStateOfThreadFn
	defer func() {
		setWatcherAliveFn(nil)
		setWatcherAliveByAgentFn(nil)
		pidStateOfThreadFn = oldPID
	}()

	// PID state always unknown in baseline tests so only watcher matters.
	pidStateOfThreadFn = func(*Thread) PIDState { return PIDUnknown }
	// Agent-id probe returns nothing in baseline (isolate thread-id behavior).
	setWatcherAliveByAgentFn(func(string) bool { return false })

	// Watcher alive for thr-stale only.
	setWatcherAliveFn(func(id string) bool { return id == "thr-stale" })
	// Fresh heartbeat → never stale, regardless of watcher.
	if EffectiveStale(freshThr, now, window) {
		t.Error("a freshly-heartbeating thread must not be stale")
	}
	// Heartbeat aged out BUT a live watcher exists → loop evidence → NOT stale.
	if EffectiveStale(staleThr, now, window) {
		t.Error("a thread with a live watcher loop must not be reported stale (the false-positive fix)")
	}
	// Heartbeat aged out AND no watcher → genuinely stale.
	setWatcherAliveFn(func(string) bool { return false })
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
	oldPID := pidStateOfThreadFn
	defer func() {
		setWatcherAliveFn(nil)
		setWatcherAliveByAgentFn(nil)
		pidStateOfThreadFn = oldPID
	}()

	// No watch-router watcher present (pgrep would find nothing for either probe).
	setWatcherAliveFn(func(string) bool { return false })
	setWatcherAliveByAgentFn(func(string) bool { return false })

	// PID alive → NOT stale even though heartbeat is old and no watcher.
	pidStateOfThreadFn = func(*Thread) PIDState { return PIDAlive }
	if EffectiveStale(thr, now, window) {
		t.Error("a thread with a live PID must not be reported stale (A27 false-alarm fix)")
	}

	// PID gone → falls through to watcher check → stale (both watchers are false).
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
	oldPID := pidStateOfThreadFn
	defer func() {
		setWatcherAliveFn(nil)
		setWatcherAliveByAgentFn(nil)
		pidStateOfThreadFn = oldPID
	}()

	// PID check should never be called for PID=0.
	pidStateOfThreadFn = func(*Thread) PIDState {
		t.Error("pidStateOfThreadFn must not be called for PID=0")
		return PIDAlive
	}
	setWatcherAliveFn(func(string) bool { return false })
	setWatcherAliveByAgentFn(func(string) bool { return false })
	if !EffectiveStale(thr, now, window) {
		t.Error("PID=0 with no watcher must be stale")
	}
}

func TestWatcherAlive_EmptyThreadID(t *testing.T) {
	if WatcherAlive("") {
		t.Error("empty thread id has no watcher")
	}
}

func TestWatcherAliveByAgent_EmptyAgentID(t *testing.T) {
	if WatcherAliveByAgent("") {
		t.Error("empty agent id has no watcher")
	}
}

// TestEffectiveStale_ReRegistrationCase is the regression fixture for the P1
// re-registration false loop-dead. After a thread re-registers with a new id the
// existing watcher script carries the OLD id in argv: WatcherAlive(new-id) returns
// false. WatcherAliveByAgent must rescue the lane via the stable script-name probe
// — but ONLY when the thread is the newest non-terminal record for its agent (A35).
// Call sites that know this use EffectiveStaleForNewest(..., true).
func TestEffectiveStale_ReRegistrationCase(t *testing.T) {
	now := time.Now().UTC()
	window := 5 * time.Minute
	oldPID := pidStateOfThreadFn
	t.Cleanup(func() {
		setWatcherAliveFn(nil)
		setWatcherAliveByAgentFn(nil)
		pidStateOfThreadFn = oldPID
	})

	pidStateOfThreadFn = func(*Thread) PIDState { return PIDUnknown }

	// Re-registered thread: new id, heartbeat stale, no PID recorded.
	thr := &Thread{
		ThreadID:   "thr-new-abc",
		AgentID:    "claude-home",
		Surface:    "claude",
		LastSeenAt: now.Add(-2 * time.Hour),
	}

	// Thread-id probe finds nothing (old watcher has old-id in argv).
	setWatcherAliveFn(func(string) bool { return false })
	// Agent-id probe finds the script (claude-home-watcher.sh is alive).
	setWatcherAliveByAgentFn(func(agent string) bool { return agent == "claude-home" })

	// isNewestOfAgent=true: this is the newest thread for the agent — agent-keyed
	// evidence is valid evidence for it.
	if EffectiveStaleForNewest(thr, now, window, true) {
		t.Error("newest re-registered thread with live agent-id watcher must NOT be stale (re-registration P1 fix)")
	}
	// isNewestOfAgent=false: bare EffectiveStale (no agent-keyed probe).
	// An older thread of the same agent must NOT be rescued by the agent probe —
	// that would be false-not-stale for 70 threads while one watcher is alive.
	if !EffectiveStaleForNewest(thr, now, window, false) {
		t.Error("older thread of same agent must remain stale even when agent-id watcher is alive (A35 scope)")
	}

	// Once the watcher script also dies, even the newest thread is genuinely stale.
	setWatcherAliveByAgentFn(func(string) bool { return false })
	if !EffectiveStaleForNewest(thr, now, window, true) {
		t.Error("re-registered thread with no watcher at all must be stale")
	}
}

// TestEffectiveStale_TwoThreadsOneAgent is the negative-control test that
// TestEffectiveStale_ReRegistrationCase cannot catch: one agent, two threads.
// The older thread must stay stale; only the newest is rescued by the agent-keyed
// probe. This test fails red with the pre-fix (unscoped) implementation and passes
// green with the scoped one (A35 — claude-home review of PR #614, 2026-08-06).
func TestEffectiveStale_TwoThreadsOneAgent(t *testing.T) {
	now := time.Now().UTC()
	window := 5 * time.Minute
	oldPID := pidStateOfThreadFn
	t.Cleanup(func() {
		setWatcherAliveFn(nil)
		setWatcherAliveByAgentFn(nil)
		pidStateOfThreadFn = oldPID
	})

	pidStateOfThreadFn = func(*Thread) PIDState { return PIDUnknown }
	setWatcherAliveFn(func(string) bool { return false })
	// Agent watcher is alive — one PID 961 vouches for the agent.
	setWatcherAliveByAgentFn(func(agent string) bool { return agent == "claude-home" })

	base := now.Add(-24 * time.Hour) // 24h ago — clearly before StartedAt
	older := &Thread{
		ThreadID:   "thr-old-111",
		AgentID:    "claude-home",
		Surface:    "claude",
		StartedAt:  base,
		LastSeenAt: now.Add(-2 * time.Hour), // stale heartbeat
	}
	newer := &Thread{
		ThreadID:   "thr-new-222",
		AgentID:    "claude-home",
		Surface:    "claude",
		StartedAt:  base.Add(time.Hour),     // registered 1h after older
		LastSeenAt: now.Add(-2 * time.Hour), // stale heartbeat
	}

	threads := []*Thread{older, newer}
	newestByAgent := NewestNonTerminalByAgent(threads)
	// "newer" must be identified as the newest.
	if newestByAgent["claude-home"] != "thr-new-222" {
		t.Fatalf("NewestNonTerminalByAgent: want thr-new-222, got %q", newestByAgent["claude-home"])
	}

	// Older thread: isNewest=false → agent-keyed probe NOT credited → stale.
	olderIsNewest := newestByAgent[older.AgentID] == older.ThreadID
	if !EffectiveStaleForNewest(older, now, window, olderIsNewest) {
		t.Error("older thread must remain stale even when agent-id watcher is alive (A35 scope)")
	}

	// Newer thread: isNewest=true → agent-keyed probe credited → NOT stale.
	newerIsNewest := newestByAgent[newer.AgentID] == newer.ThreadID
	if EffectiveStaleForNewest(newer, now, window, newerIsNewest) {
		t.Error("newest thread with live agent-id watcher must NOT be stale (re-registration P1)")
	}
}

// TestThreadArmed_ReRegistrationCase verifies that threadArmedForNewest credits
// the agent-id probe ONLY when isNewest=true (A35 scope fix). threadArmed
// itself does NOT credit the agent-keyed probe (delegates with isNewest=false)
// — call sites that own the registry use threadArmedForNewest with the computed
// isNewest flag.
func TestThreadArmed_ReRegistrationCase(t *testing.T) {
	now := time.Now().UTC()
	t.Cleanup(func() {
		setWatcherAliveFn(nil)
		setWatcherAliveByAgentFn(nil)
	})

	thr := &Thread{
		ThreadID: "thr-new-xyz",
		AgentID:  "claude-pantheon",
		Surface:  "claude",
	}

	// Thread-id probe misses (old watcher has old id in argv).
	setWatcherAliveFn(func(string) bool { return false })
	// Agent-id probe finds the script.
	setWatcherAliveByAgentFn(func(agent string) bool { return agent == "claude-pantheon" })

	// threadArmed (isNewest=false) must NOT credit agent-keyed probe — A35
	// scoping: the unscoped call would vouch for stale older threads too.
	if threadArmed(thr, now) {
		t.Error("threadArmed must not credit agent-keyed probe (isNewest=false path); use threadArmedForNewest with isNewest=true at call sites that own the registry")
	}

	// threadArmedForNewest(isNewest=true) IS armed — re-registration successor.
	if !threadArmedForNewest(thr, now, true) {
		t.Error("re-registered thread with live agent-id watcher must be armed when isNewest=true")
	}

	// Negative control: both probes dead → not armed even when isNewest.
	setWatcherAliveByAgentFn(func(string) bool { return false })
	if threadArmedForNewest(thr, now, true) {
		t.Error("threadArmedForNewest with no watcher evidence must return false")
	}
}

// Rule A21 falsification for the new watcherAliveByAgentFn. Same race-detection
// pattern as TestWatcherAliveFn_ConcurrentAccessIsRaceFree.
func TestWatcherAliveByAgentFn_ConcurrentAccessIsRaceFree(t *testing.T) {
	t.Cleanup(func() { setWatcherAliveByAgentFn(nil) })

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() { // reader
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = WatcherAliveByAgent("claude-home")
			}
		}
	}()

	wg.Add(1)
	go func() { // writer
		defer wg.Done()
		for i := 0; i < 500; i++ {
			alive := i%2 == 0
			setWatcherAliveByAgentFn(func(string) bool { return alive })
		}
		close(stop)
	}()

	wg.Wait()
}

// Rule A21 falsification. WatcherAlive is reached from goroutine-capable
// consumers while tests install stubs, so the injectable prober must be
// mutex-guarded rather than assigned bare. This test drives a reader and a
// writer concurrently: under the previous `watcherAliveFn = fn` assignment it
// fails immediately with a DATA RACE under -race. It passes only because the
// accessors hold watcherAliveMu.
func TestWatcherAliveFn_ConcurrentAccessIsRaceFree(t *testing.T) {
	t.Cleanup(func() { setWatcherAliveFn(nil) })

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() { // reader
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = WatcherAlive("thr-concurrent")
			}
		}
	}()

	wg.Add(1)
	go func() { // writer
		defer wg.Done()
		for i := 0; i < 500; i++ {
			alive := i%2 == 0
			setWatcherAliveFn(func(string) bool { return alive })
		}
		close(stop)
	}()

	wg.Wait()
}
