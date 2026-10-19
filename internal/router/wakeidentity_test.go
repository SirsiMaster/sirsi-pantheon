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
	seen chan struct{}
	once sync.Once
}

func (n *observerFailureNotice) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "heartbeat FAILED; new dispatch held") {
		n.once.Do(func() { close(n.seen) })
	}
	return len(p), nil
}

func TestWakeLoopTransientRegistryReadDoesNotExit(t *testing.T) {
	root := wakeTestRoot(t, AgentConfig{ID: "observer", Type: "gemma"})
	t.Setenv("SIRSI_AGENT_ID", "")
	t.Setenv("SIRSI_THREAD_ID", "")
	notice := &observerFailureNotice{seen: make(chan struct{})}
	previousOutput := log.Writer()
	log.SetOutput(notice)
	defer log.SetOutput(previousOutput)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunWakeLoop(ctx, root, "observer", 5*time.Millisecond) }()
	row := waitForObserver(t, root)
	// Atomic malformed read fixture is hermetic and touches no live service.
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, threadsFilename)
	bad := path + ".fixture"
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(bad, path); err != nil {
		t.Fatal(err)
	}
	// Wait for the fault to be OBSERVED, not an arbitrary short sleep that
	// may restore the file before a slower native registration/OS probe ends.
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
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		got, err := LoadThreadRegistry(root)
		if err == nil && got.Threads[row.ThreadID].LastSeenAt.After(row.LastSeenAt) {
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
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
		PID: os.Getpid(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CloseThread(root, controller.ThreadID); err != nil {
		t.Fatalf("controller cannot close itself: %v", err)
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
