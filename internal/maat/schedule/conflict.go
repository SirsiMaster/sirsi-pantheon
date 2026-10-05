package schedule

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// Actor is one process/load source active on a machine right now, as seen by an
// ActivityProbe. Kind is a coarse class ("bench", "build", "model", "runner",
// "other"); Detail is a short human string (argv fragment); Owner is the agent
// id when it can be attributed, else "".
type Actor struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	Owner  string `json:"owner,omitempty"`
	PID    int    `json:"pid,omitempty"`
}

// ActivityProbe samples the load/traffic actors on a machine. It is injectable so
// tests drive it deterministically and hosts plug rail-specific detectors later
// (A16). The default probeProcesses walks `ps` for the process classes that
// contaminate measurement — closing the FOREIGN_EXCLUDE hole (tbraw-bench and
// tcp-bench were deliberately ignored before; here they are intruders).
type ActivityProbe func(machine string) ([]Actor, error)

var (
	probeMu     sync.RWMutex
	activeProbe ActivityProbe = probeProcesses
)

// SetActivityProbe installs a probe (A21: guarded, not a bare package var).
func SetActivityProbe(p ActivityProbe) {
	probeMu.Lock()
	defer probeMu.Unlock()
	activeProbe = p
}

func getActivityProbe() ActivityProbe {
	probeMu.RLock()
	defer probeMu.RUnlock()
	return activeProbe
}

// ConflictReport is the result of one conflict check on a reserved resource.
type ConflictReport struct {
	Resource    string   `json:"resource"`
	Reservation string   `json:"reservation_id"`
	Holder      string   `json:"holder"`
	Clean       bool     `json:"clean"`
	Intruders   []Actor  `json:"intruders,omitempty"`
	Shared      []string `json:"shared,omitempty"` // granted floor-share lanes seen active alongside the holder — not intruders
	Summary     string   `json:"summary,omitempty"`
	// Pressure/PressureSource/FloorMemGB are reported alongside the process
	// scan, never folded into Clean/Intruders: memory pressure isn't an Actor
	// (no process is "wrong"), so it doesn't become an Intruder — it's a
	// parallel signal the caller (CLI/human) sees beside the clean process
	// list, closing the gap where a clean report can coexist with a host
	// that's swapping.
	Pressure       guard.PressureLevel `json:"pressure,omitempty"`
	PressureSource string              `json:"pressure_source,omitempty"`
	FloorMemGB     int                 `json:"floor_mem_gb,omitempty"` // FloorShareMemGB(resource), informational
}

// CheckConflicts samples current activity on the resource's machine and reports
// any actor that is NOT the primary holder, and not a lane holding a Ma'at-
// granted floor share on the same resource, as an intruder — for a quiet or
// loaded reservation that is live now. A build-regime reservation tolerates
// foreign build load. A floor-share lane is recorded in Shared (owner
// directive 2026-09-26: Ma'at grants floor shares deliberately, so a lane
// running on one is expected activity, never a contaminant to invalidate).
// This DETECTS and reports; the caller decides whether to Invalidate and
// notify (the CLI does both). `machine` is the machine to probe; for a rail
// resource, pass the machine that owns the rail's near end.
func (l *Ledger) CheckConflicts(resource, machine string) (ConflictReport, error) {
	holders, err := l.LiveHolders(resource)
	if err != nil {
		return ConflictReport{}, err
	}
	rep := ConflictReport{Resource: resource, Clean: true}
	if len(holders) == 0 {
		rep.Summary = "no live reservation covers " + resource + " now"
		return rep, nil
	}
	cur := primaryHolder(holders)
	rep.Reservation = cur.ID
	rep.Holder = cur.Holder
	if cur.Regime == RegimeBuild {
		rep.Summary = "build-regime reservation tolerates foreign load"
		return rep, nil
	}
	rep.Pressure, rep.PressureSource = getPressureFn()()
	if floorMemGB, ferr := l.FloorShareMemGB(resource); ferr == nil {
		rep.FloorMemGB = floorMemGB
	}
	known := make(map[string]Reservation, len(holders))
	for _, h := range holders {
		known[h.Holder] = h
	}

	actors, err := getActivityProbe()(machine)
	if err != nil {
		return ConflictReport{}, err
	}
	var ancestry map[int]int
	if cur.ExemptPID != 0 {
		// Only walk the process tree when a reservation actually named an
		// exempt PID (ponytail: no extra `ps` call on the common path). A
		// lookup failure here just means the exemption doesn't apply this
		// round — fail toward reporting, not toward silently clearing load.
		ancestry, _ = getProcessAncestryFn()()
	}
	sharedSeen := map[string]bool{}
	for _, a := range actors {
		if a.Owner != "" {
			if h, ok := known[a.Owner]; ok {
				if h.Holder != cur.Holder && h.Share == ShareFloor && !sharedSeen[h.Holder] {
					sharedSeen[h.Holder] = true
					rep.Shared = append(rep.Shared, fmt.Sprintf("shared: %s floor %d cores", h.Holder, h.Cores))
				}
				continue // the primary holder's own work, or a granted floor-share lane
			}
		}
		if a.Kind == "other" {
			continue // benign background, not a measurement contaminant
		}
		if ancestry != nil && a.PID != 0 &&
			(isDescendant(a.PID, cur.ExemptPID, ancestry) || isDescendant(cur.ExemptPID, a.PID, ancestry)) {
			continue // the reservation's own run: a descendant (e.g. its ssh launcher)
			// or an ancestor (e.g. the parent shell that exec'd the run-guard
			// script) — not foreign load
		}
		rep.Intruders = append(rep.Intruders, a)
	}
	if len(rep.Intruders) > 0 {
		rep.Clean = false
		var parts []string
		for _, a := range rep.Intruders {
			d := a.Kind
			if a.Detail != "" {
				d += ":" + a.Detail
			}
			if a.Owner != "" {
				d += "(" + a.Owner + ")"
			}
			parts = append(parts, d)
		}
		rep.Summary = "foreign " + strings.Join(parts, ", ") + " during " + string(cur.Regime) +
			" reservation " + cur.ID + " held by " + cur.Holder
	} else {
		rep.Summary = "clean: only the holder's work (and any granted floor shares) is active"
	}
	if rep.Pressure >= guard.PressureWarn {
		rep.Summary += fmt.Sprintf(" — memory pressure %s (floor %dGB)", rep.Pressure, rep.FloorMemGB)
	}
	return rep, nil
}

// primaryHolder picks the reservation CheckConflicts reports as "the"
// holder when several are live on the same resource at once: the full-share
// one if there is one, else the first live holder.
func primaryHolder(holders []Reservation) Reservation {
	for _, h := range holders {
		if h.Share != ShareFloor {
			return h
		}
	}
	return holders[0]
}

var (
	localLabelMu sync.RWMutex
	localLabelFn = defaultLocalMachineLabel
)

// SetLocalMachineLabelFn installs a local-machine-label resolver (tests; A21:
// guarded by a mutex, not a bare package var).
func SetLocalMachineLabelFn(fn func() (string, error)) {
	localLabelMu.Lock()
	defer localLabelMu.Unlock()
	if fn == nil {
		fn = defaultLocalMachineLabel
	}
	localLabelFn = fn
}

func getLocalMachineLabelFn() func() (string, error) {
	localLabelMu.RLock()
	defer localLabelMu.RUnlock()
	return localLabelFn
}

// defaultLocalMachineLabel reads what this host calls itself — the macOS
// ComputerName (which is how the m1/m5 resource labels are actually set on
// these machines), falling back to os.Hostname. It never guesses a mapping.
func defaultLocalMachineLabel() (string, error) {
	if out, err := exec.Command("scutil", "--get", "ComputerName").Output(); err == nil {
		if name := strings.ToLower(strings.TrimSpace(string(out))); name != "" {
			return name, nil
		}
	}
	host, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("resolve local machine label: %w", err)
	}
	host = strings.ToLower(strings.TrimSuffix(host, ".local"))
	if host == "" {
		return "", fmt.Errorf("resolve local machine label: empty hostname")
	}
	return host, nil
}

// probeProcesses is the default ActivityProbe: it classifies the currently
// running processes that contaminate Thunderbolt/measurement work. Owner
// attribution is best-effort (left "" when a PID cannot be tied to an agent);
// an unattributed intruder is still reported and named by its argv.
//
// It can only ever see THIS host's process table — there is no cross-host
// probe. Passing a machine that isn't this one used to silently scan local ps
// and report it as the named machine's activity (2026-09-27, three failed
// Mercury signing attempts: an M1 conflict-check run against "m5" reported the
// M1's own idle runner as an M5 intruder). Refuse instead of mislabeling.
func probeProcesses(machine string) ([]Actor, error) {
	if machine != "" {
		local, err := getLocalMachineLabelFn()()
		if err != nil {
			return nil, fmt.Errorf("probe %s: %w", machine, err)
		}
		if !strings.EqualFold(machine, local) {
			return nil, fmt.Errorf("probe %s: this host is %q — no cross-host process probe exists; run conflict-check on %s itself", machine, local, machine)
		}
	}
	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil, err
	}
	var actors []Actor
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidStr, cmd, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		cmd = strings.TrimSpace(cmd)
		kind := classifyProc(cmd)
		if kind == "" {
			continue
		}
		pid, _ := strconv.Atoi(pidStr) // 0 on parse failure: exemption lookups simply never match it
		actors = append(actors, Actor{Kind: kind, Detail: firstFields(cmd, 6), PID: pid})
	}
	return actors, nil
}

// classifyProc maps a `ps` command line to a contaminant class, or "" to ignore.
//
// A self-hosted GitHub Actions runner is IDLE — no job in flight, not load —
// whenever only its service processes are up: Runner.Listener (waits for
// jobs) and runsvc.sh (the launchd wrapper that starts it). Both live under
// an "actions-runner" path, same as the runner's actual job-execution
// process, Runner.Worker. Matching on the path substring ("actions-runner")
// or the listener's own start script ("run.sh") flagged the idle listener as
// build load and killed a live reservation (2026-09-26, M1, rc8 signing run,
// router item 20260926-171755). Only Runner.Worker — spawned per job — is load.
func classifyProc(cmd string) string {
	lc := strings.ToLower(cmd)
	// A shell wrapper (`zsh -c "<script text>"`) carries its whole script in argv, so
	// words like "go build" or "tbraw " inside it matched as if the shell were the
	// load. The real work is a child process and is classified on its own.
	if isShellWrapper(lc) {
		return ""
	}
	switch {
	case containsAny(lc, "tbraw-bench", "tcp-bench", "tbraw ", "rail-bench", "mercury-bench", "iperf"):
		return "bench"
	case containsAny(lc, "go build", "vite build", "xcodebuild", "cargo build", "runner.worker"):
		return "build"
	case containsAny(lc, "mlx_lm", "gemma serve", "sne-runner", "llama", "model-load"):
		return "model"
	}
	return ""
}

// LiveActivity reports the contaminating actors on this host right now,
// independent of any reservation. `who-is-on` uses it so "free" never reads as
// "idle": a reservation ledger only knows who asked, not who is running
// (codex-apollo repro 2026-09-26, FinalWishes work running while the ledger said free).
func LiveActivity(machine string) ([]Actor, error) {
	return getActivityProbe()(machine)
}

// isShellWrapper reports whether cmd is a shell started with -c.
func isShellWrapper(lc string) bool {
	fields := strings.Fields(lc)
	if len(fields) < 2 {
		return false
	}
	base := fields[0][strings.LastIndex(fields[0], "/")+1:]
	if base != "zsh" && base != "bash" && base != "sh" {
		return false
	}
	for _, f := range fields[1:] {
		if f == "-c" || f == "-lc" || f == "-ic" || f == "-lic" {
			return true
		}
		if !strings.HasPrefix(f, "-") {
			return false
		}
	}
	return false
}

// ProcessAncestryFn returns pid -> ppid for every process this host can see.
// Injectable (A21: guarded, not a bare package var) so tests drive ancestry
// deterministically without shelling out to `ps`.
type ProcessAncestryFn func() (map[int]int, error)

var (
	ancestryMu sync.RWMutex
	ancestryFn ProcessAncestryFn = processAncestry
)

// SetProcessAncestryFn installs a process-ancestry resolver (tests).
func SetProcessAncestryFn(fn ProcessAncestryFn) {
	ancestryMu.Lock()
	defer ancestryMu.Unlock()
	if fn == nil {
		fn = processAncestry
	}
	ancestryFn = fn
}

func getProcessAncestryFn() ProcessAncestryFn {
	ancestryMu.RLock()
	defer ancestryMu.RUnlock()
	return ancestryFn
}

// processAncestry walks `ps` once for pid/ppid pairs across every process
// this host can see — the same process table probeProcesses already reads,
// just with ppid instead of command.
func processAncestry() (map[int]int, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return nil, err
	}
	parents := make(map[int]int)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		parents[pid] = ppid
	}
	return parents, nil
}

// isDescendant reports whether pid is ancestor, or a descendant of ancestor,
// by walking parents up to the root. Bounded so a cyclic or self-referential
// ppid entry (a stale/reused PID) can never spin forever.
func isDescendant(pid, ancestor int, parents map[int]int) bool {
	for steps := 0; pid != 0 && steps < 4096; steps++ {
		if pid == ancestor {
			return true
		}
		next, ok := parents[pid]
		if !ok || next == pid {
			return false
		}
		pid = next
	}
	return false
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func firstFields(s string, n int) string {
	f := strings.Fields(s)
	if len(f) > n {
		f = f[:n]
	}
	return strings.Join(f, " ")
}
