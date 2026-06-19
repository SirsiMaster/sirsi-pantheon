package guard

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const gb = int64(1) << 30

// sampleAt builds a MemSample at a given free-percent with a chosen top process.
func sampleAt(freePct float64, top ...MemProc) MemSample {
	return MemSample{TotalRAM: 48 * gb, FreeBytes: int64(freePct / 100 * float64(48*gb)), FreePercent: freePct, Top: top}
}

func TestClassifyMem(t *testing.T) {
	// DefaultMemGovernor thresholds: warn 15, critical 8, emergency 4.
	g := DefaultMemGovernor()
	cases := []struct {
		free float64
		want MemTier
	}{
		{40, TierOK},
		{15.1, TierOK},
		{14.9, TierWarn},
		{8.1, TierWarn},
		{7.9, TierCritical},
		{4.1, TierCritical},
		{3.9, TierEmergency},
		{0.5, TierEmergency},
	}
	for _, c := range cases {
		got := ClassifyMem(sampleAt(c.free), g.WarnPercent, g.CriticalPercent, g.EmergencyPercent)
		if got != c.want {
			t.Errorf("free=%.1f%%: want %v, got %v", c.free, c.want, got)
		}
	}
}

func TestFindRunawaySkipsProtectedAndAgents(t *testing.T) {
	// Largest is WindowServer (protected), then a claude agent, then the real hog.
	s := sampleAt(7,
		MemProc{PID: 10, Name: "WindowServer", RSS: 30 * gb},
		MemProc{PID: 20, Name: "claude", RSS: 20 * gb},
		MemProc{PID: 30, Name: "python", RSS: 12 * gb},
	)
	r := FindRunaway(s)
	if r == nil || r.PID != 30 {
		t.Fatalf("want the python hog (pid 30) as runaway, got %+v", r)
	}
}

func TestFindRunawayNilWhenOnlyProtected(t *testing.T) {
	s := sampleAt(7,
		MemProc{PID: 10, Name: "WindowServer", RSS: 30 * gb},
		MemProc{PID: 20, Name: "codex", RSS: 20 * gb},
	)
	if r := FindRunaway(s); r != nil {
		t.Fatalf("want nil (only protected/agent processes big), got %+v", r)
	}
}

// withSeams swaps the injectable governed-lookup + intervention functions and
// restores them after the test, capturing what the governor decided to do.
type seamCalls struct {
	suspended []int
	resumed   []int
	killed    []int
}

func withSeams(t *testing.T, governed map[int]string, fn func(c *seamCalls)) {
	t.Helper()
	og, os1, or, ok := governedFn, suspendFn, resumeFn, killFn
	defer func() { governedFn, suspendFn, resumeFn, killFn = og, os1, or, ok }()
	c := &seamCalls{}
	governedFn = func() map[int]string { return governed }
	suspendFn = func(pid int, name string) error { c.suspended = append(c.suspended, pid); return nil }
	resumeFn = func(pid int) error { c.resumed = append(c.resumed, pid); return nil }
	killFn = func(pid int, name string) error { c.killed = append(c.killed, pid); return nil }
	fn(c)
}

// TestWarnOnlyNeverIntervenes is the A1 invariant: with Govern=false, Hapi never
// suspends or kills anything — even at emergency — it only surfaces the runaway.
func TestWarnOnlyNeverIntervenes(t *testing.T) {
	withSeams(t, map[int]string{30: "python"}, func(c *seamCalls) {
		og := hapiSampleFn
		defer setHapiSampleFn(og)
		setHapiSampleFn(func() (MemSample, error) {
			return sampleAt(2, MemProc{PID: 30, Name: "python", RSS: 40 * gb}), nil
		})
		g := DefaultMemGovernor()
		g.Govern = false // warn-only
		res, err := g.GovernOnce()
		if err != nil {
			t.Fatal(err)
		}
		if res.Tier != TierEmergency {
			t.Fatalf("want emergency tier, got %v", res.Tier)
		}
		if len(c.suspended)+len(c.killed) != 0 {
			t.Errorf("warn-only must never act: suspended=%v killed=%v", c.suspended, c.killed)
		}
		if res.Runaway == nil || res.Runaway.PID != 30 {
			t.Errorf("warn-only must still surface the runaway, got %+v", res.Runaway)
		}
	})
}

// TestGovernSuspendsGovernedAtCritical: with teeth on, a governed runaway is
// SUSPENDED (reversible) at critical — the layer that stops the 06-18 balloon.
func TestGovernSuspendsGovernedAtCritical(t *testing.T) {
	withSeams(t, map[int]string{30: "python"}, func(c *seamCalls) {
		og := hapiSampleFn
		defer setHapiSampleFn(og)
		setHapiSampleFn(func() (MemSample, error) {
			return sampleAt(7, MemProc{PID: 30, Name: "python", RSS: 20 * gb}), nil
		})
		g := DefaultMemGovernor()
		g.Govern = true
		res, err := g.GovernOnce()
		if err != nil {
			t.Fatal(err)
		}
		if len(c.suspended) != 1 || c.suspended[0] != 30 {
			t.Fatalf("want governed pid 30 suspended at critical, got %v", c.suspended)
		}
		if len(c.killed) != 0 {
			t.Errorf("must NOT kill at critical (suspend is reversible-first), killed=%v", c.killed)
		}
		if len(res.Suspended) != 1 {
			t.Errorf("result must record the suspension, got %+v", res.Suspended)
		}
	})
}

// TestGovernKillsGovernedAtEmergency: only at emergency, and only governed PIDs.
func TestGovernKillsGovernedAtEmergency(t *testing.T) {
	withSeams(t, map[int]string{30: "python"}, func(c *seamCalls) {
		og := hapiSampleFn
		defer setHapiSampleFn(og)
		setHapiSampleFn(func() (MemSample, error) {
			return sampleAt(2, MemProc{PID: 30, Name: "python", RSS: 40 * gb}), nil
		})
		g := DefaultMemGovernor()
		g.Govern = true
		if _, err := g.GovernOnce(); err != nil {
			t.Fatal(err)
		}
		if len(c.killed) != 1 || c.killed[0] != 30 {
			t.Fatalf("want governed pid 30 killed at emergency, got %v", c.killed)
		}
	})
}

// TestRecoveryResumesSuspended: once pressure clears, what Hapi paused is resumed.
func TestRecoveryResumesSuspended(t *testing.T) {
	withSeams(t, map[int]string{30: "python"}, func(c *seamCalls) {
		og := hapiSampleFn
		defer setHapiSampleFn(og)
		g := DefaultMemGovernor()
		g.Govern = true

		// Pass 1: critical → suspend pid 30.
		setHapiSampleFn(func() (MemSample, error) {
			return sampleAt(7, MemProc{PID: 30, Name: "python", RSS: 20 * gb}), nil
		})
		if _, err := g.GovernOnce(); err != nil {
			t.Fatal(err)
		}
		if len(c.suspended) != 1 {
			t.Fatalf("setup: expected a suspension, got %v", c.suspended)
		}

		// Pass 2: back to OK → must resume pid 30.
		setHapiSampleFn(func() (MemSample, error) {
			return sampleAt(40, MemProc{PID: 30, Name: "python", RSS: 2 * gb}), nil
		})
		if _, err := g.GovernOnce(); err != nil {
			t.Fatal(err)
		}
		if len(c.resumed) != 1 || c.resumed[0] != 30 {
			t.Fatalf("want pid 30 resumed on recovery, got %v", c.resumed)
		}
	})
}

// TestNonGovernedNeverActedEvenWithTeeth: a huge NON-governed hog at emergency is
// never killed — teeth apply only to consented (governed) compute.
func TestNonGovernedNeverActedEvenWithTeeth(t *testing.T) {
	withSeams(t, map[int]string{}, func(c *seamCalls) { // nothing governed
		og := hapiSampleFn
		defer setHapiSampleFn(og)
		setHapiSampleFn(func() (MemSample, error) {
			return sampleAt(2, MemProc{PID: 99, Name: "SomeBigApp", RSS: 44 * gb}), nil
		})
		g := DefaultMemGovernor()
		g.Govern = true
		res, err := g.GovernOnce()
		if err != nil {
			t.Fatal(err)
		}
		if len(c.suspended)+len(c.killed) != 0 {
			t.Errorf("non-governed process must never be auto-acted, got suspended=%v killed=%v", c.suspended, c.killed)
		}
		if res.Runaway == nil || res.Runaway.PID != 99 {
			t.Errorf("but it must still be surfaced as the runaway, got %+v", res.Runaway)
		}
	})
}

// TestSuspendResumeRealProcess proves the actual teeth work on a REAL process —
// the injected-seam tests above verify the governor's DECISIONS, this verifies the
// SIGSTOP/SIGCONT syscalls themselves halt and resume a process. It spawns a child
// `sleep` (A1-safe: our own throwaway), suspends it (OS state → T = stopped),
// then resumes it (state → S/R). If this ever regresses, "suspend a runaway before
// Jetsam" is a lie.
func TestSuspendResumeRealProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn sleep: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	state := func() string {
		out, _ := exec.Command("ps", "-o", "state=", "-p", strconv.Itoa(pid)).Output()
		return strings.TrimSpace(string(out))
	}

	if err := hapiSuspend(pid, "sleep"); err != nil {
		t.Fatalf("hapiSuspend failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if s := state(); !strings.HasPrefix(s, "T") {
		t.Fatalf("after SIGSTOP, want stopped state T, got %q", s)
	}

	if err := hapiResume(pid); err != nil {
		t.Fatalf("hapiResume failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if s := state(); strings.HasPrefix(s, "T") {
		t.Fatalf("after SIGCONT, process still stopped (state %q)", s)
	}
}

// TestSuspendRefusesProtected proves the A1 gate: even with a real pid, Hapi
// refuses to suspend a protected/agent-named process.
func TestSuspendRefusesProtected(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn sleep: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	if err := hapiSuspend(pid, "WindowServer"); err == nil {
		t.Error("must refuse to suspend a process named like WindowServer (protected)")
	}
	if err := hapiSuspend(pid, "claude"); err == nil {
		t.Error("must refuse to suspend a live agent (claude)")
	}
	// The same real pid under a non-protected name is allowed.
	if err := hapiSuspend(pid, "sleep"); err != nil {
		t.Errorf("should allow suspending a non-protected process: %v", err)
	}
	_ = hapiResume(pid)
}
