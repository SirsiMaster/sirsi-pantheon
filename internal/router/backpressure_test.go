package router

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestShouldDeferDispatchHighLoadHolds is the "must fail red" direction (A35):
// a load average at/above core count must defer dispatch.
func TestShouldDeferDispatchHighLoadHolds(t *testing.T) {
	defer SetLoadAvgFn(nil)
	SetLoadAvgFn(func() (float64, bool) { return float64(runtime.NumCPU()) + 5, true })

	hold, load, cores := shouldDeferDispatch()
	if !hold {
		t.Fatalf("high load (%v) must defer dispatch against %d cores", load, cores)
	}
}

// TestShouldDeferDispatchLowLoadAllows is the other direction: comfortably
// below core count must not defer.
func TestShouldDeferDispatchLowLoadAllows(t *testing.T) {
	defer SetLoadAvgFn(nil)
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })

	hold, load, _ := shouldDeferDispatch()
	if hold {
		t.Fatalf("low load (%v) must not defer dispatch", load)
	}
}

// TestShouldDeferDispatchUnknownLoadAllows: a read failure is not evidence of
// an overloaded host — it must fail open, not become a silent fabric-wide stall.
func TestShouldDeferDispatchUnknownLoadAllows(t *testing.T) {
	defer SetLoadAvgFn(nil)
	SetLoadAvgFn(func() (float64, bool) { return 0, false })

	hold, _, _ := shouldDeferDispatch()
	if hold {
		t.Fatal("unknown load average must not defer dispatch")
	}
}

// TestFabricDispatchOverloadedRecordsHeal proves the gate is owner-visible
// (task requirement 2), not silent.
func TestFabricDispatchOverloadedRecordsHeal(t *testing.T) {
	defer SetLoadAvgFn(nil)
	drainHeals()
	SetLoadAvgFn(func() (float64, bool) { return float64(runtime.NumCPU()) + 5, true })

	if !fabricDispatchOverloaded("worker-agent", 3) {
		t.Fatal("expected dispatch to be held")
	}
	heals := drainHeals()
	if len(heals) != 1 {
		t.Fatalf("expected exactly one recorded heal, got %v", heals)
	}
}

// TestShouldDeferDispatchThreshold pins the saturation edge: effective load at
// 95% of cores defers, at 50% does not (low-priority background work that
// inflates load average but leaves idle CPU must not starve lanes).
func TestShouldDeferDispatchThreshold(t *testing.T) {
	defer SetLoadAvgFn(nil)
	c := float64(runtime.NumCPU())
	for _, tc := range []struct {
		load float64
		want bool
	}{{c * 0.95, true}, {c * 0.5, false}} {
		SetLoadAvgFn(func() (float64, bool) { return tc.load, true })
		if hold, _, _ := shouldDeferDispatch(); hold != tc.want {
			t.Fatalf("load %.1f of %.0f cores: hold=%v want %v", tc.load, c, hold, tc.want)
		}
	}
}

// TestDefaultLoadReaderReads: the real reader must produce a non-negative value
// on this host (top or sysctl fallback).
func TestDefaultLoadReaderReads(t *testing.T) {
	if v, ok := defaultLoadAvg1m(); !ok || v < 0 {
		t.Fatalf("default reader gave %v, %v", v, ok)
	}
}

// TestSharedBusyCoresMeasuresOncePerTTL: many callers within the TTL share one measurement (the wake loops used to
// run top each, every pass); a stale reading is re-measured once; a held lock serves the previous reading.
func TestSharedBusyCoresMeasuresOncePerTTL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.json")
	clock := time.Unix(1_000_000, 0)
	now := func() time.Time { return clock }
	calls := 0
	measure := func() (float64, bool) { calls++; return float64(calls), true }
	for i := 0; i < 50; i++ {
		if v, ok := sharedBusyCores(path, 15*time.Second, now, measure); !ok || v != 1 {
			t.Fatalf("call %d: got %v,%v want 1,true", i, v, ok)
		}
	}
	if calls != 1 {
		t.Fatalf("50 callers inside the TTL measured %d times, want 1", calls)
	}
	clock = clock.Add(16 * time.Second)
	if v, _ := sharedBusyCores(path, 15*time.Second, now, measure); v != 2 || calls != 2 {
		t.Fatalf("stale reading: got %v after %d measures, want 2 after 2", v, calls)
	}
	clock = clock.Add(16 * time.Second)
	if err := os.WriteFile(path+".lock", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if v, _ := sharedBusyCores(path, 15*time.Second, now, measure); v != 2 || calls != 2 {
		t.Fatalf("refresh in progress elsewhere: got %v after %d measures, want the previous 2 and no new measure", v, calls)
	}
}

// TestSharedBusyCoresIgnoresPlantedSymlink: a symlink at the cache path is never read or written through.
func TestSharedBusyCoresIgnoresPlantedSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "victim")
	if err := os.WriteFile(target, []byte(`{"at":9999999999,"cores":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "busy.json")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	calls := 0
	v, ok := sharedBusyCores(path, 15*time.Second, time.Now, func() (float64, bool) { calls++; return 7, true })
	if !ok || v != 7 || calls != 1 {
		t.Fatalf("planted reading was trusted: got %v,%v after %d measures, want 7,true after 1", v, ok, calls)
	}
	if b, _ := os.ReadFile(target); string(b) != `{"at":9999999999,"cores":0}` {
		t.Fatalf("the symlink target was written through: %s", b)
	}
}
