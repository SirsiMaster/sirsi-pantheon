package schedule

// Per-rail (per-cable) traffic counter: attributes load to a SPECIFIC
// Thunderbolt interface rather than just classifying processes (the default
// probeProcesses). Follow-up to the process-only ActivityProbe (README
// "Per-rail traffic counters ... are a deeper detector ... rail detector is
// a follow-up"). Not wired in as the default probe: a caller opts in with
// SetActivityProbe(NetTrafficProbe) (or composes it with probeProcesses via
// ComposeActivityProbes) on a host/rail where cable-level attribution matters.
//
// Same local-only discipline as probeProcesses (2026-09-27 refuse-don't-mislabel
// incident, maat-schedule#conflict.go): this can only ever sample the host it
// runs on, and refuses rather than silently measuring the wrong machine.

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/scales"
)

// trafficSampleWindow is how long NetTrafficProbe waits between the two
// byte-counter snapshots it diffs. Short enough to stay responsive inside a
// conflict-check call, long enough that a sub-counter-resolution burst
// doesn't read as zero traffic. A var (not a const) so tests can drive it to
// zero rather than sleeping real wall-clock time.
var (
	trafficSampleWindowMu sync.RWMutex
	trafficSampleWindow   = 500 * time.Millisecond
)

func getTrafficSampleWindow() time.Duration {
	trafficSampleWindowMu.RLock()
	defer trafficSampleWindowMu.RUnlock()
	return trafficSampleWindow
}

// setTrafficSampleWindowForTest overrides the sample window (tests only).
func setTrafficSampleWindowForTest(d time.Duration) {
	trafficSampleWindowMu.Lock()
	defer trafficSampleWindowMu.Unlock()
	trafficSampleWindow = d
}

// trafficNoiseFloorBytesPerSec is the combined in+out rate below which a lane
// is treated as idle control-plane chatter, not a traffic actor. A Thunderbolt
// bench/transfer lane runs orders of magnitude above this; background
// heartbeats/ARP do not.
// ponytail: a single fixed floor for every lane, host, and workload; per-lane
// calibration if a quiet lane's own idle chatter ever sits above it.
const trafficNoiseFloorBytesPerSec = 51200 // 50 KB/s

// ifaceByteCounters reads the host's per-interface link-layer byte counters.
// Injectable (A16/A21) so tests drive two synthetic snapshots deterministically
// instead of racing real network state.
var (
	ifaceCounterMu sync.RWMutex
	ifaceCounterFn func() (map[string][2]int64, error) = readNetstatIB
	tbLaneListerFn func() ([]scales.TBLane, error)     = listTBLanesDiscardDrift
)

// listTBLanesDiscardDrift adapts scales.CollectTBLaneDrift (the only exported
// lister, which also reports a drift count this probe has no use for) to the
// plain lister shape tbLaneListerFn needs.
func listTBLanesDiscardDrift() ([]scales.TBLane, error) {
	_, lanes, err := scales.CollectTBLaneDrift()
	return lanes, err
}

// SetTrafficProviders overrides the byte-counter and TB-lane-list sources
// (for testing). Pass nil to keep the current provider.
func SetTrafficProviders(counters func() (map[string][2]int64, error), lanes func() ([]scales.TBLane, error)) {
	ifaceCounterMu.Lock()
	defer ifaceCounterMu.Unlock()
	if counters != nil {
		ifaceCounterFn = counters
	}
	if lanes != nil {
		tbLaneListerFn = lanes
	}
}

func getTrafficProviders() (func() (map[string][2]int64, error), func() ([]scales.TBLane, error)) {
	ifaceCounterMu.RLock()
	defer ifaceCounterMu.RUnlock()
	return ifaceCounterFn, tbLaneListerFn
}

// ResetTrafficProvidersForTest restores the real counter/lane sources. Unlike
// SetTrafficProviders(nil, nil) — which is a deliberate no-op update, not a
// reset — this is what a test's defer actually needs, so a mock installed by
// one test can never leak into the next.
func ResetTrafficProvidersForTest() {
	ifaceCounterMu.Lock()
	defer ifaceCounterMu.Unlock()
	ifaceCounterFn = readNetstatIB
	tbLaneListerFn = listTBLanesDiscardDrift
}

// NetTrafficProbe is an ActivityProbe that attributes load to a specific
// Thunderbolt cable by sampling its link-layer byte counters twice,
// trafficSampleWindow apart, and reporting an Actor per active lane whose
// combined in+out rate clears trafficNoiseFloorBytesPerSec. When ifaceScope
// is non-empty, only that one lane is sampled and its counters/resets are
// the only ones that can error this call — an unrelated lane elsewhere on
// the machine is never this reservation's business (A35: the sampling and
// its error boundary are scoped to the same lane the caller actually cares
// about, not to every active lane on the host).
func NetTrafficProbe(machine, ifaceScope string) ([]Actor, error) {
	if machine != "" {
		local, err := getLocalMachineLabelFn()()
		if err != nil {
			return nil, fmt.Errorf("traffic probe %s: %w", machine, err)
		}
		if !strings.EqualFold(machine, local) {
			return nil, fmt.Errorf("traffic probe %s: this host is %q — no cross-host traffic probe exists; run conflict-check on %s itself", machine, local, machine)
		}
	}
	counters, lanes := getTrafficProviders()
	lanesList, err := lanes()
	if err != nil {
		return nil, fmt.Errorf("list thunderbolt lanes: %w", err)
	}
	var active []scales.TBLane
	for _, l := range lanesList {
		if !l.Active {
			continue
		}
		if ifaceScope != "" && !strings.EqualFold(l.Iface, ifaceScope) {
			continue // not the lane this check was scoped to
		}
		active = append(active, l)
	}
	if len(active) == 0 {
		return nil, nil
	}
	before, err := counters()
	if err != nil {
		return nil, fmt.Errorf("sample byte counters: %w", err)
	}
	window := getTrafficSampleWindow()
	time.Sleep(window)
	after, err := counters()
	if err != nil {
		return nil, fmt.Errorf("sample byte counters: %w", err)
	}
	windowSec := window.Seconds()
	if windowSec == 0 {
		windowSec = 1 // test-only (window forced to 0): report raw deltas, not a divide-by-zero
	}
	var actors []Actor
	for _, l := range active {
		a, aok := before[l.Iface]
		b, bok := after[l.Iface]
		if !aok || !bok {
			// An active lane with no readable counter row (malformed `netstat -ib`
			// output, or the interface vanished/renamed between snapshots) is
			// UNKNOWN, never clean: silently skipping it reported "no traffic" for
			// a lane we simply failed to read. Fail toward reporting (A35 — a
			// check narrower than its claim is worse than no check).
			return nil, fmt.Errorf("sample byte counters: active lane %s (%s) missing or unreadable in netstat output", l.Port, l.Iface)
		}
		inPS := int64(float64(b[0]-a[0]) / windowSec)
		outPS := int64(float64(b[1]-a[1]) / windowSec)
		if inPS < 0 || outPS < 0 {
			// A counter reset between snapshots (interface flap/replug, counter
			// wraparound) makes the interval UNMEASURABLE, not clean: silently
			// skipping it reported "no traffic" for a lane we simply couldn't read
			// this round (the same false-clean shape as a missing counter row
			// above — A35, same fail-toward-reporting discipline).
			return nil, fmt.Errorf("sample byte counters: active lane %s (%s) counter reset between snapshots — unmeasurable, not clean", l.Port, l.Iface)
		}
		if inPS+outPS < trafficNoiseFloorBytesPerSec {
			continue
		}
		actors = append(actors, Actor{
			Kind:   "traffic",
			Detail: fmt.Sprintf("%s (%s): %d B/s in, %d B/s out", l.Port, l.Iface, inPS, outPS),
			Iface:  l.Iface,
		})
	}
	return actors, nil
}

// readNetstatIB is the real byte-counter source: the link-layer row of
// `netstat -ib` (Network column "<Link#N>"), keyed by interface name to
// [inBytes, outBytes]. Column position varies with whether the link row
// carries a MAC (Address non-empty), so counters are read from the END of
// the row (Coll, Obytes, Oerrs, Opkts, Ibytes, Ierrs, Ipkts), not a fixed index.
func readNetstatIB() (map[string][2]int64, error) {
	out, err := exec.Command("netstat", "-ib").Output()
	if err != nil {
		return nil, err
	}
	return parseNetstatIB(string(out))
}

// parseNetstatIB is readNetstatIB's pure parsing half, split out so tests
// drive it with a fixture string instead of mocking exec.Command.
func parseNetstatIB(out string) (map[string][2]int64, error) {
	counters := map[string][2]int64{}
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 || fields[2] == "" || !strings.HasPrefix(fields[2], "<Link#") {
			continue
		}
		inBytes, err1 := strconv.ParseInt(fields[len(fields)-5], 10, 64)
		outBytes, err2 := strconv.ParseInt(fields[len(fields)-2], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		counters[fields[0]] = [2]int64{inBytes, outBytes}
	}
	return counters, scanner.Err()
}

// ComposeActivityProbes merges actors from several probes into one, so a
// host can run both the process classifier and NetTrafficProbe together. Any
// probe's error aborts the whole sample (fail toward reporting, matching
// CheckConflicts's own fail-closed discipline).
func ComposeActivityProbes(probes ...ActivityProbe) ActivityProbe {
	return func(machine, ifaceScope string) ([]Actor, error) {
		var all []Actor
		for _, p := range probes {
			actors, err := p(machine, ifaceScope)
			if err != nil {
				return nil, err
			}
			all = append(all, actors...)
		}
		return all, nil
	}
}
