package guard

// deathspiral.go — the "Memory Death Spiral" health check (router item
// 20260716-172540). On 2026-07-16 the machine sat at load 31.8 / swap 95% /
// 244 leaked claude-code sessions and `sirsi diagnose` said 94/100 🟡, flagging
// only a Spotlight storm: the health model never read load average, swap
// exhaustion %, compressor occupancy, or per-parent process fan-out — the
// exact memory-death signals Pantheon exists to catch. This check reads all
// four and scores the combined condition Critical (live-critical → forces 🔴).

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// Apple Silicon page size for vm_stat readings (16 KB, not 4 KB — the 4x-low bug).
const vmStatPageBytes = 16384

// fanOutAlarm is the per-parent child-process count that reads as a session
// leak (the ScheduleWakeup/CCD leak parented 244 claude-code processes under
// one Claude.app PID). Ordinary apps fan out helpers in the tens.
const fanOutAlarm = 100

// isDeathSpiral is THE spiral ladder — one definition, both callers (the doctor
// check and the launchd liveness probe), so the two surfaces can never drift on
// what "dying" means. Each signal alone has an innocent explanation: swap fills
// during a transient spike and STAYS full, and a busy machine runs hot. The
// spiral is swap exhaustion together with either a paging-driven load storm or
// genuinely no reclaimable memory left.
func isDeathSpiral(swapPct, load1, availGB float64, cores int) bool {
	return swapPct >= 90 && (load1 >= 2*float64(cores) || availGB < 0.5)
}

// checkMemoryDeathSpiral emits one "Memory Death Spiral" finding.
//
// Severity ladder:
//   - Critical (live — forces RED): swap ≥ 90% exhausted AND (1-min load ≥ 2×cores
//     OR available RAM < 512 MB). See isDeathSpiral.
//   - Warn: swap ≥ 80%, OR load ≥ 1.5×cores, OR a single parent with ≥ 100 children.
//   - OK otherwise, with the measured headroom shown as evidence.
func checkMemoryDeathSpiral(p platform.Platform, report *DoctorReport) {
	load1, loadOK := readLoad1(p)
	cores := readCores(p)
	swapPct, swapUsedGB, swapOK := readSwapPct(p)
	availGB, compressedGB := readVMStatGB(p)
	fanCount, fanParent := maxProcessFanOut(p)

	if !loadOK && !swapOK {
		return // no signals readable on this platform — emit nothing, never guess
	}

	f := DiagnosticFinding{
		Check: "Memory Death Spiral",
		Detail: fmt.Sprintf("load1 %.1f / %d cores | swap %.0f%% (%.1f GB) | available %.2f GB | compressor %.1f GB | max fan-out %d (%s)",
			load1, cores, swapPct, swapUsedGB, availGB, compressedGB, fanCount, fanParent),
	}

	spiral := isDeathSpiral(swapPct, load1, availGB, cores)
	leak := fanCount >= fanOutAlarm

	switch {
	case spiral:
		f.Severity = SeverityCritical
		f.Message = fmt.Sprintf("Memory death spiral — swap %.0f%% exhausted, load %.1f on %d cores, %.2f GB available. The machine is paging itself to death.", swapPct, load1, cores, availGB)
		if leak {
			f.Message += fmt.Sprintf(" Likely driver: %s has %d child processes — restart it to reclaim them.", fanParent, fanCount)
		}
	case swapPct >= 80 || load1 >= 1.5*float64(cores) || leak:
		f.Severity = SeverityWarn
		switch {
		case leak:
			f.Message = fmt.Sprintf("Process leak: %s has %d child processes — restart it to reclaim memory before this becomes a spiral.", fanParent, fanCount)
		case swapPct >= 80:
			f.Message = fmt.Sprintf("Swap %.0f%% exhausted (%.1f GB) — nearing the wall; close or restart heavy apps.", swapPct, swapUsedGB)
		default:
			f.Message = fmt.Sprintf("Load %.1f on %d cores — the machine is saturated.", load1, cores)
		}
	default:
		f.Severity = SeverityOK
		f.Message = fmt.Sprintf("Memory headroom normal — swap %.0f%%, load %.1f/%d cores", swapPct, load1, cores)
	}
	report.Findings = append(report.Findings, f)
}

// MemoryDeath is a one-shot read of the memory-death signals, for consumers
// outside the doctor render path (the launchd liveness watch). It mirrors the
// exact spiral ladder in checkMemoryDeathSpiral so both surfaces agree on what
// "dying" means — one definition, two callers.
type MemoryDeath struct {
	SwapPct     float64
	SwapUsedGB  float64
	AvailableGB float64 // free + inactive — what a process can actually get
	Load1       float64
	Cores       int
	Readable    bool // false when no signal is readable on this platform (never guess)
	Dying       bool // the live-critical spiral — see isDeathSpiral
}

// SampleMemoryDeath reads swap%, free RAM, and load in one pass and classifies
// the spiral condition. Read-only; safe to call from a launchd probe.
func SampleMemoryDeath() MemoryDeath {
	p := platform.Current()
	load1, loadOK := readLoad1(p)
	cores := readCores(p)
	swapPct, swapUsedGB, swapOK := readSwapPct(p)
	availGB, _ := readVMStatGB(p)
	md := MemoryDeath{
		SwapPct: swapPct, SwapUsedGB: swapUsedGB, AvailableGB: availGB,
		Load1: load1, Cores: cores, Readable: loadOK || swapOK,
	}
	md.Dying = isDeathSpiral(swapPct, load1, availGB, cores)
	return md
}

// readLoad1 parses `sysctl -n vm.loadavg` ("{ 31.80 29.00 22.10 }").
func readLoad1(p platform.Platform) (float64, bool) {
	out, err := p.Command("sysctl", "-n", "vm.loadavg")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(strings.Trim(strings.TrimSpace(string(out)), "{}"))
	if len(fields) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	return v, err == nil
}

// readCores parses `sysctl -n hw.ncpu`; falls back to 8 so a probe failure
// never divides by zero or masks a real spiral behind an absurd core count.
func readCores(p platform.Platform) int {
	out, err := p.Command("sysctl", "-n", "hw.ncpu")
	if err == nil {
		if n, perr := strconv.Atoi(strings.TrimSpace(string(out))); perr == nil && n > 0 {
			return n
		}
	}
	return 8
}

// readSwapPct parses `sysctl -n vm.swapusage`
// ("total = 26624.00M  used = 25231.00M  free = 1393.00M ...").
func readSwapPct(p platform.Platform) (pct, usedGB float64, ok bool) {
	out, err := p.Command("sysctl", "-n", "vm.swapusage")
	if err != nil {
		return 0, 0, false
	}
	var total, used float64
	fields := strings.Fields(string(out))
	for i := 0; i+2 < len(fields); i++ {
		v, perr := strconv.ParseFloat(strings.TrimSuffix(fields[i+2], "M"), 64)
		if perr != nil {
			continue
		}
		switch fields[i] {
		case "total":
			total = v
		case "used":
			used = v
		}
	}
	if total <= 0 {
		return 0, 0, false
	}
	return used / total * 100, used / 1024, true
}

// readVMStatGB reads free + compressor-occupied memory from `vm_stat`.
func readVMStatGB(p platform.Platform) (availGB, compressedGB float64) {
	out, err := p.Command("vm_stat")
	if err != nil {
		return 0, 0
	}
	pages := func(label string) float64 {
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), label) {
				continue
			}
			n := strings.TrimSpace(strings.TrimSuffix(strings.SplitN(line, ":", 2)[1], "."))
			v, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
			return v
		}
		return 0
	}
	const gb = 1 << 30
	// AVAILABLE, not free. On macOS "Pages free" is near-zero by design on any
	// long-running machine — the OS parks idle RAM in "Pages inactive" as cache
	// and reclaims it on demand. Reading free alone called a 48 GB host with
	// 20 GB of reclaimable inactive pages a death spiral while macOS itself
	// reported 85% memory free (2026-07-24). Available = free + inactive is the
	// quantity a process can actually get.
	return (pages("Pages free") + pages("Pages inactive")) * vmStatPageBytes / gb,
		pages("Pages occupied by compressor") * vmStatPageBytes / gb
}

// maxProcessFanOut finds the parent with the most direct children from one
// `ps -axo ppid=,comm=` pass, naming the parent via its own comm lookup.
func maxProcessFanOut(p platform.Platform) (int, string) {
	out, err := p.Command("ps", "-axo", "pid=,ppid=,comm=")
	if err != nil {
		return 0, "-"
	}
	children := map[string]int{}
	comm := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		children[fields[1]]++
		comm[fields[0]] = fields[len(fields)-1]
	}
	// launchd (ppid 1) parents everything by design — not a leak signal.
	delete(children, "1")
	delete(children, "0")
	type kv struct {
		ppid string
		n    int
	}
	var all []kv
	for ppid, n := range children {
		all = append(all, kv{ppid, n})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
	if len(all) == 0 {
		return 0, "-"
	}
	name := comm[all[0].ppid]
	if name == "" {
		name = "pid " + all[0].ppid
	}
	// Basename the command path for readability.
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return all[0].n, name
}
