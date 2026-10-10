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
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

// defaultLoadAvg1m returns the host's effective load in core-equivalents: busy
// CPU share x cores, from the second aggregate `iostat` sample (the first is since boot).
// If iostat is unreadable it falls back to half the 1-minute load average (the
// old gate tripped at 2x). A read failure returns 0, false — an unreadable
// host is not evidence of an overloaded host, so callers must not gate.
func defaultLoadAvg1m() (float64, bool) {
	return coordinatedHostLoad(hostLoadCachePath(), loadProbeFn, 7*time.Second)
}

// Every cooperating process uses the same persistent lock inode. Never unlink
// it: doing so permits a second inode/lock and overlapping probes. Kernel locks
// are released on exit/crash. Lock contention is bounded and returns unknown;
// it never launches an uncoordinated duplicate or invents a pressure value.
func coordinatedHostLoad(p string, probe func() (float64, bool), wait time.Duration) (float64, bool) {
	if p == "" {
		return 0, false
	}
	if v, ok, fresh := readPressureRecord(p); fresh {
		return v, ok
	}
	// A fresh home (CI, a new user) has no ~/.sirsi yet: O_CREATE cannot
	// create the missing parent directory, so OpenFile would fail and every
	// caller would see unknown without ever reaching probe(). Create it once,
	// up front, same as every other first-write-to-~/.sirsi site in this repo.
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return 0, false
	}
	f, err := os.OpenFile(p+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	deadline := time.Now().Add(wait)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return 0, false
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The lock is released on close/exit regardless; the unlock here is only
	// to free it for a waiting contender before this function's other
	// deferred work runs. Its error carries no actionable recovery (errcheck).
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	if v, ok, fresh := readPressureRecord(p); fresh {
		return v, ok
	}
	v, ok := probe()
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		v, ok = 0, false
	}
	// Persist unknown too: failure must not provoke nine simultaneous retries.
	// A write failure preserves the actual probe result but makes no one-probe-per-TTL guarantee.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".host-pressure-*")
	if err != nil {
		return v, ok
	}
	name := tmp.Name()
	defer os.Remove(name)
	_, err = fmt.Fprintf(tmp, "v2 %d %t %.17g\n", time.Now().UnixMilli(), ok, v)
	closeErr := tmp.Close()
	if err != nil || closeErr != nil {
		return v, ok
	}
	if os.Rename(name, p) != nil {
		return v, ok
	}
	return v, ok
}

func readPressureRecord(p string) (float64, bool, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, false, false
	}
	var version string
	var ms int64
	var ok bool
	var v float64
	if n, err := fmt.Sscanf(string(b), "%s %d %t %f", &version, &ms, &ok, &v); err != nil || n != 4 || version != "v2" {
		return 0, false, false
	}
	age := time.Since(time.UnixMilli(ms))
	if age < 0 || age > hostLoadCacheTTL || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, false, false
	}
	if !ok {
		return 0, false, true
	}
	return v, true, true
}

// hostLoadCacheTTL: every wake loop on a host asks this question each cycle, and the answer
// formerly used a process-enumerating top probe. The aggregate iostat replacement
// must be cost-qualified on the native host before deployment. Cooperating upgraded
// processes share one successful cache publication per TTL; filesystem failure
// permits sequential retries, never overlapping probes. A gate
// that tolerates a 30 s old reading loses nothing, since dispatch pacing is minutes.
const hostLoadCacheTTL = 30 * time.Second

var (
	loadProbeFn       = probeLoad
	hostLoadCachePath = func() string {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".sirsi", "host-pressure-v2.cache")
	}
)

// probeLoad reads aggregate kernel CPU statistics, avoiding top's process scan.
// iostat's first row is since boot; only the second data row is admitted.
// -w 1 requests a nominal interval, not a guaranteed one-second window:
// IOKit notifications can wake Apple's CFRunLoop early. Integer idle rounding
// is about 0.5pp, but temporal sampling error has NO established numeric bound.
// This is a recent aggregate observation, not controlled wall-interval proof.
// Native custodian acceptance of these semantics is required before rollout.
func probeLoad() (float64, bool) {
	return probeLoadWith(func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, name, args...).Output()
	}, runtime.NumCPU())
}

func probeLoadWith(run func(string, ...string) ([]byte, error), cores int) (float64, bool) {
	if cores <= 0 {
		return 0, false
	}
	if out, err := run("/usr/sbin/iostat", "-d", "-C", "-n", "0", "-c", "2", "-w", "1"); err == nil {
		if idle, ok := parseIntervalIdle(string(out)); ok {
			return float64(cores) * (100 - idle) / 100, true
		}
	}
	out, err := run("/usr/sbin/sysctl", "-n", "vm.loadavg")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(strings.Trim(strings.TrimSpace(string(out)), "{}"))
	if len(fields) < 1 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, false
	}
	return v / 2, true
}

// CPU-only output must have an exact us/sy/id header and two valid rows.
// Reject layout changes, truncated output, and non-finite/out-of-range values.
func parseIntervalIdle(out string) (float64, bool) {
	header, rows, idle := false, 0, 0.0
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || (len(f) == 1 && f[0] == "cpu") {
			continue
		}
		if len(f) == 3 && f[0] == "us" && f[1] == "sy" && f[2] == "id" {
			header = true
			continue
		}
		if !header || len(f) != 3 {
			return 0, false
		}
		var values [3]float64
		for i := range values {
			v, err := strconv.ParseFloat(f[i], 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
				return 0, false
			}
			values[i] = v
		}
		if math.Abs(values[0]+values[1]+values[2]-100) > 2 {
			return 0, false
		}
		idle = values[2]
		rows++
	}
	return idle, rows == 2
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
