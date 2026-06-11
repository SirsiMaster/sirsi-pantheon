package router

// ADR-024 Amendment 1 — (3) reap-key composite identity (pid, start_time).
// These exercise the recycled-PID defense and, critically, the no-false-reap
// guarantee on a genuinely live thread (the regression that reaped a live
// session this session because liveness keyed on a bare PID).

import "testing"

// probeStubs installs alive/start probers and returns a restore func. No
// goroutines read these probers in these synchronous tests, so a deferred
// restore is safe (Rule A21's defer caveat applies only to goroutine readers).
func probeStubs(t *testing.T, state PIDState, start string) func() {
	t.Helper()
	oldState, oldStart, oldCommand := getPIDStateFn(), getPIDStartFn(), getPIDCommandFn()
	setPIDStateFn(func(int) PIDState { return state })
	setPIDStartFn(func(int) string { return start })
	setPIDCommandFn(func(int) string { return "" })
	return func() { setPIDStateFn(oldState); setPIDStartFn(oldStart); setPIDCommandFn(oldCommand) }
}

func TestPIDStateOf_Composite(t *testing.T) {
	cases := []struct {
		name      string
		liveState PIDState
		liveStart string
		recorded  string
		want      PIDState
		dead      bool
	}{
		{"recycled: alive but start mismatch", PIDAlive, "Mon Jun 2 02:00", "Mon Jun 1 09:00", PIDRecycled, true},
		{"live: alive and start matches", PIDAlive, "Mon Jun 2 02:00", "Mon Jun 2 02:00", PIDAlive, false},
		{"legacy: empty recorded start falls back to bare-pid alive", PIDAlive, "Mon Jun 2 02:00", "", PIDAlive, false},
		{"unreadable live start never false-reaps a live pid", PIDAlive, "", "Mon Jun 2 02:00", PIDAlive, false},
		{"gone short-circuits before start check", PIDGone, "anything", "Mon Jun 2 02:00", PIDGone, true},
		{"defunct short-circuits before start check", PIDDefunct, "anything", "Mon Jun 2 02:00", PIDDefunct, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			restore := probeStubs(t, c.liveState, c.liveStart)
			defer restore()
			got := PIDStateOf(4242, c.recorded)
			if got != c.want {
				t.Errorf("PIDStateOf = %q, want %q", got, c.want)
			}
			if DeadByOSTruth(got) != c.dead {
				t.Errorf("DeadByOSTruth(%q) = %v, want %v", got, DeadByOSTruth(got), c.dead)
			}
		})
	}
	// PIDUnknown below the agent PID floor regardless of probers. PID 1 is
	// launchd/init, not a valid agent-thread anchor.
	for _, pid := range []int{0, 1} {
		if got := PIDStateOf(pid, "x"); got != PIDUnknown {
			t.Errorf("PIDStateOf(%d) = %q, want unknown", pid, got)
		}
	}
}

// A reaped recycled PID: the recorded process is gone, a different process holds
// the number. The record must be reaped as recycled, never left active.
func TestReapDeadThreads_RecycledPIDReaped(t *testing.T) {
	root := t.TempDir()
	host := "h1"
	thr, err := RegisterThread(root, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", PID: 4242, Host: host, StartTime: "Mon Jun 1 09:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	restore := probeStubs(t, PIDAlive, "Mon Jun 2 02:00") // alive, but DIFFERENT start
	defer restore()

	reaped, err := ReapDeadThreads(root, host)
	if err != nil {
		t.Fatalf("ReapDeadThreads: %v", err)
	}
	if len(reaped) != 1 || reaped[0].ThreadID != thr.ThreadID || reaped[0].State != PIDRecycled {
		t.Fatalf("reaped = %+v, want one recycled record for %s", reaped, thr.ThreadID)
	}
	reg, _ := LoadThreadRegistry(root)
	if reg.Threads[thr.ThreadID].Status != ThreadStatusReaped {
		t.Errorf("status = %q, want reaped", reg.Threads[thr.ThreadID].Status)
	}
}

// The no-false-reap guarantee: a genuinely live thread whose recorded start
// still matches the live process MUST NOT be reaped. This is the exact
// regression the composite key fixes.
func TestReapDeadThreads_LiveMatchingStartSurvives(t *testing.T) {
	root := t.TempDir()
	host := "h1"
	thr, err := RegisterThread(root, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", PID: 4242, Host: host, StartTime: "Mon Jun 2 02:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	restore := probeStubs(t, PIDAlive, "Mon Jun 2 02:00") // alive AND matching start
	defer restore()

	reaped, err := ReapDeadThreads(root, host)
	if err != nil {
		t.Fatalf("ReapDeadThreads: %v", err)
	}
	if len(reaped) != 0 {
		t.Fatalf("reaped a live thread: %+v (no-false-reap violated)", reaped)
	}
	reg, _ := LoadThreadRegistry(root)
	if reg.Threads[thr.ThreadID].Status != ThreadStatusActive {
		t.Errorf("status = %q, want active (live thread must survive)", reg.Threads[thr.ThreadID].Status)
	}
}

func TestReapDeadThreads_LivePIDWrongCommandReaped(t *testing.T) {
	root := t.TempDir()
	host := "h1"
	thr, err := RegisterThread(root, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", PID: 4242, Host: host, StartTime: "Mon Jun 2 02:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	restore := probeStubs(t, PIDAlive, "Mon Jun 2 02:00")
	defer restore()
	setPIDCommandFn(func(int) string { return "/bin/zsh" })

	reaped, err := ReapDeadThreads(root, host)
	if err != nil {
		t.Fatalf("ReapDeadThreads: %v", err)
	}
	if len(reaped) != 1 || reaped[0].ThreadID != thr.ThreadID || reaped[0].State != PIDMismatched {
		t.Fatalf("reaped = %+v, want one mismatched record for %s", reaped, thr.ThreadID)
	}
	reg, _ := LoadThreadRegistry(root)
	if reg.Threads[thr.ThreadID].Status != ThreadStatusReaped {
		t.Errorf("status = %q, want reaped", reg.Threads[thr.ThreadID].Status)
	}
}

func TestReapDeadThreads_LivePIDMatchingCommandSurvives(t *testing.T) {
	root := t.TempDir()
	host := "h1"
	thr, err := RegisterThread(root, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", PID: 4242, Host: host, StartTime: "Mon Jun 2 02:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	restore := probeStubs(t, PIDAlive, "Mon Jun 2 02:00")
	defer restore()
	setPIDCommandFn(func(int) string { return "/Applications/Claude.app/Contents/MacOS/claude --resume abc" })

	reaped, err := ReapDeadThreads(root, host)
	if err != nil {
		t.Fatalf("ReapDeadThreads: %v", err)
	}
	if len(reaped) != 0 {
		t.Fatalf("reaped a matching live command: %+v", reaped)
	}
	reg, _ := LoadThreadRegistry(root)
	if reg.Threads[thr.ThreadID].Status != ThreadStatusActive {
		t.Errorf("status = %q, want active", reg.Threads[thr.ThreadID].Status)
	}
}

// Register's idempotency fast-path is composite-keyed: a re-register on the same
// (agent_id, pid) with a DIFFERENT live start mints a fresh thread (the old PID
// was recycled), while a matching start reuses the record.
func TestRegisterThread_CompositeFastPath(t *testing.T) {
	root := t.TempDir()
	// Seed an existing record with a known start signature.
	first, err := RegisterThread(root, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", PID: 4242, StartTime: "Mon Jun 1 09:00",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Re-register same (agent, pid), but the live PID now reports a NEW start →
	// recycled → mint fresh, don't adopt the stale record.
	restoreNew := probeStubs(t, PIDAlive, "Mon Jun 2 02:00")
	fresh, err := RegisterThread(root, &Thread{AgentID: "claude-pantheon", Surface: "claude", PID: 4242})
	restoreNew()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ThreadID == first.ThreadID {
		t.Error("recycled PID must mint a fresh thread, not adopt the stale (pid) record")
	}

	// Re-register matching the fresh record's start → reuse (idempotent).
	restoreSame := probeStubs(t, PIDAlive, "Mon Jun 2 02:00")
	again, err := RegisterThread(root, &Thread{AgentID: "claude-pantheon", Surface: "claude", PID: 4242})
	restoreSame()
	if err != nil {
		t.Fatal(err)
	}
	if again.ThreadID != fresh.ThreadID {
		t.Errorf("matching start must reuse the record (idempotent): got %s, want %s", again.ThreadID, fresh.ThreadID)
	}
}
