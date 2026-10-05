// backpressure.go — load-average + memory-pressure dispatch gate (R7/G6).
//
// Incident, 2026-08-06: with only three lanes live, unbounded worker build
// parallelism (each spawned consumer's `go build`/`go test` fanning out to
// GOMAXPROCS=NCPU) drove load average to 36 on an 18-core host — 2x
// oversubscription from 10 concurrent compile/test processes. Capping
// GOMAXPROCS on spawned consumers (consumer.go) addresses the fan-out; this
// file addresses the other half: never START a new lane while the host is
// already saturated, so a burst of inbox depth cannot pile dispatch on top of
// dispatch.
//
// Incident, 2026-09-26 (claude-io, router item ...m1-stall): an M1 (16 GB)
// went unresponsive (≥32s, all liveness rails dark) with co-tenants — a
// FinalWishes vitest run, an iOS Simulator, a Codex CLI session, the Hermes
// receiver — swapping 3.5/4 GB with a 3 GB compressor and 251k swapouts. Load
// average never crossed the core count; the host still stalled. CORES were
// never the resource that failed — MEMORY was, and this gate only checked the
// former. A35 (Scope The Check To The Claim): a gate that claims "don't
// dispatch onto a saturated host" but only reads CPU is scoped narrower than
// its claim. Memory pressure (internal/guard, already the ADR-031-B kernel
// signal Hapi subscribes to) is now a second, independent defer signal.
//
// Reuses the same `sysctl -n vm.loadavg` shell-out internal/vitals already
// uses for the TUI/menubar/dashboard (vitals.go collectLoadAvg) rather than
// reimplementing it — router does not import vitals (a heavier package with
// its own process/disk/network collectors) for one number, so the parse is
// duplicated at this single call site instead.
package router

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// loadAvgMu guards the injected seam (Rule A16/A21): tests substitute a fixed
// value instead of shelling the real sysctl.
var (
	loadAvgMu sync.RWMutex
	loadAvgFn = defaultLoadAvg1m

	pressureMu sync.RWMutex
	pressureFn = guard.CurrentPressure
)

// defaultLoadAvg1m shells `sysctl -n vm.loadavg` (macOS; ADR-032) and returns
// the 1-minute figure. A read failure returns 0, false — callers must treat
// "unknown" as "don't gate", the same fail-open stance ResolveConsumer takes
// on an unreadable cwd is NOT appropriate here (that fails closed on purpose);
// an unreadable load average is not evidence of an overloaded host.
func defaultLoadAvg1m() (float64, bool) {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0, false
	}
	// Output: "{ 1.23 4.56 7.89 }"
	raw := strings.Trim(strings.TrimSpace(string(out)), "{}")
	fields := strings.Fields(raw)
	if len(fields) < 1 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// SetLoadAvgFn installs a test double for the load-average reader. Passing
// nil restores the real sysctl-backed default.
func SetLoadAvgFn(fn func() (float64, bool)) {
	loadAvgMu.Lock()
	defer loadAvgMu.Unlock()
	if fn != nil {
		loadAvgFn = fn
		return
	}
	loadAvgFn = defaultLoadAvg1m
}

func getLoadAvgFn() func() (float64, bool) {
	loadAvgMu.RLock()
	defer loadAvgMu.RUnlock()
	return loadAvgFn
}

// SetPressureFn installs a test double for the memory-pressure reader.
// Passing nil restores the real guard.CurrentPressure default.
func SetPressureFn(fn func() (guard.PressureLevel, string)) {
	pressureMu.Lock()
	defer pressureMu.Unlock()
	if fn != nil {
		pressureFn = fn
		return
	}
	pressureFn = guard.CurrentPressure
}

func getPressureFn() func() (guard.PressureLevel, string) {
	pressureMu.RLock()
	defer pressureMu.RUnlock()
	return pressureFn
}

// shouldDeferDispatch reports whether the host is saturated and dispatch
// should be skipped this pass — on EITHER of two independent signals: load
// average at/above core count, or memory pressure at Warn/Critical (the
// 2026-09-26 M1 stall: load never crossed cores, memory did). An unknown load
// average never defers on its own — a read failure must not itself become a
// fabric-wide stall; PressureUnknown is likewise never a defer reason.
func shouldDeferDispatch() (hold bool, load float64, cores int, pressure guard.PressureLevel) {
	load, ok := getLoadAvgFn()()
	cores = runtime.NumCPU()
	pressure, _ = getPressureFn()()
	if pressure >= guard.PressureWarn {
		return true, load, cores, pressure
	}
	if !ok {
		return false, load, cores, pressure
	}
	return load >= float64(cores), load, cores, pressure
}
