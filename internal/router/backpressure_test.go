package router

import (
	"runtime"
	"testing"
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
