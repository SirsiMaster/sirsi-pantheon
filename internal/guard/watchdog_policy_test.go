package guard

import (
	"context"
	"testing"
	"time"
)

// ADR-064: AutoRenice holds for governed compute and for a host that is not
// under memory pressure; it still fires for an unknown hog under pressure.

func withPressure(t *testing.T, l PressureLevel) {
	orig := getHostPressureFn()
	t.Cleanup(func() { setHostPressureFn(orig) })
	setHostPressureFn(func() PressureLevel { return l })
}

func TestAutoReniceHeld_ExemptNameAndPressureGate(t *testing.T) {
	cfg := DefaultWatchConfig()
	under := func() PressureLevel { return PressureWarn }
	calm := func() PressureLevel { return PressureNormal }
	unknown := func() PressureLevel { return PressureUnknown }

	for _, name := range []string{"Python", "sne-server", "codex", "clang", "swift-frontend", "tbraw.test", "ioconnect-sleeve"} {
		if why := cfg.autoReniceHeld(name, under); why == "" {
			t.Errorf("%s must be exempt under the default allowlist", name)
		}
	}
	if why := cfg.autoReniceHeld("hot-proc", under); why != "" {
		t.Errorf("hot-proc under pressure must be reniced, got hold %q", why)
	}
	if why := cfg.autoReniceHeld("hot-proc", calm); why == "" {
		t.Error("hot-proc with a calm host must be held (pressure gate)")
	}
	if why := cfg.autoReniceHeld("hot-proc", unknown); why == "" {
		t.Error("unknown pressure must hold, not renice")
	}
	// Empty non-nil list exempts nothing; gate off renices regardless of pressure.
	cfg.AutoReniceExempt = []string{}
	cfg.AutoReniceOnlyUnderPressure = false
	if why := cfg.autoReniceHeld("Python", calm); why != "" {
		t.Errorf("opted-out policy must renice Python, got hold %q", why)
	}
}

func TestWatchdog_AutoReniceHoldsForGovernedCompute(t *testing.T) {
	saveAndRestoreSampler(t)
	setSampleFn(func(n int) ([]ProcessInfo, error) {
		return []ProcessInfo{{PID: 4242, Name: "Python", CPUPercent: 540.0, RSS: 1 << 30}}, nil
	})
	withPressure(t, PressureCritical)
	orig := getReniceByPIDFn()
	t.Cleanup(func() { setReniceByPIDFn(orig) })
	reniced := make(chan int, 4)
	setReniceByPIDFn(func(pid int, _ string) error { reniced <- pid; return nil })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	cfg := DefaultWatchConfig()
	cfg.Interval, cfg.SustainCount, cfg.SelfBudget, cfg.AutoRenice = 10*time.Millisecond, 2, 50.0, true
	w := StartWatch(ctx, cfg)
	<-ctx.Done()
	select {
	case pid := <-reniced:
		t.Fatalf("governed compute (Python) was reniced: pid %d", pid)
	default:
	}
	if _, alerts, _ := w.Stats(); alerts == 0 {
		t.Fatal("the alert itself must still be emitted when the renice is held")
	}
}

func TestWatchdog_AutoReniceHoldsWithoutPressure(t *testing.T) {
	saveAndRestoreSampler(t)
	setSampleFn(func(n int) ([]ProcessInfo, error) {
		return []ProcessInfo{{PID: 999, Name: "hot-proc", CPUPercent: 95.0, RSS: 1 << 20}}, nil
	})
	withPressure(t, PressureNormal)
	orig := getReniceByPIDFn()
	t.Cleanup(func() { setReniceByPIDFn(orig) })
	reniced := make(chan int, 4)
	setReniceByPIDFn(func(pid int, _ string) error { reniced <- pid; return nil })

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	cfg := DefaultWatchConfig()
	cfg.Interval, cfg.SustainCount, cfg.SelfBudget, cfg.AutoRenice = 10*time.Millisecond, 2, 50.0, true
	StartWatch(ctx, cfg)
	<-ctx.Done()
	select {
	case pid := <-reniced:
		t.Fatalf("calm host: hot-proc must not be reniced, got pid %d", pid)
	default:
	}
}

func TestUndoRenice(t *testing.T) {
	var gotNice, gotUndo int
	err := undoReniceWith(4242,
		func(pid, nice int) error { gotNice = nice; return nil },
		func(pid int) error { gotUndo = pid; return nil })
	if err != nil || gotNice != 0 || gotUndo != 4242 {
		t.Fatalf("undo: err=%v nice=%d untaskpolicy pid=%d", err, gotNice, gotUndo)
	}
	if undoReniceWith(1, nil, nil) == nil {
		t.Fatal("pid 1 must be refused")
	}
}
