// backpressure.go — load-average dispatch gate (R7/G6).
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
)

// loadAvgMu guards the injected seam (Rule A16/A21): tests substitute a fixed
// value instead of shelling the real sysctl.
var (
	loadAvgMu sync.RWMutex
	loadAvgFn = defaultLoadAvg1m
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

// shouldDeferDispatch reports whether load average is at or above the core
// count and dispatch should be skipped this pass. An unknown load average
// never defers — a read failure must not itself become a fabric-wide stall.
func shouldDeferDispatch() (hold bool, load float64, cores int) {
	load, ok := getLoadAvgFn()()
	cores = runtime.NumCPU()
	if !ok {
		return false, load, cores
	}
	return load >= float64(cores), load, cores
}
