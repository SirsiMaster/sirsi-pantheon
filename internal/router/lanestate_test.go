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
		{"quarantine is not workable", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, Hold: HoldQuarantine}, time.Minute)), AgentConfig{}, VerdictHeld, false, "no fresh attended session visible in this view"},
		// No lead/attended session these holds defer to is visible here.
		{"no_grant without a live session has no consumer", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, Hold: HoldNoGrant}, time.Minute)), AgentConfig{}, VerdictHeld, false, "no registered session visible in this view"},
		{"attended hold without a live session has no consumer", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, Hold: HoldAttended}, time.Minute)), AgentConfig{}, VerdictHeld, false, "no registered session visible in this view"},
		// What a declared-attended loop actually publishes (wakeLoopConsumerCapable is false for it).
		{"declared-attended loop reads held, not watch-only", regWith(worker("a", false, &LaneState{Hold: HoldAttended}, time.Minute)), AgentConfig{}, VerdictHeld, false, "no registered session visible in this view"},
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

// TestPingLaneIdleSessionIsNotUnregistered: an interactive session with a stale heartbeat keeps every
// verdict as before, but the detail must report only what is observed (the heartbeat gap), never
// "not registered" and never a claim that the session still exists or will read the inbox.
func TestPingLaneIdleSessionIsNotUnregistered(t *testing.T) {
	idle := func() *Thread {
		return &Thread{ThreadID: "thr-idle", AgentID: "a", Surface: "claude", Status: ThreadStatusActive,
			Host: "m5", LastSeenAt: pingNow.Add(-25 * time.Minute)}
	}
	cases := []struct {
		name    string
		reg     *ThreadRegistry
		verdict string
	}{
		{"no_grant hold", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, Hold: HoldNoGrant}, time.Minute), idle()), VerdictHeld},
		{"attended hold", regWith(worker("a", false, &LaneState{Hold: HoldAttended}, time.Minute), idle()), VerdictHeld},
		{"quarantined worker", regWith(worker("a", true, &LaneState{ConsumerDeclared: true, Hold: HoldQuarantine}, time.Minute), idle()), VerdictHeld},
		{"watch-only loop", regWith(worker("a", false, &LaneState{}, time.Minute), idle()), VerdictWatchOnly},
		{"only the idle session", regWith(idle()), VerdictUnreachable},
	}
	for _, c := range cases {
		got := PingLane(c.reg, AgentConfig{}, "a", pingNow)
		if got.Verdict != c.verdict || got.Workable {
			t.Errorf("%s: verdict %s workable=%v, want %s not workable (wording only, no dispatch change)", c.name, got.Verdict, got.Workable, c.verdict)
		}
		want := "thr-idle on m5: last heartbeat " + pingNow.Add(-25*time.Minute).UTC().Format(time.RFC3339Nano) +
			" (age 25m0s; freshness limit 5m0s); heartbeat freshness and inbox consumption are unproven"
		if !strings.Contains(got.Detail, want) || strings.Contains(got.Detail, "not registered") || strings.Contains(got.Detail, "gone away") {
			t.Errorf("%s: detail %q must report the stale heartbeat as observed, without claiming absence or future consumption", c.name, got.Detail)
		}
	}
	// Suspended and terminal rows cannot supply stale attended evidence either.
	for _, status := range []ThreadStatus{ThreadStatusSuspended, ThreadStatusClosed, ThreadStatusReaped} {
		s := idle()
		s.Status = status
		if got := PingLane(regWith(worker("a", true, &LaneState{ConsumerDeclared: true, Hold: HoldNoGrant}, time.Minute), s), AgentConfig{}, "a", pingNow); !strings.Contains(got.Detail, "no registered session visible in this view") || strings.Contains(got.Detail, s.ThreadID) {
			t.Errorf("%s session must keep the no-visible-session wording: %q", status, got.Detail)
		}
	}
}

// Mixed rows use private in-memory registries and a fixed clock. A newer held
// worker never overrides a fresh attended session; a PID/session binding never
// promotes a stale heartbeat. The same Workable result feeds routability.
func TestPingLaneAttendedFreshnessWithNewerHeldWorker(t *testing.T) {
	cases := []struct {
		name     string
		present  bool
		age      time.Duration
		status   ThreadStatus
		verdict  string
		workable bool
		stale    bool
	}{
		{"fresh beats newer held worker", true, 4 * time.Minute, ThreadStatusActive, VerdictLive, true, false},
		{"exact five-minute boundary is fresh", true, DefaultThreadStaleAfter, ThreadStatusActive, VerdictLive, true, false},
		{"one nanosecond beyond boundary is stale", true, DefaultThreadStaleAfter + time.Nanosecond, ThreadStatusActive, VerdictHeld, false, true},
		{"registered seven-minute heartbeat is not absent", true, 7 * time.Minute, ThreadStatusActive, VerdictHeld, false, true},
		{"no attended registration in view", false, 0, "", VerdictHeld, false, false},
		{"fresh closed session excluded", true, time.Minute, ThreadStatusClosed, VerdictHeld, false, false},
		{"fresh reaped session excluded", true, time.Minute, ThreadStatusReaped, VerdictHeld, false, false},
		{"fresh suspended session excluded", true, time.Minute, ThreadStatusSuspended, VerdictHeld, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := worker("a", false, &LaneState{Hold: HoldAttended}, time.Second)
			w.ThreadID = "thr-held-worker"
			rows := []*Thread{w}
			var session *Thread
			stamp := pingNow.Add(-c.age)
			if c.present {
				session = &Thread{ThreadID: "thr-attended", AgentID: "a", Surface: "codex", Status: c.status,
					Host: "fixture-host", LastSeenAt: stamp, PID: os.Getpid(), SessionID: "fixture-session"}
				rows = append(rows, session)
			}
			reg := regWith(rows...)
			cfg := AgentConfig{ID: "a", Attended: true}
			got := PingLane(reg, cfg, "a", pingNow)
			wantThread := w.ThreadID
			if c.workable {
				wantThread = session.ThreadID
			}
			if got.Verdict != c.verdict || got.Workable != c.workable || got.Thread != wantThread {
				t.Fatalf("got %+v, want %s workable=%v thread=%s", got, c.verdict, c.workable, wantThread)
			}
			if c.stale {
				for _, want := range []string{"registered attended codex session thr-attended", stamp.UTC().Format(time.RFC3339Nano),
					"age " + c.age.String(), "freshness limit 5m0s", "heartbeat freshness and inbox consumption are unproven"} {
					if !strings.Contains(got.Detail, want) {
						t.Errorf("detail %q lacks %q", got.Detail, want)
					}
				}
			} else if !c.workable {
				if !strings.Contains(got.Detail, "no registered session visible in this view") || strings.Contains(got.Detail, "thr-attended") {
					t.Errorf("absent/excluded session must not supply attended evidence: %q", got.Detail)
				}
			}
			for _, bad := range []string{"not registered", "needs-owner", "must `sirsi thread register`", "may read the inbox"} {
				if strings.Contains(got.Detail, bad) {
					t.Errorf("unproven absence/action/consumption claim %q in %q", bad, got.Detail)
				}
			}
			unroutable := UnroutableFromRuntime(&Registry{Agents: map[string]AgentConfig{"a": cfg}}, reg, pingNow)
			if value, ok := unroutable["a"]; !ok || value != !c.workable {
				t.Errorf("routability diverges from Workable: %v", unroutable)
			}
			if session != nil && (!session.LastSeenAt.Equal(stamp) || session.Status != c.status || session.PID != os.Getpid()) {
				t.Fatal("passive classification changed the attended binding")
			}
		})
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
	if err := os.WriteFile(lock, []byte("mercury\n"), 0o644); err != nil {
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
