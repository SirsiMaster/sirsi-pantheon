package router

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// sleepConsumer writes a recorder script that appends its own PID to log on
// start, then sleeps briefly — long enough that two concurrent admitConsumer
// callers racing the flock would both observe it "running" if (and only if)
// the critical section actually serialized them.
func sleepConsumer(t *testing.T, log string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sleep-consumer.sh")
	body := "#!/bin/sh\necho $$ >> " + log + "\nsleep 1\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write consumer: %v", err)
	}
	return path
}

// TestAdmitConsumerNegativeControl6_ConcurrentCallersNeverDuplicateSpawn is
// ADR-065 Phase 1 Negative Control 6 (rs-37 task 4): two concurrent callers of
// the shared admission boundary for the SAME agent must produce exactly one
// real spawn — the loser adopts the winner's pid, it never dispatches its own.
//
// Red-before-green (A35) was verified manually, not as a standing test: with
// lockConsumerAdmission temporarily stubbed to a no-op (no serialization),
// this exact test reproduces >1 spawn under `go test -race -count=20`; with
// the real flock restored it is 1/1 every run. A test that sometimes races
// and sometimes doesn't is not shipped here — it would be flaky CI, not
// evidence.
func TestAdmitConsumerNegativeControl6_ConcurrentCallersNeverDuplicateSpawn(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "invocations.log")
	consumer := &ResolvedConsumer{Argv: []string{sleepConsumer(t, logPath)}}

	const callers = 8
	var wg sync.WaitGroup
	pids := make([]int, callers)
	adopted := make([]bool, callers)
	errs := make([]error, callers)
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()
			run, wasAdopted, err := admitConsumer(root, "race-agent", consumer)
			errs[i] = err
			if run != nil {
				pids[i] = run.pid
			}
			adopted[i] = wasAdopted
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: admitConsumer error: %v", i, err)
		}
	}

	spawned := 0
	firstPID := pids[0]
	for i := 0; i < callers; i++ {
		if !adopted[i] {
			spawned++
		}
		if pids[i] != firstPID {
			t.Fatalf("caller %d returned pid %d, want every caller to agree on the single spawned pid %d",
				i, pids[i], firstPID)
		}
	}
	if spawned != 1 {
		t.Fatalf("got %d real spawns across %d concurrent callers, want exactly 1 (zero-duplicate-spawn threshold, SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md)",
			spawned, callers)
	}

	// Give the fixture's "sleep 1" a moment to exit, then confirm the log —
	// the ground truth of how many processes actually ran — agrees.
	deadline := time.Now().Add(5 * time.Second)
	var lines []string
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(logPath)
		lines = splitNonEmptyLines(string(b))
		if len(lines) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(lines) != 1 {
		t.Fatalf("invocations.log recorded %d process starts (%v), want exactly 1 — a shared PID filename alone is not atomic spawn admission",
			len(lines), lines)
	}
}

func splitNonEmptyLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if line := s[start:i]; len(line) > 0 {
				out = append(out, line)
			}
			start = i + 1
		}
	}
	if line := s[start:]; len(line) > 0 {
		out = append(out, line)
	}
	return out
}

// TestConsumerAdmissionLockPathIsStableAcrossCalls guards the inode-stability
// property admitConsumer's correctness depends on: the lock path must never
// change shape (e.g. by embedding a timestamp or pid) across calls for the
// same (routerRoot, agentID), or concurrent callers would lock different
// files and the critical section would be decorative.
func TestConsumerAdmissionLockPathIsStableAcrossCalls(t *testing.T) {
	root := t.TempDir()
	a := consumerAdmissionLockPath(root, "agent-x")
	b := consumerAdmissionLockPath(root, "agent-x")
	if a != b {
		t.Fatalf("consumerAdmissionLockPath not stable: %q != %q", a, b)
	}
	if a != consumerPIDFilePath(root, "agent-x")+".lock" {
		t.Fatalf("consumerAdmissionLockPath = %q, want the PID marker path + \".lock\"", a)
	}
}

// TestAdmitConsumerAdoptsAnAlreadyRunningConsumerWithoutLock confirms the
// read-only adoption path (no concurrent spawn in flight) still works through
// the lock — a regression here would mean every RunWakeLoop restart
// duplicate-spawns against its own just-recorded marker.
func TestAdmitConsumerAdoptsAnAlreadyRunningConsumerWithoutLock(t *testing.T) {
	root := t.TempDir()
	writeConsumerPIDFile(root, "steady-agent", os.Getpid(), getPIDStartFn()(os.Getpid()))
	t.Cleanup(func() { clearConsumerPIDFile(root, "steady-agent") })

	run, adopted, err := admitConsumer(root, "steady-agent", &ResolvedConsumer{Argv: []string{"/bin/true"}})
	if err != nil {
		t.Fatalf("admitConsumer: %v", err)
	}
	if !adopted {
		t.Fatalf("admitConsumer spawned a new process instead of adopting the recorded live one")
	}
	if run.pid != os.Getpid() {
		t.Fatalf("adopted pid = %d, want this test process's own pid %d", run.pid, os.Getpid())
	}
}
