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
	if ClassifyConsumerFailure("unrecognised") != OutcomeExitedError {
		t.Error("an unrecognised failure must classify as exited_error")
	}
}

// TestCurrentHold: the loop publishes the hold that is actually in force.
func TestCurrentHold(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "rails.lock")
	t.Setenv("MAAT_RAILS_LOCK", lock)
	t.Setenv("HOME", t.TempDir()) // no fabric-quarantine marker
	if h, _ := currentHold("a", 0, time.Time{}); h == HoldWindow {
		t.Fatalf("no lock: hold must not be window, got %q", h)
	}
	if err := os.WriteFile(lock, []byte("hermes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if h, _ := currentHold("a", 0, time.Time{}); h != HoldWindow {
		t.Fatalf("lock present: hold = %q, want window", h)
	}
	_ = os.Remove(lock)
	until := time.Now().Add(time.Hour)
	if h, u := currentHold("a", 1, until); h != HoldBackoff && h != HoldLoad || (h == HoldBackoff && !u.Equal(until)) {
		t.Fatalf("backoff: hold=%q until=%v", h, u)
	}
	if h, _ := currentHold("a", wakeLoopFruitlessQuarantine, time.Time{}); h != HoldQuarantine {
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
