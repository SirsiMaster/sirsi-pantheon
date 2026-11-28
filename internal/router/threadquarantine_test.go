package router

import (
	"strings"
	"testing"
	"time"
)

func quarThread(id, status string, idle time.Duration) *Thread {
	return &Thread{
		ThreadID: id, AgentID: "agent-" + id, Surface: "service", Status: ThreadStatus(status),
		StartedAt: time.Now().UTC().Add(-48 * time.Hour), LastSeenAt: time.Now().UTC().Add(-idle),
	}
}

func statusOf(t *testing.T, root, id string) *Thread {
	t.Helper()
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	th := reg.Threads[id]
	if th == nil {
		t.Fatalf("thread %s vanished — quarantine must never delete a record", id)
	}
	return th
}

func TestQuarantineThreads(t *testing.T) {
	stubWatcher(t, false)
	tests := []struct {
		name       string
		thread     *Thread
		single     bool
		dryRun     bool
		wantErr    string
		wantStatus ThreadStatus
		wantRows   int
	}{
		{"stale single quarantined", quarThread("a", "active", 3*time.Hour), true, false, "", ThreadStatusQuarantined, 1},
		{"stale blocked keeps prior status", quarThread("b", "blocked", 3*time.Hour), true, false, "", ThreadStatusQuarantined, 1},
		{"fresh single refused", quarThread("c", "active", time.Minute), true, false, "refusing", ThreadStatusActive, 0},
		{"fresh bulk skipped", quarThread("d", "active", time.Minute), false, false, "", ThreadStatusActive, 0},
		{"closed single refused", quarThread("e", "closed", 3*time.Hour), true, false, "only live-but-silent", ThreadStatusClosed, 0},
		{"suspended bulk untouched", quarThread("f", "suspended", 3*time.Hour), false, false, "", ThreadStatusSuspended, 0},
		{"stale bulk quarantined", quarThread("g", "idle", 3*time.Hour), false, false, "", ThreadStatusQuarantined, 1},
		{"dry-run changes nothing", quarThread("h", "active", 3*time.Hour), false, true, "", ThreadStatusActive, 1},
		{"foreign host not touched", func() *Thread { x := quarThread("i", "active", 3*time.Hour); x.Host = "some-other-host-zz"; return x }(), false, false, "", ThreadStatusActive, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeThreads(t, tc.thread)
			id := ""
			if tc.single {
				id = tc.thread.ThreadID
			}
			rows, err := QuarantineThreads(root, id, time.Hour, "tester", "unit", tc.dryRun)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(rows) != tc.wantRows {
				t.Fatalf("rows = %d, want %d (%+v)", len(rows), tc.wantRows, rows)
			}
			got := statusOf(t, root, tc.thread.ThreadID)
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %s, want %s", got.Status, tc.wantStatus)
			}
			if got.Status == ThreadStatusQuarantined {
				if got.Quarantine == nil || got.Quarantine.By != "tester" || got.Quarantine.Reason != "unit" || got.Quarantine.PriorStatus != tc.thread.Status {
					t.Fatalf("audit note wrong: %+v", got.Quarantine)
				}
				if got.LastSeenAt.Sub(tc.thread.LastSeenAt).Abs() > time.Second {
					t.Fatal("quarantine must preserve last_seen_at (the staleness evidence)")
				}
			}
		})
	}
}

func TestQuarantineHeartbeatReleasesAndUnquarantine(t *testing.T) {
	stubWatcher(t, false)
	root := writeThreads(t, quarThread("hb", "idle", 3*time.Hour), quarThread("manual", "blocked", 3*time.Hour))
	if _, err := QuarantineThreads(root, "", time.Hour, "tester", "unit", false); err != nil {
		t.Fatal(err)
	}
	if statusOf(t, root, "hb").Status != ThreadStatusQuarantined {
		t.Fatal("setup: hb not quarantined")
	}

	// A heartbeat from the same thread proves it alive and releases it.
	if _, err := Heartbeat(root, "hb", HeartbeatUpdate{}); err != nil {
		t.Fatal(err)
	}
	hb := statusOf(t, root, "hb")
	if hb.Status != ThreadStatusIdle || hb.Quarantine != nil || time.Since(hb.LastSeenAt) > time.Minute {
		t.Fatalf("heartbeat did not release: status=%s quarantine=%v seen=%s", hb.Status, hb.Quarantine, hb.LastSeenAt)
	}

	// Manual release restores prior status and does NOT fake a fresh heartbeat.
	before := statusOf(t, root, "manual").LastSeenAt
	if _, err := UnquarantineThread(root, "manual"); err != nil {
		t.Fatal(err)
	}
	m := statusOf(t, root, "manual")
	if m.Status != ThreadStatusBlocked || !m.LastSeenAt.Equal(before) {
		t.Fatalf("unquarantine: status=%s seen changed=%v", m.Status, !m.LastSeenAt.Equal(before))
	}
	if _, err := UnquarantineThread(root, "manual"); err == nil {
		t.Fatal("unquarantine of a non-quarantined thread must error")
	}
}

func TestQuarantineReRegisterReleases(t *testing.T) {
	stubWatcher(t, false)
	th := &Thread{ThreadID: "sess", AgentID: "claude-x", Surface: "claude", SessionID: "sid-1", Host: "", Status: ThreadStatusActive,
		StartedAt: time.Now().UTC().Add(-48 * time.Hour), LastSeenAt: time.Now().UTC().Add(-3 * time.Hour)}
	root := writeThreads(t, th)
	if _, err := QuarantineThreads(root, "sess", time.Hour, "tester", "unit", false); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterThread(root, &Thread{AgentID: "claude-x", Surface: "claude", SessionID: "sid-1"}); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, "sess"); got.Status != ThreadStatusActive || got.Quarantine != nil {
		t.Fatalf("re-register did not release: %s %v", got.Status, got.Quarantine)
	}
}

// Negative control: the same fresh-looking record counts as live when active and
// vanishes from attended/armed detection when quarantined.
func TestQuarantinedExcludedFromAttendedAndArmed(t *testing.T) {
	stubWatcher(t, false)
	for _, tc := range []struct {
		status ThreadStatus
		want   bool
	}{{ThreadStatusActive, true}, {ThreadStatusQuarantined, false}} {
		th := attendedThread("claude-home", "claude", 10*time.Second)
		th.Status = tc.status
		root := writeThreads(t, th)
		if got := AttendedSessionLive(root, "claude-home"); got != tc.want {
			t.Errorf("%s: AttendedSessionLive = %v, want %v", tc.status, got, tc.want)
		}
		w := attendedThread("worker-agent", surfaceWorker, 10*time.Second)
		w.Status = tc.status
		if got := AgentArmed(writeThreads(t, w), "worker-agent"); got != tc.want {
			t.Errorf("%s: AgentArmed = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestQuarantinedNeverReaped(t *testing.T) {
	stubWatcher(t, false)
	th := quarThread("dead", "active", 3*time.Hour)
	th.PID = 999999 // not a live process
	root := writeThreads(t, th)
	if _, err := QuarantineThreads(root, "", time.Hour, "tester", "unit", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ReapDeadThreads(root); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, "dead").Status; got != ThreadStatusQuarantined {
		t.Fatalf("reaper touched a quarantined record: %s", got)
	}
}
