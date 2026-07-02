// Package guard — hapi.go
//
// 🌊 Hapi — the live MEMORY governor (the "protect" pillar of ADR-031-A).
//
// The Isis watchdog (watchdog.go) governs CPU. Nothing governed system MEMORY —
// and on 2026-06-18 that exact gap let Pantheon's OWN warm-inference broker
// balloon ~53 GB on a 48 GB M5 Max into a kernel Jetsam that froze the host
// (docs/case-studies/2026-06-18-pantheon-did-not-prevent-oom.md). Renice — the
// only relief the CPU side had — yields scheduling time but frees ZERO bytes of
// RAM, so it could not have stopped it.
//
// Hapi closes the gap. It samples system free RAM + per-process RSS on a bounded
// tick and intervenes BEFORE the kernel does, escalating only as pressure rises:
//
//	OK ──▶ WARN (name the runaway) ──▶ SUSPEND a runaway (SIGSTOP halts its
//	       balloon — reversible) ──▶ KILL as the last resort.
//
// Two regimes, separated by CONSENT (ADR-031-A, PANTHEON_RULES A1):
//
//   - GOVERNED processes — Pantheon-spawned compute that registered its PID with
//     Hapi (the gemma broker). They consented to governance, so Hapi acts on them
//     with teeth automatically: suspend at critical, kill at emergency. This is
//     "Hapi made real" / Layer 4 — the broker can NEVER again OOM the host, because
//     Hapi halts it first. SIGSTOP is reversible: when pressure clears, Hapi
//     resumes (SIGCONT) what it paused.
//
//   - EVERY OTHER process — Hapi WARNS and RECOMMENDS; it does NOT auto-kill a
//     process the user never asked it to govern. The user confirms (same
//     dry-run-by-default contract as `sirsi relieve` / `sirsi slay`).
//
// Protected processes (WindowServer, the kernel, audio, the session UI, sirsi
// itself, and live Claude/Codex agents) are NEVER suspended or killed — defense
// in depth via isProtectedReniceTarget + an agent allowlist. Protecting the other
// agents from a runaway is the governor's job, exactly as the RAM gate is
// cross-agent-aware (ADR-031-A, gemmaAgentReserveBytes).
package guard

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// MemTier is the system memory-pressure level Hapi acts on, by free-RAM percent.
type MemTier int

const (
	TierOK        MemTier = iota // plenty of headroom — nothing to do
	TierWarn                     // getting tight — name the runaway, recommend
	TierCritical                 // suspend a governed runaway (reversible) before Jetsam
	TierEmergency                // kill governed compute — the host comes first
)

func (t MemTier) String() string {
	switch t {
	case TierWarn:
		return "warn"
	case TierCritical:
		return "critical"
	case TierEmergency:
		return "emergency"
	default:
		return "ok"
	}
}

// MemProc is one process and its resident memory.
type MemProc struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
	RSS  int64  `json:"rss_bytes"`
}

// MemSample is a single read of system memory + the largest resident processes.
type MemSample struct {
	TotalRAM    int64     `json:"total_ram"`
	FreeBytes   int64     `json:"free_bytes"`
	FreePercent float64   `json:"free_percent"`
	Top         []MemProc `json:"top"` // RSS descending
}

// Injectable sampler (Rule A16/A21) so the governor is testable without a real,
// memory-pressured host.
var (
	hapiSampleMu sync.RWMutex
	hapiSampleFn = defaultHapiSample
)

func hapiSample() (MemSample, error) {
	hapiSampleMu.RLock()
	fn := hapiSampleFn
	hapiSampleMu.RUnlock()
	return fn()
}

func setHapiSampleFn(fn func() (MemSample, error)) {
	hapiSampleMu.Lock()
	hapiSampleFn = fn
	hapiSampleMu.Unlock()
}

func defaultHapiSample() (MemSample, error) {
	// TotalRAMBytes is the same hw.memsize probe DetectHardware runs, minus the
	// ~600ms system_profiler GPU detour a memory sample never needed — this keeps
	// the `sirsi vitals` fast path (and every Hapi tick) well under its budget.
	s := MemSample{TotalRAM: seba.TotalRAMBytes(), FreeBytes: hapiFreeRAMBytes()}
	total := s.TotalRAM
	if total > 0 {
		s.FreePercent = float64(s.FreeBytes) / float64(total) * 100
	}
	top, err := hapiTopByRSS(12)
	if err != nil {
		return s, err
	}
	s.Top = top
	return s, nil
}

// hapiVMStatFn / hapiPsFn are the raw command seams (Rule A16) so the vm_stat/ps
// parsers below are unit-testable with canned output, without reading real
// system state. Production keeps the real commands.
var (
	hapiVMStatFn = func() ([]byte, error) { return exec.Command("vm_stat").Output() }
	hapiPsFn     = func() ([]byte, error) { return exec.Command("ps", "-axo", "pid,rss,comm").Output() }
)

// hapiFreeRAMBytes reads vm_stat and returns reclaimable memory: free + inactive
// + speculative pages × page size. This mirrors the broker's RAM gate
// (gemmaFreeRAMBytes) so the gate and the governor agree on "free".
func hapiFreeRAMBytes() int64 {
	out, err := hapiVMStatFn()
	if err != nil {
		return 0
	}
	pageSize := int64(16384) // M-series default; corrected from the header below
	var free, inactive, speculative int64
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "page size of") {
			f := strings.Fields(line)
			for i, tok := range f {
				if tok == "of" && i+1 < len(f) {
					if v, e := strconv.ParseInt(f[i+1], 10, 64); e == nil {
						pageSize = v
					}
				}
			}
			continue
		}
		val := func(prefix string) (int64, bool) {
			if !strings.HasPrefix(line, prefix) {
				return 0, false
			}
			v := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, prefix)), ".")
			n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			return n, true
		}
		if v, ok := val("Pages free:"); ok {
			free = v
		} else if v, ok := val("Pages inactive:"); ok {
			inactive = v
		} else if v, ok := val("Pages speculative:"); ok {
			speculative = v
		}
	}
	return (free + inactive + speculative) * pageSize
}

// hapiTopByRSS returns the top-N processes by resident memory, descending.
func hapiTopByRSS(topN int) ([]MemProc, error) {
	out, err := hapiPsFn()
	if err != nil {
		return nil, err
	}
	var procs []MemProc
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			first = false
			continue
		}
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		rss, _ := strconv.ParseInt(f[1], 10, 64)
		procs = append(procs, MemProc{PID: pid, Name: strings.Join(f[2:], " "), RSS: rss * 1024})
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].RSS > procs[j].RSS })
	if len(procs) > topN {
		procs = procs[:topN]
	}
	return procs, nil
}

// SampleMemory returns a one-shot read of system memory + the largest resident
// processes. Exported for the `sirsi hapi` CLI and the menubar status surface.
func SampleMemory() (MemSample, error) { return hapiSample() }

// ClassifyMem maps a sample to a pressure tier using the given free-RAM percent
// thresholds (lower free % = higher tier). This is the FALLBACK path: when the
// kernel dispatch level is available it RAISES the tier floor (memTierFromPressure
// in GovernOnce); free-% remains the only source of TierEmergency (the kernel only
// reports NORMAL/WARN/CRITICAL).
func ClassifyMem(s MemSample, warnPct, critPct, emergPct float64) MemTier {
	switch {
	case s.FreePercent < emergPct:
		return TierEmergency
	case s.FreePercent < critPct:
		return TierCritical
	case s.FreePercent < warnPct:
		return TierWarn
	default:
		return TierOK
	}
}

// memTierFromPressure maps a kernel memory-pressure level onto the governor's
// tiers. The kernel reports NORMAL/WARN/CRITICAL only; kill (TierEmergency) stays
// driven by the free-%/RSS accounting, so the kernel level can RAISE the tier to
// suspend-governed but never on its own decides a kill.
func memTierFromPressure(l PressureLevel) MemTier {
	switch l {
	case PressureCritical:
		return TierCritical
	case PressureWarn:
		return TierWarn
	default:
		return TierOK
	}
}

// hapiIsAgent reports whether a process name belongs to a live coding agent we
// must never touch — they are doing the actual work the host exists for.
func hapiIsAgent(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "claude") || strings.Contains(n, "codex")
}

// FindRunaway returns the largest-RSS process that is neither protected nor a live
// agent — the thing a human would point at and say "that's the leak." Read-only;
// safe for a status preview. Returns nil if only protected/agent processes are big.
func FindRunaway(s MemSample) *MemProc {
	for i := range s.Top { // RSS descending
		p := s.Top[i]
		if isProtectedReniceTarget(p.Name) || hapiIsAgent(p.Name) {
			continue
		}
		return &p
	}
	return nil
}

// ── Governed registry ──────────────────────────────────────────────────────
// Pantheon-spawned compute registers its PID here to consent to being governed
// with teeth. Stored as `<pid> <name>` lines in ~/.sirsi/hapi-governed. Dead PIDs
// are pruned on read so a reused PID is never acted on (the PID-alive lesson).

func hapiGovernedPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sirsi", "hapi-governed")
}

// HapiRegisterGoverned records pid/name as Pantheon-governed compute. The gemma
// broker calls this on launch so Hapi may suspend/kill it under pressure.
func HapiRegisterGoverned(pid int, name string) error {
	_ = os.MkdirAll(filepath.Dir(hapiGovernedPath()), 0o755)
	cur := hapiGovernedPIDs()
	cur[pid] = name
	return hapiWriteGoverned(cur)
}

// HapiUnregisterGoverned removes a PID (broker shutdown).
func HapiUnregisterGoverned(pid int) error {
	cur := hapiGovernedPIDs()
	delete(cur, pid)
	return hapiWriteGoverned(cur)
}

func hapiWriteGoverned(m map[int]string) error {
	var sb strings.Builder
	for pid, name := range m {
		fmt.Fprintf(&sb, "%d %s\n", pid, name)
	}
	return os.WriteFile(hapiGovernedPath(), []byte(sb.String()), 0o644)
}

// hapiGovernedPIDs returns the live governed processes (dead PIDs pruned).
func hapiGovernedPIDs() map[int]string {
	m := map[int]string{}
	b, err := os.ReadFile(hapiGovernedPath())
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		pid, e := strconv.Atoi(f[0])
		if e != nil || !hapiProcessAlive(pid) {
			continue
		}
		name := ""
		if len(f) > 1 {
			name = strings.Join(f[1:], " ")
		}
		m[pid] = name
	}
	return m
}

// hapiProcessAlive + the signal-based intervention primitives (hapiSuspend/
// hapiResume/hapiKill) live in the platform files (hapi_signals_unix.go /
// hapi_signals_windows.go): POSIX signals (SIGSTOP/SIGCONT/SIGTERM, kill(2)) are
// Unix-only, so the Windows cross-build (goreleaser) gets stubs that report
// "unsupported" — the governor's teeth simply no-op there.

// Injectable governed-lookup + intervention seams (Rule A16/A21): tests swap
// these to verify the governor's decisions without touching real processes.
// They MUST be read via the get*Fn accessors and swapped via set*Fn under
// hapiFnMu — the MemGovernor's goroutine (Start) reads them concurrently with a
// test's swap, so raw assignment is a data race (Rule A21), mirroring the
// hapiSampleMu/setHapiSampleFn pattern above.
var (
	hapiFnMu   sync.RWMutex
	governedFn = hapiGovernedPIDs
	suspendFn  = hapiSuspend
	resumeFn   = hapiResume
	killFn     = hapiKill
)

func getGovernedFn() func() map[int]string {
	hapiFnMu.RLock()
	defer hapiFnMu.RUnlock()
	return governedFn
}
func getSuspendFn() func(int, string) error {
	hapiFnMu.RLock()
	defer hapiFnMu.RUnlock()
	return suspendFn
}
func getResumeFn() func(int) error {
	hapiFnMu.RLock()
	defer hapiFnMu.RUnlock()
	return resumeFn
}
func getKillFn() func(int, string) error {
	hapiFnMu.RLock()
	defer hapiFnMu.RUnlock()
	return killFn
}
func setGovernedFn(fn func() map[int]string) {
	hapiFnMu.Lock()
	defer hapiFnMu.Unlock()
	governedFn = fn
}
func setSuspendFn(fn func(int, string) error) {
	hapiFnMu.Lock()
	defer hapiFnMu.Unlock()
	suspendFn = fn
}
func setResumeFn(fn func(int) error)       { hapiFnMu.Lock(); defer hapiFnMu.Unlock(); resumeFn = fn }
func setKillFn(fn func(int, string) error) { hapiFnMu.Lock(); defer hapiFnMu.Unlock(); killFn = fn }

// ── Intervention primitives ────────────────────────────────────────────────

// hapiCanAct is the single A1 gate every suspend/kill passes through: never the
// init/kernel, never ourselves, never a protected process, never a live agent.
func hapiCanAct(pid int, name string) error {
	if pid <= 1 {
		return fmt.Errorf("refusing to act on PID %d (system)", pid)
	}
	if pid == os.Getpid() {
		return fmt.Errorf("refusing to act on self (pid %d)", pid)
	}
	if isProtectedReniceTarget(name) {
		return fmt.Errorf("refusing to act on protected process %q", name)
	}
	if hapiIsAgent(name) {
		return fmt.Errorf("refusing to act on live agent %q", name)
	}
	return nil
}

// hapiSuspend (SIGSTOP), hapiResume (SIGCONT), and hapiKill (SIGTERM) are defined
// per-platform in hapi_signals_unix.go / hapi_signals_windows.go (see the note
// above hapiCanAct). They share hapiCanAct as the A1 gate on Unix.

// ── The governor ───────────────────────────────────────────────────────────

// MemGovernor watches memory pressure and intervenes per the consent tiers.
type MemGovernor struct {
	WarnPercent      float64       // below this free %, surface the runaway
	CriticalPercent  float64       // below this, suspend a governed runaway
	EmergencyPercent float64       // below this, kill governed compute
	Interval         time.Duration // sampling tick
	Govern           bool          // act with teeth on governed PIDs (false = warn-only)

	mu        sync.Mutex
	suspended map[int]string // PIDs Hapi has SIGSTOP'd, to resume on recovery
}

// DefaultMemGovernor returns sane thresholds: warn at 15% free, suspend a governed
// runaway at 8%, kill it at 4%. On a 48 GB host those are ~7.2 / 3.8 / 1.9 GB.
func DefaultMemGovernor() *MemGovernor {
	return &MemGovernor{
		WarnPercent:      15,
		CriticalPercent:  8,
		EmergencyPercent: 4,
		Interval:         3 * time.Second,
		suspended:        map[int]string{},
	}
}

// GovernResult reports one governance pass — what was observed and what was done.
type GovernResult struct {
	Sample         MemSample     `json:"sample"`
	Tier           MemTier       `json:"tier"`
	Runaway        *MemProc      `json:"runaway,omitempty"`
	Actions        []string      `json:"actions,omitempty"`   // human-readable interventions
	Suspended      []int         `json:"suspended,omitempty"` // PIDs paused this pass
	Pressure       PressureLevel `json:"pressure,omitempty"`  // kernel level when a watcher reported, else Unknown
	PressureSource string        `json:"pressure_source,omitempty"`
}

// GovernOnce samples, classifies, and (when Govern is set) intervenes on GOVERNED
// compute only. Non-governed processes are surfaced via Runaway, never auto-acted.
//
// Pressure source: when the kernel dispatch watcher has reported a level (this
// process — i.e. the Hapi daemon), it RAISES the tier floor (memTierFromPressure)
// so the governor reacts to the node-proportional kernel signal, not just the
// free-% heuristic; free-% remains the fallback and the only path to TierEmergency.
func (g *MemGovernor) GovernOnce() (GovernResult, error) {
	s, err := hapiSample()
	if err != nil {
		return GovernResult{Sample: s}, err
	}
	tier := ClassifyMem(s, g.WarnPercent, g.CriticalPercent, g.EmergencyPercent)
	pressureSource := "free-percent"
	pressure := PressureUnknown
	if kl, ok := observedPressure(); ok {
		pressure, pressureSource = kl, "kernel-dispatch"
		if kt := memTierFromPressure(kl); kt > tier {
			tier = kt
		}
	}
	res := GovernResult{Sample: s, Tier: tier, Runaway: FindRunaway(s), Pressure: pressure, PressureSource: pressureSource}

	// Recovery: pressure cleared → resume everything we paused.
	if tier == TierOK {
		g.mu.Lock()
		for pid, name := range g.suspended {
			if getResumeFn()(pid) == nil {
				res.Actions = append(res.Actions, fmt.Sprintf("resumed governed %s (pid %d) — pressure cleared", name, pid))
				stele.Inscribe("hapi", stele.TypeGuardAlert, "", map[string]string{"action": "resume", "pid": strconv.Itoa(pid), "name": name})
			}
			delete(g.suspended, pid)
		}
		g.mu.Unlock()
		return res, nil
	}

	if !g.Govern {
		return res, nil // warn-only: the caller surfaces tier + runaway
	}

	governed := getGovernedFn()()
	rssOf := map[int]int64{}
	for _, p := range s.Top {
		rssOf[p.PID] = p.RSS
	}

	// CRITICAL: suspend the largest not-yet-paused governed process to halt its
	// balloon before the kernel Jetsams. Reversible.
	if tier >= TierCritical {
		var target int
		var tname string
		var tmax int64 = -1
		for pid, name := range governed {
			g.mu.Lock()
			paused := g.suspended[pid] != ""
			g.mu.Unlock()
			if paused {
				continue
			}
			if r := rssOf[pid]; r > tmax {
				tmax, target, tname = r, pid, name
			}
		}
		if target != 0 {
			if getSuspendFn()(target, tname) == nil {
				g.mu.Lock()
				g.suspended[target] = tname
				g.mu.Unlock()
				res.Actions = append(res.Actions, fmt.Sprintf("SUSPENDED governed %s (pid %d, %s resident) — halted its growth before Jetsam; reversible", tname, target, seba.FormatBytes(tmax)))
				res.Suspended = append(res.Suspended, target)
				stele.Inscribe("hapi", stele.TypeGuardAlert, "", map[string]string{"action": "suspend", "pid": strconv.Itoa(target), "name": tname, "tier": tier.String()})
			}
		}
	}

	// EMERGENCY: free is critically low even after suspending — terminate governed
	// compute (it consented; the host comes first). Never a non-governed process.
	if tier >= TierEmergency {
		for pid, name := range governed {
			if getKillFn()(pid, name) == nil {
				g.mu.Lock()
				delete(g.suspended, pid)
				g.mu.Unlock()
				res.Actions = append(res.Actions, fmt.Sprintf("KILLED governed %s (pid %d) — emergency; free RAM critically low", name, pid))
				stele.Inscribe("hapi", stele.TypeGuardAlert, "", map[string]string{"action": "kill", "pid": strconv.Itoa(pid), "name": name, "tier": tier.String()})
			}
		}
	}
	return res, nil
}

// Start runs the governor on a bounded tick until stop is closed. On shutdown it
// resumes anything it paused, so Hapi never leaves a process suspended.
//
// It also subscribes to the kernel memory-pressure watcher (ADR-031-B #4) so the
// governor reacts the INSTANT the kernel escalates — "act immediately on CRITICAL"
// — instead of waiting for the next tick, and re-stamps the cross-process pressure
// cache each pass so one-shot readers (the broker's pre-launch gate, `hapi status`,
// the menubar) see the live kernel level. Fail-soft: a no-op watcher (non-darwin /
// no cgo) simply never sends, and the free-% tick remains the fallback.
func (g *MemGovernor) Start(stop <-chan struct{}, onPass func(GovernResult)) {
	if g.Interval <= 0 {
		g.Interval = 3 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { <-stop; cancel() }()
	events := make(chan PressureEvent, 8)
	_ = NewPressureWatcher().Subscribe(ctx, events)

	stele.Inscribe("hapi", stele.TypeGuardStart, "", map[string]string{"interval": g.Interval.String(), "govern": strconv.FormatBool(g.Govern)})
	t := time.NewTicker(g.Interval)
	defer t.Stop()
	pass := func() {
		res, err := g.GovernOnce()
		if err != nil {
			return
		}
		if lvl, ok := observedPressure(); ok {
			writePressureCache(lvl) // re-stamp: liveness + current level for one-shot readers
		}
		if onPass != nil {
			onPass(res)
		}
	}
	for {
		select {
		case <-stop:
			g.mu.Lock()
			for pid := range g.suspended {
				_ = getResumeFn()(pid)
			}
			g.mu.Unlock()
			stele.Inscribe("hapi", stele.TypeGuardStop, "", nil)
			return
		case <-events:
			pass() // kernel transition — govern now, don't wait for the tick
		case <-t.C:
			pass()
		}
	}
}
