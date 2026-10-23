package router

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

func waitForObserver(t *testing.T, root string) *Thread {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		reg, err := LoadThreadRegistry(root)
		if err == nil {
			for _, row := range reg.Threads {
				if row.WakeMechanism == WakeLaunchAgent && !row.Status.IsTerminal() && row.Lane != nil {
					return row
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("observer registration not observed")
	return nil
}

func TestWakeLoopTerminalObserverStopsWithoutRevival(t *testing.T) {
	root := wakeTestRoot(t, AgentConfig{ID: "observer", Type: "gemma"})
	t.Setenv("SIRSI_AGENT_ID", "")
	t.Setenv("SIRSI_THREAD_ID", "")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunWakeLoop(ctx, root, "observer", 5*time.Millisecond) }()
	row := waitForObserver(t, root)
	// Simulate an authoritative terminal transition, not a child claiming the
	// observer's shutdown rights. The loop must exit and never rewrite it live.
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	reg.Threads[row.ThreadID].Status = ThreadStatusReaped
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrHeartbeatThreadUnavailable) {
			t.Fatalf("terminal observer result: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("terminal observer kept running")
	}
	reg, err = LoadThreadRegistry(root)
	if err != nil || reg.Threads[row.ThreadID].Status != ThreadStatusReaped {
		t.Fatalf("terminal evidence changed: %+v %v", reg, err)
	}
}

type observerFailureNotice struct {
	agentID   string
	seen      chan struct{}
	writeHeld chan struct{}
	seenOnce  sync.Once
	writeOnce sync.Once
}

func (n *observerFailureNotice) Write(p []byte) (int, error) {
	line := string(p)
	if !strings.Contains(line, "wake-loop "+n.agentID+": heartbeat FAILED; new dispatch held") {
		return len(p), nil
	}
	if strings.Contains(line, "parse threads.json") {
		n.seenOnce.Do(func() { close(n.seen) })
	}
	if strings.Contains(strings.ToLower(line), "permission denied") &&
		(strings.Contains(line, "create temp threads.json") || strings.Contains(line, "replace threads.json")) {
		n.writeOnce.Do(func() { close(n.writeHeld) })
	}
	return len(p), nil
}

func TestWakeLoopTransientRegistryReadDoesNotExit(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permission barrier is unavailable when running as root")
	}
	t.Setenv(routercfg.StoreWakeEnv, "0")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	const agentID = "transient-observer"
	// Registry recovery is independent of native CPU sampling. In particular,
	// a cold test HOME must not run a multi-second host `top` each 5ms tick.
	oldLoad := getLoadAvgFn()
	SetLoadAvgFn(func() (float64, bool) { return 0, true })
	defer SetLoadAvgFn(oldLoad)
	root := wakeTestRoot(t, AgentConfig{
		ID: agentID, Type: "gemma", Repo: t.TempDir(), Workstream: "observer-fixture",
		Wake: WakeConfig{Mechanism: "none"},
	})
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	originalMode := rootInfo.Mode().Perm()
	rootReadOnly := false
	var fixtureFile *os.File
	var validSnapshot []byte
	t.Cleanup(func() {
		if fixtureFile != nil {
			if len(validSnapshot) > 0 {
				if err := fixtureFile.Truncate(0); err != nil {
					t.Errorf("restore registry snapshot length: %v", err)
				} else if _, err := fixtureFile.WriteAt(validSnapshot, 0); err != nil {
					t.Errorf("restore registry snapshot bytes: %v", err)
				} else if err := fixtureFile.Sync(); err != nil {
					t.Errorf("sync restored registry snapshot: %v", err)
				}
			}
			if err := fixtureFile.Close(); err != nil {
				t.Errorf("close fixture registry file: %v", err)
			}
		}
		if rootReadOnly {
			if err := os.Chmod(root, originalMode); err != nil {
				t.Errorf("restore fixture root permissions: %v", err)
			}
		}
	})
	t.Setenv("SIRSI_AGENT_ID", "")
	t.Setenv("SIRSI_THREAD_ID", "")
	notice := &observerFailureNotice{agentID: agentID, seen: make(chan struct{}), writeHeld: make(chan struct{})}
	previousOutput := log.Writer()
	log.SetOutput(notice)
	defer log.SetOutput(previousOutput)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() { defer close(exited); done <- RunWakeLoop(ctx, root, agentID, 5*time.Millisecond) }()
	defer func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(2 * time.Second):
			t.Error("fixture observer did not terminate")
		}
	}()
	row := waitForObserver(t, root)
	path := filepath.Join(root, threadsFilename)
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	rootReadOnly = true
	// Confirm the write barrier using the same legacy atomic-save path before
	// injecting malformed bytes. This failure is not evidence of a read fault.
	select {
	case <-notice.writeHeld:
	case err := <-done:
		t.Fatalf("observer exited instead of holding on the write barrier: %v", err)
	case <-ctx.Done():
		t.Fatal("legacy heartbeat write did not hit the permission barrier")
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	baseline := reg.Threads[row.ThreadID]
	if baseline == nil {
		t.Fatalf("observer registration %q disappeared after write barrier", row.ThreadID)
	}
	baselineLastSeen := baseline.LastSeenAt
	validSnapshot, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fixtureFile, err = os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixtureFile.Truncate(0); err != nil {
		t.Fatal(err)
	}
	if _, err := fixtureFile.WriteAt([]byte("{"), 0); err != nil {
		t.Fatal(err)
	}
	if err := fixtureFile.Sync(); err != nil {
		t.Fatal(err)
	}
	// Only the parse error proves the injected read fault was observed.
	select {
	case <-notice.seen:
	case err := <-done:
		t.Fatalf("observer exited instead of holding on the read fault: %v", err)
	case <-ctx.Done():
		t.Fatal("injected read fault was never observed")
	}
	select {
	case err := <-done:
		t.Fatalf("transient read stopped observer: %v", err)
	default:
	}
	if err := SaveThreadRegistry(root, reg); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("direct stale snapshot save = %v, want permission denied from barrier", err)
	}
	if err := fixtureFile.Truncate(0); err != nil {
		t.Fatal(err)
	}
	if _, err := fixtureFile.WriteAt(validSnapshot, 0); err != nil {
		t.Fatal(err)
	}
	if err := fixtureFile.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := fixtureFile.Close(); err != nil {
		t.Fatal(err)
	}
	fixtureFile = nil
	if err := os.Chmod(root, originalMode); err != nil {
		t.Fatal(err)
	}
	rootReadOnly = false
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("observer exited before recovery completed: %v", err)
		default:
		}
		got, err := LoadThreadRegistry(root)
		if err == nil {
			thread := got.Threads[row.ThreadID]
			if thread != nil && thread.LastSeenAt.After(baselineLastSeen) {
				cancel()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("observer heartbeat did not recover")
}

func TestConsumerCannotCloseLiveWakeController(t *testing.T) {
	oldState, oldStart := getPIDStateFn(), getPIDStartFn()
	setPIDStateFn(func(int) PIDState { return PIDAlive })
	setPIDStartFn(func(int) string { return "controller-start" })
	t.Cleanup(func() { setPIDStateFn(oldState); setPIDStartFn(oldStart) })
	root := t.TempDir()
	controller, err := RegisterThread(root, &Thread{
		AgentID: "fixture", Surface: surfaceWorker, WakeMechanism: WakeLaunchAgent,
		PID: os.Getpid() + 10000, StartTime: "controller-start",
		Lane: &LaneState{ConsumerDeclared: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CloseThread(root, controller.ThreadID); !errors.Is(err, ErrLiveWakeController) {
		t.Fatalf("consumer close error = %v, want live-controller refusal", err)
	}
	reg, _ := LoadThreadRegistry(root)
	if reg.Threads[controller.ThreadID].Status.IsTerminal() {
		t.Fatal("consumer closed a live observer")
	}
	if _, err := Heartbeat(root, controller.ThreadID, HeartbeatUpdate{}); err != nil {
		t.Fatalf("refused close broke controller heartbeat: %v", err)
	}
	setPIDStateFn(func(int) PIDState { return PIDGone })
	if _, err := CloseThread(root, controller.ThreadID); err != nil {
		t.Fatalf("dead controller metadata cannot close: %v", err)
	}
}

func TestWakeControllerCanCloseItself(t *testing.T) {
	root := t.TempDir()
	controller, err := RegisterThread(root, &Thread{
		AgentID: "fixture", Surface: surfaceWorker, WakeMechanism: WakeLaunchAgent,
		PID:  os.Getpid(),
		Lane: &LaneState{ConsumerDeclared: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CloseThread(root, controller.ThreadID); err != nil {
		t.Fatalf("controller cannot close itself: %v", err)
	}
}

// Ma'at registers a short-lived executor before its subprocess runs. Registry
// policy may inherit launchagent as its lane's wake mechanism, but that does
// not turn the executor into a resident observer. Cleanup must not refuse it.
func TestBoundedExecutorWithInheritedWakeMechanismCanClose(t *testing.T) {
	oldState, oldStart := getPIDStateFn(), getPIDStartFn()
	setPIDStateFn(func(int) PIDState { return PIDAlive })
	setPIDStartFn(func(int) string { return "executor-start" })
	t.Cleanup(func() { setPIDStateFn(oldState); setPIDStartFn(oldStart) })
	root := t.TempDir()
	executor, err := RegisterThread(root, &Thread{
		AgentID: "fixture", Surface: surfaceWorker, WakeMechanism: WakeLaunchAgent,
		PID: os.Getpid() + 10000, StartTime: "executor-start",
	})
	if err != nil {
		t.Fatal(err)
	}
	if executor.Lane != nil {
		t.Fatal("bounded executor unexpectedly publishes resident observer state")
	}
	if _, err := CloseThread(root, executor.ThreadID); err != nil {
		t.Fatalf("ordinary executor cleanup falsely classified as wake-controller cleanup: %v", err)
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil || reg.Threads[executor.ThreadID].Status != ThreadStatusClosed {
		t.Fatalf("executor did not close: reg=%+v err=%v", reg, err)
	}
}

func TestHeartbeatObserverIdentityFence(t *testing.T) {
	root := t.TempDir()
	thr, err := RegisterThread(root, &Thread{
		AgentID: "fixture", Surface: surfaceWorker, PID: 90001, StartTime: "generation-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, upd := range []HeartbeatUpdate{
		{ExpectedPID: 90002, ExpectedStart: thr.StartTime},
		{ExpectedPID: 90001, ExpectedStart: "generation-two"},
	} {
		if _, err := Heartbeat(root, thr.ThreadID, upd); !errors.Is(err, ErrHeartbeatThreadUnavailable) {
			t.Fatalf("identity mismatch = %v, want unavailable", err)
		}
	}
	updated, err := Heartbeat(root, thr.ThreadID, HeartbeatUpdate{
		ExpectedPID: thr.PID, ExpectedStart: thr.StartTime,
	})
	if err != nil || updated.PID != thr.PID {
		t.Fatalf("correct identity heartbeat = %+v, %v", updated, err)
	}
	if _, err := CloseThread(root, thr.ThreadID); err != nil {
		t.Fatal(err)
	}
	if _, err := Heartbeat(root, thr.ThreadID, HeartbeatUpdate{}); !errors.Is(err, ErrHeartbeatThreadUnavailable) {
		t.Fatalf("terminal heartbeat = %v, want unavailable", err)
	}
	if _, err := Heartbeat(root, "never-registered", HeartbeatUpdate{}); !errors.Is(err, ErrHeartbeatThreadUnavailable) {
		t.Fatalf("unknown heartbeat = %v, want unavailable", err)
	}
}

func TestAdoptWorkerThreadPIDReuseAndMissingIdentityRefused(t *testing.T) {
	old := getPIDStartFn()
	t.Cleanup(func() { setPIDStartFn(old) })
	setPIDStartFn(func(int) string { return "original" })
	root := t.TempDir()
	seedWorker(t, root, "fixture", 90003, time.Now().UTC())
	for _, start := range []string{"different-generation", ""} {
		setPIDStartFn(func(int) string { return start })
		got, err := adoptWorkerThread(root, "fixture", 90003)
		if err != nil || got != nil {
			t.Fatalf("start %q adopted previous generation: %+v, %v", start, got, err)
		}
	}
}

func TestCloseThreadPreservesOriginalTerminalEvidence(t *testing.T) {
	root := t.TempDir()
	thr, _ := RegisterThread(root, &Thread{AgentID: "fixture", Surface: surfaceWorker})
	reg, _ := LoadThreadRegistry(root)
	row := reg.Threads[thr.ThreadID]
	row.Status, row.LastError = ThreadStatusReaped, "original OS-truth evidence"
	stamp := row.LastSeenAt
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	got, err := CloseThread(root, thr.ThreadID)
	if err != nil || got.Status != ThreadStatusReaped || got.LastError != row.LastError || !got.LastSeenAt.Equal(stamp) {
		t.Fatalf("terminal evidence overwritten: %+v, %v", got, err)
	}
}
