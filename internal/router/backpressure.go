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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// loadAvgMu guards the injected seam (Rule A16/A21): tests substitute a fixed
// value instead of shelling the real sysctl.
var (
	loadAvgMu sync.RWMutex
	loadAvgFn = defaultLoadAvg1m
)

// saturationIdlePct is the CPU idle share below which the host counts as
// saturated. Load average counts every runnable thread — Spotlight and Photos
// analysis included — which run at the lowest priority and only take cores
// nothing else wants; on 2026-10-01 the M1 showed load 13-17 on 10 cores at 36%
// idle and the gate starved every lane. Idle CPU is what a new consumer
// actually competes for.
const saturationIdlePct = 10.0

var cpuIdleRe = regexp.MustCompile(`([0-9.]+)% idle`)

// defaultLoadAvg1m returns the host's effective load in core-equivalents: busy
// CPU share x cores, from the second `top` sample (the first is since boot).
// If top is unreadable it falls back to half the 1-minute load average (the
// old gate tripped at 2x). A read failure returns 0, false — an unreadable
// host is not evidence of an overloaded host, so callers must not gate.
func defaultLoadAvg1m() (float64, bool) {
	if v, ok := readHostLoadCache(); ok {
		return v, true
	}
	v, ok := loadProbeFn()
	if ok {
		writeHostLoadCache(v)
	}
	return v, ok
}

// hostLoadCacheTTL: every wake loop on a host asks this question each cycle, and the answer
// costs a `top -l 2` (3-8 s at 5-8% of a core). Nine loops measured 0.1-0.15 of a core,
// constantly (Mercury, 2026-10-07). One probe per host per TTL serves all of them; a gate
// that tolerates a 30 s old reading loses nothing, since dispatch pacing is minutes.
const hostLoadCacheTTL = 30 * time.Second

var (
	loadProbeFn       = probeLoad
	hostLoadNow       = time.Now
	hostLoadCachePath = func() string {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".sirsi", "host-load.cache")
	}
)

func readHostLoadCache() (float64, bool) {
	p := hostLoadCachePath()
	if p == "" {
		return 0, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, false
	}
	var ms int64
	var v float64
	if n, _ := fmt.Sscanf(string(b), "%d %f", &ms, &v); n != 2 {
		return 0, false
	}
	age := hostLoadNow().Sub(time.UnixMilli(ms))
	if age < 0 || age > hostLoadCacheTTL {
		return 0, false
	}
	return v, true
}

// writeHostLoadCache is best effort and atomic (tmp + rename): a failed write only means
// the next loop probes again.
func writeHostLoadCache(v float64) {
	p := hostLoadCachePath()
	if p == "" {
		return
	}
	tmp := fmt.Sprintf("%s.%d", p, os.Getpid())
	if os.WriteFile(tmp, []byte(fmt.Sprintf("%d %f", hostLoadNow().UnixMilli(), v)), 0o600) == nil {
		if os.Rename(tmp, p) != nil {
			_ = os.Remove(tmp)
		}
	}
}

func probeLoad() (float64, bool) {
	if out, err := exec.Command("top", "-l", "2", "-n", "0", "-s", "1").Output(); err == nil {
		if m := cpuIdleRe.FindAllStringSubmatch(string(out), -1); len(m) > 0 {
			if idle, err := strconv.ParseFloat(m[len(m)-1][1], 64); err == nil {
				return float64(runtime.NumCPU()) * (100 - idle) / 100, true
			}
		}
	}
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0, false
	}
	// Output: "{ 1.23 4.56 7.89 }"
	fields := strings.Fields(strings.Trim(strings.TrimSpace(string(out)), "{}"))
	if len(fields) < 1 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return v / 2, true
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

// shouldDeferDispatch reports whether effective load (see defaultLoadAvg1m) is
// within saturationIdlePct of every core and dispatch should be skipped this pass. An unknown load average
// never defers — a read failure must not itself become a fabric-wide stall.
func shouldDeferDispatch() (hold bool, load float64, cores int) {
	load, ok := getLoadAvgFn()()
	cores = runtime.NumCPU()
	if !ok {
		return false, load, cores
	}
	return load >= float64(cores)*(1-saturationIdlePct/100), load, cores
}
