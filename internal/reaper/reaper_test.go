package reaper

import (
	"testing"
)

const claudeCmd = "claude-code/1.2.3/claude.app/Contents/MacOS/claude --output-format stream-json"

// procTable models: caller sirsi(pid 500) ← claude session(400, the OWNER'S LIVE
// conversation) ← Claude.app(300) ← launchd(1); plus two leaked claude sessions
// (700, 800) NOT in that chain, and one fresh/small claude session (900) below
// the floors.
func procTable() []Proc {
	return []Proc{
		{PID: 1, PPID: 0, Command: "/sbin/launchd"},
		{PID: 300, PPID: 1, Command: "/Applications/Claude.app/Contents/MacOS/Claude"},
		{PID: 400, PPID: 300, AgeSec: 5000, RSSMB: 400, Command: claudeCmd}, // OWNER'S LIVE SESSION
		{PID: 500, PPID: 400, AgeSec: 3, RSSMB: 30, Command: "sirsi reap-sessions"},
		{PID: 700, PPID: 1, AgeSec: 4000, RSSMB: 300, Command: claudeCmd}, // leaked
		{PID: 800, PPID: 1, AgeSec: 9000, RSSMB: 250, Command: claudeCmd}, // leaked
		{PID: 900, PPID: 1, AgeSec: 60, RSSMB: 200, Command: claudeCmd},   // fresh (age < floor)
		{PID: 950, PPID: 1, AgeSec: 9000, RSSMB: 40, Command: claudeCmd},  // tiny (rss < floor)
	}
}

// TestSafetyContract_NeverReapsCallerAncestry is THE regression guard: the
// caller's live claude session (400) — reachable from sirsi(500) up the ppid
// chain — must NEVER be selected, no matter how old or large.
func TestSafetyContract_NeverReapsCallerAncestry(t *testing.T) {
	procs := procTable()
	protected := AncestrySet(500, procs)

	// The owner's live session and every ancestor must be protected.
	for _, pid := range []int{500, 400, 300} {
		if !protected[pid] {
			t.Fatalf("ancestry pid %d must be protected, protected=%v", pid, protected)
		}
	}
	if protected[1] {
		t.Error("launchd (pid 1) must not be in the protected set (walk stops at 1)")
	}

	kill, _ := selectCandidates(procs, protected, Options{})
	for _, pid := range kill {
		if protected[pid] {
			t.Fatalf("SAFETY VIOLATION: protected pid %d appeared in kill set %v", pid, kill)
		}
		if pid == 400 {
			t.Fatalf("SAFETY VIOLATION: the owner's live session (400) was selected — %v", kill)
		}
	}
	// Exactly the two genuinely-leaked sessions, sorted.
	if len(kill) != 2 || kill[0] != 700 || kill[1] != 800 {
		t.Errorf("kill set = %v, want [700 800] (leaked only; fresh 900 and tiny 950 below floors)", kill)
	}
}

// TestPlan_DryRunMutatesNothing asserts a dry-run selects but never calls Kill.
func TestPlan_DryRunMutatesNothing(t *testing.T) {
	killed := 0
	deps := Deps{
		Procs:   func() ([]Proc, error) { return procTable(), nil },
		SelfPID: 500,
		Kill:    func(pid, sig int) error { killed++; return nil },
		Alive:   func(pid int) bool { return true },
	}
	res, err := Run(Options{Apply: false}, deps) // dry-run
	if err != nil {
		t.Fatal(err)
	}
	if killed != 0 {
		t.Errorf("dry-run called Kill %d times, want 0", killed)
	}
	if len(res.Candidates) != 2 {
		t.Errorf("candidates = %v, want 2 leaked", res.Candidates)
	}
}

// TestRun_ApplyKillsOnlyLeaked confirms --apply SIGTERMs exactly the leaked
// sessions and never a protected PID, and Force SIGKILLs a survivor.
func TestRun_ApplyKillsOnlyLeaked(t *testing.T) {
	var termed, killed []int
	// 800 ignores SIGTERM (stays alive) until SIGKILL.
	dead := map[int]bool{}
	deps := Deps{
		Procs:   func() ([]Proc, error) { return procTable(), nil },
		SelfPID: 500,
		Kill: func(pid, sig int) error {
			switch sig {
			case 15:
				termed = append(termed, pid)
				if pid != 800 {
					dead[pid] = true // 700 dies on TERM; 800 resists
				}
			case 9:
				killed = append(killed, pid)
				dead[pid] = true
			}
			return nil
		},
		Alive: func(pid int) bool { return !dead[pid] },
	}
	res, err := Run(Options{Apply: true, Force: true}, deps)
	if err != nil {
		t.Fatal(err)
	}
	for _, pid := range termed {
		if pid == 400 {
			t.Fatal("SAFETY VIOLATION: SIGTERM sent to the owner's live session")
		}
	}
	if len(termed) != 2 {
		t.Errorf("SIGTERM sent to %v, want the 2 leaked", termed)
	}
	if len(killed) != 1 || killed[0] != 800 {
		t.Errorf("SIGKILL sent to %v, want [800] (the TERM-resistant survivor)", killed)
	}
	if len(res.Survivors) != 0 {
		t.Errorf("survivors = %v, want none after force", res.Survivors)
	}
}
