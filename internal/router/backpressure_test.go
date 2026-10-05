package router

import (
	"runtime"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

func normalPressure() (guard.PressureLevel, string) { return guard.PressureNormal, "test" }

// TestShouldDeferDispatchHighLoadHolds is the "must fail red" direction (A35):
// a load average at/above core count must defer dispatch.
func TestShouldDeferDispatchHighLoadHolds(t *testing.T) {
	defer SetLoadAvgFn(nil)
	defer SetPressureFn(nil)
	SetLoadAvgFn(func() (float64, bool) { return float64(runtime.NumCPU()) + 5, true })
	SetPressureFn(normalPressure)

	hold, load, cores, _ := shouldDeferDispatch()
	if !hold {
		t.Fatalf("high load (%v) must defer dispatch against %d cores", load, cores)
	}
}

// TestShouldDeferDispatchLowLoadAllows is the other direction: comfortably
// below core count and normal memory pressure must not defer.
func TestShouldDeferDispatchLowLoadAllows(t *testing.T) {
	defer SetLoadAvgFn(nil)
	defer SetPressureFn(nil)
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	SetPressureFn(normalPressure)

	hold, load, _, _ := shouldDeferDispatch()
	if hold {
		t.Fatalf("low load (%v) must not defer dispatch", load)
	}
}

// TestShouldDeferDispatchUnknownLoadAllows: a read failure is not evidence of
// an overloaded host — it must fail open, not become a silent fabric-wide stall.
func TestShouldDeferDispatchUnknownLoadAllows(t *testing.T) {
	defer SetLoadAvgFn(nil)
	defer SetPressureFn(nil)
	SetLoadAvgFn(func() (float64, bool) { return 0, false })
	SetPressureFn(normalPressure)

	hold, _, _, _ := shouldDeferDispatch()
	if hold {
		t.Fatal("unknown load average must not defer dispatch")
	}
}

// TestShouldDeferDispatchMemoryPressureHolds is the "must fail red" direction
// for the 2026-09-26 gap (A35): the M1 stall had load average UNDER the core
// count the whole time — memory pressure is the only signal that would have
// caught it. Critical pressure must defer even with idle CPU.
func TestShouldDeferDispatchMemoryPressureHolds(t *testing.T) {
	defer SetLoadAvgFn(nil)
	defer SetPressureFn(nil)
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	SetPressureFn(func() (guard.PressureLevel, string) { return guard.PressureCritical, "test" })

	hold, _, _, pressure := shouldDeferDispatch()
	if !hold {
		t.Fatalf("critical memory pressure (%v) must defer dispatch even with idle CPU", pressure)
	}
}

// TestShouldDeferDispatchUnknownPressureAllows: an unresolved pressure source
// (no watcher, no cache) must not itself become a defer reason, matching the
// unknown-load-average fail-open stance.
func TestShouldDeferDispatchUnknownPressureAllows(t *testing.T) {
	defer SetLoadAvgFn(nil)
	defer SetPressureFn(nil)
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	SetPressureFn(func() (guard.PressureLevel, string) { return guard.PressureUnknown, "unknown" })

	hold, _, _, _ := shouldDeferDispatch()
	if hold {
		t.Fatal("unknown memory pressure must not defer dispatch")
	}
}

// TestFabricDispatchOverloadedRecordsHeal proves the gate is owner-visible
// (task requirement 2), not silent.
func TestFabricDispatchOverloadedRecordsHeal(t *testing.T) {
	defer SetLoadAvgFn(nil)
	defer SetPressureFn(nil)
	drainHeals()
	SetLoadAvgFn(func() (float64, bool) { return float64(runtime.NumCPU()) + 5, true })
	SetPressureFn(normalPressure)

	if !fabricDispatchOverloaded("worker-agent", 3) {
		t.Fatal("expected dispatch to be held")
	}
	heals := drainHeals()
	if len(heals) != 1 {
		t.Fatalf("expected exactly one recorded heal, got %v", heals)
	}
}
