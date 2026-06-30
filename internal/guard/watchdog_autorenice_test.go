package guard

import (
	"context"
	"testing"
	"time"
)

// AutoRenice was dead code: its `hotStreak[p.PID] == 0` guard sat inside the
// `>= SustainCount` block, so it was never 0 there and the opt-in never fired.
// This drives the watcher with a sustained hot process and asserts the (now
// corrected) auto-renice actually runs — exactly once, via the A21-safe seam.
func TestWatchdog_AutoReniceFiresOnSustainedAlert(t *testing.T) {
	saveAndRestoreSampler(t)
	setSampleFn(func(n int) ([]ProcessInfo, error) {
		return []ProcessInfo{
			{PID: 999, Name: "hot-proc", CPUPercent: 95.0, RSS: 1024 * 1024},
		}, nil
	})

	// Capture auto-renice calls through the A21-safe seam (the watch loop reads
	// it; we restore under lock).
	orig := getReniceByPIDFn()
	t.Cleanup(func() { setReniceByPIDFn(orig) })
	reniced := make(chan int, 4)
	setReniceByPIDFn(func(pid int, _ string) error {
		reniced <- pid
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	cfg := WatchConfig{
		Interval:     10 * time.Millisecond,
		CPUThreshold: 90.0,
		SustainCount: 2,
		MaxAlerts:    1,
		SampleSize:   5,
		SelfBudget:   50.0, // high so we don't back off
		AutoRenice:   true, // the opt-in under test
	}
	StartWatch(ctx, cfg)

	select {
	case pid := <-reniced:
		if pid != 999 {
			t.Fatalf("auto-renice fired for pid %d, want 999", pid)
		}
	case <-time.After(800 * time.Millisecond):
		t.Fatal("AutoRenice never fired on a sustained alert — the dead-code guard regressed")
	}
}
