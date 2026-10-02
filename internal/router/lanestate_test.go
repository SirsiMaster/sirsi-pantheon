package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var pingNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func regWith(ts ...*Thread) *ThreadRegistry {
	r := &ThreadRegistry{Threads: map[string]*Thread{}}
	for i, t := range ts {
		if t.ThreadID == "" {
			t.ThreadID = "thr-" + string(rune('a'+i))
		}
		r.Threads[t.ThreadID] = t
	}
	return r
}

func worker(agent string, capable bool, lane *LaneState, age time.Duration) *Thread {
	return &Thread{AgentID: agent, Surface: "worker", Status: ThreadStatusIdle, LastSeenAt: pingNow.Add(-age), ConsumerCapable: capable, Lane: lane}
}

// TestPingLaneVerdicts covers every verdict in both directions (A35): the states
// that CAN work say so, and each state that cannot says exactly why — including
// the one that read "armed" before (a heartbeating loop with no consumer).
func TestPingLaneVerdicts(t *testing.T) {
	ok := &LaneState{ConsumerDeclared: true, LastOutcome: OutcomeOK, LastProgressAt: pingNow.Add(-time.Minute)}
	cases := []struct {
		name    string
		reg     *ThreadRegistry
		cfg     AgentConfig
		want    string
		working bool
		detail  string
	}{
		{"attended session", regWith(&Thread{AgentID: "a", Surface: "claude", Status: ThreadStatusActive, LastSeenAt: pingNow.Add(-time.Minute)}), AgentConfig{}, VerdictLive, true, "attended"},
		{"worker with consumer", regWith(worker("a", true, ok, time.Minute)), AgentConfig{}, VerdictWakeable, true, "made progress"},
		{"worker predating lane state", regWith(worker("a", true, nil, time.Minute)), AgentConfig{}, VerdictWakeable, true, "not proven"},
		{"watch-only loop (heartbeat fresh but no consumer)", regWith(worker("a", false, &LaneState{}, time.Minute)), AgentConfig{}, VerdictWatchOnly, false, "never work it"},
		{"auth required", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, LastOutcome: OutcomeAuthRequired, LastDetail: "Not logged in"}, time.Minute)), AgentConfig{}, VerdictAuthRequired, false, "Not logged in"},
		{"held by window", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, Hold: HoldWindow}, time.Minute)), AgentConfig{}, VerdictHeld, true, "window"},
		{"held by backoff with expiry", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, Hold: HoldBackoff, HoldUntil: pingNow.Add(time.Hour)}, time.Minute)), AgentConfig{}, VerdictHeld, true, "until"},
		{"quarantine is not workable", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, Hold: HoldQuarantine}, time.Minute)), AgentConfig{}, VerdictHeld, false, "needs a human"},
		{"relay unreachable", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, LastOutcome: OutcomeRelay, LastDetail: "64 requests in flight"}, time.Minute)), AgentConfig{}, VerdictHeld, false, "relay"},
		{"stale worker", regWith(worker("a", true, ok, time.Hour)), AgentConfig{}, VerdictUnreachable, false, "last reported"},
		{"no thread, wake none", regWith(), AgentConfig{}, VerdictUnstaffed, false, "no worker path"},
		{"suspended thread does not count", regWith(&Thread{AgentID: "a", Surface: "claude", Status: ThreadStatusSuspended, LastSeenAt: pingNow}), AgentConfig{}, VerdictUnstaffed, false, ""},
		{"closed thread does not count", regWith(&Thread{AgentID: "a", Surface: "claude", Status: ThreadStatusClosed, LastSeenAt: pingNow}), AgentConfig{}, VerdictUnstaffed, false, ""},
		{"someone else's thread is not mine", regWith(worker("b", true, ok, time.Minute)), AgentConfig{}, VerdictUnstaffed, false, ""},
	}
	for _, c := range cases {
		got := PingLane(c.reg, c.cfg, "a", pingNow)
		if got.Verdict != c.want || got.Workable != c.working {
			t.Errorf("%s: got %s workable=%v (%s), want %s workable=%v", c.name, got.Verdict, got.Workable, got.Detail, c.want, c.working)
		}
		if c.detail != "" && !strings.Contains(got.Detail, c.detail) {
			t.Errorf("%s: detail %q missing %q", c.name, got.Detail, c.detail)
		}
	}
}

// TestPingLaneWatchesAnotherInbox: a thread that WATCHES the lane counts for it,
// not only a thread registered under its own id.
func TestPingLaneWatchesAnotherInbox(t *testing.T) {
	w := worker("other", true, &LaneState{ConsumerDeclared: true, LastOutcome: OutcomeOK}, time.Minute)
	w.Watches = []string{"a"}
	if got := PingLane(regWith(w), AgentConfig{}, "a", pingNow); got.Verdict != VerdictWakeable {
		t.Fatalf("watcher thread must serve the lane: %+v", got)
	}
}

func TestKnownFailure(t *testing.T) {
	for tail, want := range map[string]string{
		"Not logged in · Please run /login":                                    OutcomeAuthRequired,
		"Failed to authenticate. API Error: 401 OAuth access token is invalid": OutcomeAuthRequired,
		"spool: 64 requests in flight for this lane; relay stalled?":           OutcomeRelay,
		"something unrelated failed":                                           "",
		"":                                                                     "",
	} {
		if got := KnownFailure(tail); got != want {
			t.Errorf("KnownFailure(%q) = %q, want %q", tail, got, want)
		}
	}
	if ClassifyConsumerFailure("unrecognized") != OutcomeExitedError {
		t.Error("an unrecognized failure must classify as exited_error")
	}
}

// TestCurrentHold: the loop publishes the hold that is actually in force.
func TestCurrentHold(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "rails.lock")
	t.Setenv("MAAT_RAILS_LOCK", lock)
	t.Setenv("HOME", t.TempDir()) // no fabric-quarantine marker
	if h, _ := currentHold("", "a", 0, time.Time{}); h == HoldWindow {
		t.Fatalf("no lock: hold must not be window, got %q", h)
	}
	if err := os.WriteFile(lock, []byte("hermes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if h, _ := currentHold("", "a", 0, time.Time{}); h != HoldWindow {
		t.Fatalf("lock present: hold = %q, want window", h)
	}
	_ = os.Remove(lock)
	until := time.Now().Add(time.Hour)
	if h, u := currentHold("", "a", 1, until); h != HoldBackoff && h != HoldLoad || (h == HoldBackoff && !u.Equal(until)) {
		t.Fatalf("backoff: hold=%q until=%v", h, u)
	}
	if h, _ := currentHold("", "a", wakeLoopFruitlessQuarantine, time.Time{}); h != HoldQuarantine {
		t.Fatalf("fruitless quarantine: hold = %q", h)
	}
}

// TestHeartbeatPublishesLaneState: the lane state a loop publishes survives the
// registry round trip, so a sender on another host reads what the worker wrote.
func TestHeartbeatPublishesLaneState(t *testing.T) {
	tmp := t.TempDir()
	thr, err := RegisterThread(tmp, &Thread{AgentID: "codex-pantheon", Surface: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	want := &LaneState{ConsumerDeclared: true, LastOutcome: OutcomeAuthRequired, LastDetail: "Not logged in", Hold: HoldBackoff}
	if _, err = Heartbeat(tmp, thr.ThreadID, HeartbeatUpdate{Status: ThreadStatusIdle, Lane: want}); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadThreadRegistry(tmp)
	if err != nil {
		t.Fatal(err)
	}
	got := reg.Threads[thr.ThreadID].Lane
	if got == nil || got.LastOutcome != OutcomeAuthRequired || got.LastDetail != "Not logged in" || got.Hold != HoldBackoff || !got.ConsumerDeclared {
		t.Fatalf("lane state lost in the round trip: %+v", got)
	}
	if got.PublishedAt.IsZero() {
		t.Fatal("PublishedAt must be stamped on publish")
	}
	// A heartbeat WITHOUT lane state must not wipe what was published.
	if _, err = Heartbeat(tmp, thr.ThreadID, HeartbeatUpdate{Status: ThreadStatusIdle}); err != nil {
		t.Fatal(err)
	}
	reg, _ = LoadThreadRegistry(tmp)
	if reg.Threads[thr.ThreadID].Lane == nil {
		t.Fatal("a heartbeat with no lane update erased the published lane state")
	}
}

// TestPingLaneJudgesTheBestWorker: a lane with a working loop on one host and a
// NEWER-heartbeating watch-only loop on another is WAKEABLE, not WATCH_ONLY.
func TestPingLaneJudgesTheBestWorker(t *testing.T) {
	good := worker("a", true, &LaneState{ConsumerDeclared: true, LastOutcome: OutcomeOK}, 2*time.Minute)
	watchOnly := worker("a", false, &LaneState{}, 5*time.Second) // newer heartbeat
	for _, reg := range []*ThreadRegistry{regWith(good, watchOnly), regWith(watchOnly, good)} {
		if got := PingLane(reg, AgentConfig{}, "a", pingNow); got.Verdict != VerdictWakeable {
			t.Fatalf("a working worker must outrank a newer watch-only one: %+v", got)
		}
	}
	// And with ONLY the watch-only loop the verdict is still WATCH_ONLY.
	if got := PingLane(regWith(watchOnly), AgentConfig{}, "a", pingNow); got.Verdict != VerdictWatchOnly {
		t.Fatalf("watch-only alone: %+v", got)
	}
}

// TestBindConsumerThread: the worker is told its thread id in the argv/prompt as
// well as the environment, so a sandbox that strips env vars cannot unlink it.
func TestBindConsumerThread(t *testing.T) {
	rc := &ResolvedConsumer{Argv: []string{"codex", "exec", "You are a. Claim with --thread {{thread}} and report thread {{thread}}."}}
	bindConsumerThread(rc, "thr-abc123")
	got := strings.Join(rc.Argv, " ")
	if strings.Contains(got, "{{thread}}") || strings.Count(got, "thr-abc123") != 2 {
		t.Fatalf("placeholder not substituted everywhere: %q", got)
	}
	found := false
	for _, e := range rc.Env {
		if e == "SIRSI_THREAD_ID=thr-abc123" {
			found = true
		}
	}
	if !found {
		t.Fatalf("SIRSI_THREAD_ID not set in env: %v", rc.Env)
	}
}

// A live attended session on the lane holds the headless worker; with none, the
// lane dispatches (both directions).
func TestAttendedSessionHoldsHeadlessDispatch(t *testing.T) {
	t.Setenv("MAAT_RAILS_LOCK", filepath.Join(t.TempDir(), "none"))
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	defer SetLoadAvgFn(nil)
	defer setAttendedLiveFn(nil)

	setAttendedLiveFn(func(_, agent string) bool { return agent == "lane-a" })
	if !attendedSessionOwnsInbox("root", "lane-a", 3) {
		t.Fatal("attended session live: dispatch must be held")
	}
	if h, _ := currentHold("root", "lane-a", 0, time.Time{}); h != HoldAttended {
		t.Fatalf("lane state hold = %q, want %q", h, HoldAttended)
	}
	if attendedSessionOwnsInbox("root", "lane-b", 3) {
		t.Fatal("no attended session on lane-b: it must dispatch")
	}
	if h, _ := currentHold("root", "lane-b", 0, time.Time{}); h == HoldAttended {
		t.Fatal("lane-b wrongly reported attended")
	}
}
