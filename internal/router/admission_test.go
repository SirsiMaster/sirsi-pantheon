package router

import (
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

// TestAdoptRunningConsumerSerializesWithAdmitConsumer is the regression test
// for sirsi-software-admin's PR #1045 finding P1: a standalone startup
// adoption (wake.go's loop-restart path) called the unlocked predecessor of
// adoptRunningConsumer, racing a concurrent admitConsumer (the informer
// admitting/publishing a successor) without sharing the admission flock. The
// startup caller's read-dead-then-unconditional-clear could erase the
// successor's just-published marker out from under it, leaving a live,
// markerless process that the next admission would duplicate.
//
// Fired many times concurrently against a pre-existing dead marker: with the
// fix, adoptRunningConsumer takes the SAME flock admitConsumer does, so
// whichever caller wins completes its own check-then-mutate atomically.
// Confirmed red before this fix by temporarily reverting adoptRunningConsumer
// to call adoptRunningConsumerLocked directly (no lock) — this test then
// intermittently found an empty marker after a live spawn under
// `go test -race -count=20`.
func TestAdoptRunningConsumerSerializesWithAdmitConsumer(t *testing.T) {
	root := t.TempDir()
	const agent = "startup-race-agent"
	logPath := filepath.Join(root, "invocations.log")
	consumer := &ResolvedConsumer{Argv: []string{sleepConsumer(t, logPath)}}

	// A dead marker left behind by a previous run — the shape both the
	// informer's admitConsumer and wake.go's startup adoption see first.
	writeMarker(t, root, agent, 2147480000)

	const racers = 8
	var wg sync.WaitGroup
	wg.Add(racers)
	for i := 0; i < racers; i++ {
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				if _, _, err := admitConsumer(root, agent, consumer); err != nil {
					t.Errorf("admitConsumer: %v", err)
				}
				return
			}
			adoptRunningConsumer(root, agent)
		}(i)
	}
	wg.Wait()

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
		t.Fatalf("invocations.log recorded %d process starts (%v), want exactly 1 real spawn across admitConsumer + adoptRunningConsumer racing the same agent",
			len(lines), lines)
	}

	pid, startedAt, ok := readConsumerPIDFile(root, agent)
	if !ok {
		t.Fatalf("marker missing after the race settled — a startup adoption erased the live successor's marker (finding P1)")
	}
	if PIDStateOf(pid, startedAt) != PIDAlive {
		t.Fatalf("marker (%d, %q) after the race does not name a live process", pid, startedAt)
	}
}

// TestClearConsumerPIDFileIfMatchRefusesToDeleteANewerMarker is codex-pantheon's
// PR #1044 finding P1, reproduced: an old consumer's delayed cleanup goroutine
// fires after a concurrent admission has already recorded a NEWER consumer
// under the same agentID. Confirmed red before this fix — wake.go called the
// unconditional clearConsumerPIDFile here, which deleted the newer marker and
// let the next admission spawn a duplicate beside the live successor.
func TestClearConsumerPIDFileIfMatchRefusesToDeleteANewerMarker(t *testing.T) {
	root := t.TempDir()
	const agent = "racing-agent"

	if err := writeConsumerPIDFile(root, agent, 101, "old-start"); err != nil {
		t.Fatalf("write old marker: %v", err)
	}
	// Simulate: old consumer (101) exited, a concurrent admitConsumer saw it
	// dead and recorded a new live consumer (102) under the same marker path
	// BEFORE the old consumer's delayed cleanup goroutine runs.
	if err := writeConsumerPIDFile(root, agent, 102, "new-start"); err != nil {
		t.Fatalf("write new marker: %v", err)
	}

	// The old consumer's cleanup goroutine now fires, naming its own now-stale
	// signature — it must not touch the newer marker.
	clearConsumerPIDFileIfMatch(root, agent, 101, "old-start")

	pid, startedAt, ok := readConsumerPIDFile(root, agent)
	if !ok {
		t.Fatalf("newer marker was deleted by a stale cleanup for a different consumer")
	}
	if pid != 102 || startedAt != "new-start" {
		t.Fatalf("marker = (%d, %q), want the untouched newer consumer (102, \"new-start\")", pid, startedAt)
	}

	// A cleanup that DOES name the current marker's exact signature still
	// removes it — the guard is precision, not a blanket refusal.
	clearConsumerPIDFileIfMatch(root, agent, 102, "new-start")
	if _, _, ok := readConsumerPIDFile(root, agent); ok {
		t.Fatalf("clearConsumerPIDFileIfMatch left a marker that matched exactly")
	}
}

// TestAdmitConsumerRollsBackDispatchWhenMarkerPublicationFails is
// codex-pantheon's PR #1044 finding P2, reproduced: admitConsumer must not
// report a successful admission when the durable marker write fails, because
// an unrecorded live consumer is indistinguishable from "nothing running" to
// the next caller — it would spawn a duplicate rather than adopt. Confirmed
// red before this fix — the write error was discarded and admitConsumer
// returned success with a live, unmarked process.
func TestAdmitConsumerRollsBackDispatchWhenMarkerPublicationFails(t *testing.T) {
	root := t.TempDir()
	const agent = "unpublishable-agent"

	// Force writeConsumerPIDFile to fail: the marker PATH itself is a
	// non-empty directory, so os.WriteFile returns EISDIR. Non-empty matters —
	// adoptRunningConsumer's stale-marker cleanup (os.Remove) would otherwise
	// silently delete an EMPTY directory there before dispatch ever runs,
	// leaving nothing in the way by the time writeConsumerPIDFile is called.
	markerPath := consumerPIDFilePath(root, agent)
	if err := os.MkdirAll(markerPath, 0o755); err != nil {
		t.Fatalf("seed directory at marker path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(markerPath, "keep-non-empty"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed file inside marker-path directory: %v", err)
	}

	// terminateConsumer's rollback reads consumerKillGrace from the package var
	// at call time; lower it before dispatching so the SIGKILL that proves the
	// rollback ran lands well inside the test timeout instead of the 20s
	// production default.
	oldGrace := consumerKillGrace
	consumerKillGrace = 50 * time.Millisecond
	defer func() { consumerKillGrace = oldGrace }()

	// loginShellArgv (admission.go) wraps Argv in a LOGIN shell ("-lc") when
	// $SHELL is set and executable — its rc-sourcing startup is slow enough
	// that the rollback's SIGTERM, sent the instant dispatch returns, almost
	// always arrives before the wrapped script even execs, let alone installs
	// its own trap. Unset SHELL so the script runs directly via its shebang.
	t.Setenv("SHELL", "")

	// Even without the login-shell detour, there is still a fork/exec/trap
	// race: the rollback's SIGTERM can reach the child before the script's
	// own "trap '' TERM" line has executed, killing it via SIGTERM's default
	// disposition before it ever echoes its pid (observed: intermittent
	// failures below ~150ms, well under consumerKillGrace, meaning TERM
	// killed it, not the KILL that follows the grace). SIG_IGN survives
	// fork+exec unless the new program installs its own handler, so ignoring
	// SIGTERM in this test process before spawning closes the race at its
	// root — the child inherits "ignored" from the instant it exists, before
	// the shell has run a single line.
	signal.Ignore(syscall.SIGTERM)
	defer signal.Reset(syscall.SIGTERM)

	log := filepath.Join(t.TempDir(), "spawned.log")
	// A long sleep (not sleepConsumer's 1s) so the process is still alive at
	// the moment admitConsumer returns, and only the rollback's SIGTERM/KILL
	// explains it dying afterward.
	// Ignores the rollback's first SIGTERM so it survives long enough to
	// record its own pid — the point is to prove the SIGKILL that follows
	// (untrappable) actually lands, not to race the shell's own startup.
	script := filepath.Join(t.TempDir(), "long-sleep.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap '' TERM\necho $$ >> "+log+"\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write long-sleep consumer: %v", err)
	}

	run, adopted, err := admitConsumer(root, agent, &ResolvedConsumer{Argv: []string{script}})
	if err == nil {
		t.Fatalf("admitConsumer returned success with an unpublished marker (run=%+v)", run)
	}
	if adopted {
		t.Fatalf("admitConsumer reported adopted=true on a failed publish")
	}
	if run != nil {
		t.Fatalf("admitConsumer returned a non-nil run on a failed publish: %+v", run)
	}

	// admitConsumer already returned above — by construction (see the comment
	// at the rollback call site) it does not return until the rollback is
	// CONFIRMED dead, not merely signaled. So this checks immediately, with
	// no grace poll: a pid still alive here means admitConsumer released its
	// admission lock before termination actually completed, letting a
	// concurrent/next-tick caller admit a second live, unrecorded consumer
	// during that gap (codex-pantheon review of 1d965e57, finding P1 — "the
	// added author test waits for eventual death, so it does not test this
	// admission interval").
	pidBytes, rerr := os.ReadFile(log)
	if rerr != nil || len(pidBytes) == 0 {
		t.Fatalf("spawned process never recorded its pid in %s: %v", log, rerr)
	}
	spawnedPID, perr := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if perr != nil {
		t.Fatalf("parse spawned pid %q: %v", pidBytes, perr)
	}
	if syscall.Kill(spawnedPID, 0) == nil {
		_ = syscall.Kill(-spawnedPID, syscall.SIGKILL)
		t.Fatalf("spawned pid %d still alive the instant admitConsumer returned — admission lock was released before rollback was confirmed dead", spawnedPID)
	}
}
