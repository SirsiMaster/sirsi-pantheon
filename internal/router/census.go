// Package router — census.go
//
// Universal Thread Census (owner directive 2026-07-17): EVERY non-system
// agent-class process on this machine — CPU or GPU surface — must exist as a
// registered thread, with no misses, current and future. `thread discover`
// already reconciles INTERACTIVE sessions (claude/codex/… mapped by repo cwd);
// the census covers what discover structurally cannot: INFRASTRUCTURE
// processes with no repo binding — the on-device model broker holding 30 GB of
// Metal memory (the 2026-07-17 balloon P0 was a GPU process registered
// NOWHERE), model workers, servers. A process the registry cannot see is a
// process no board, reaper, or overseer can govern.
//
// Extensibility contract (canon A33): every new agent-class service ships a
// matcher row here IN THE SAME CHANGE. The census runs as a supervisor duty,
// so a future process is caught within one cadence of first launch.
package router

import (
	"os/exec"
	"strconv"
	"strings"
)

// CensusProc is one process from the host enumeration (pid + full command).
type CensusProc struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

// EnumerateCensusProcs lists the host processes (exported for the CLI verb;
// injectable default per Rule A16).
var enumerateProcsFn = defaultEnumerateProcs

func EnumerateCensusProcs() ([]CensusProc, error) { return enumerateProcsFn() }

func defaultEnumerateProcs() ([]CensusProc, error) {
	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil, err
	}
	var procs []CensusProc
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(fields) != 2 {
			continue
		}
		pid, perr := strconv.Atoi(fields[0])
		if perr != nil {
			continue
		}
		procs = append(procs, CensusProc{PID: pid, Command: fields[1]})
	}
	return procs, nil
}

// RunCensusDuty is the supervisor-duty entry point: enumerate + reconcile.
// Never fails the supervisor pass — an enumeration error is returned for the
// duty report and the loop moves on.
func RunCensusDuty(routerRoot, _ string) error {
	procs, err := enumerateProcsFn()
	if err != nil {
		return err
	}
	RunCensus(routerRoot, procs)
	return nil
}

// censusMatcher maps a command substring to the thread identity it must carry.
type censusMatcher struct {
	substr  string
	agentID string
	surface string
}

// censusMatchers is THE registry of infra agent-class processes. Order
// matters: first match wins. Surfaces follow A27 ids; "gpu-server" marks a
// process holding accelerator memory (unified/Metal) so the board and reaper
// see the GPU surface explicitly.
var censusMatchers = []censusMatcher{
	{"gemma-capped-server.py", "gemma", "gpu-server"},
	{"mlx_lm.server", "gemma", "gpu-server"},
	{"mlx_lm.generate", "gemma", "gpu-server"},
	{"sirsi-gemma-worker.sh", "gemma", "worker"},
	{"sirsi-gemma-triage.sh", "gemma", "worker"},
	{"sirsi horus supervise", "horus-supervisor", "worker"},
}

// CensusOutcome classifies the census decision for one process.
type CensusOutcome string

const (
	CensusRegistered CensusOutcome = "registered"      // was unregistered → now a thread
	CensusKnown      CensusOutcome = "already-tracked" // a live thread already carries this pid
)

// CensusAction is one census decision.
type CensusAction struct {
	Proc    CensusProc    `json:"proc"`
	AgentID string        `json:"agent_id"`
	Surface string        `json:"surface"`
	Outcome CensusOutcome `json:"outcome"`
	Thread  string        `json:"thread_id,omitempty"`
	Error   string        `json:"error,omitempty"`
}

// matchCensus returns the identity for a process, or false for non-agent
// (system/user) processes — the census governs agent-class only.
func matchCensus(command string) (censusMatcher, bool) {
	for _, m := range censusMatchers {
		if strings.Contains(command, m.substr) {
			return m, true
		}
	}
	return censusMatcher{}, false
}

// RunCensus reconciles the enumerated processes against the thread registry
// and registers every unregistered agent-class process. Registration reuses
// RegisterThread's (agent, pid) idempotency + machine-id stamping, so a second
// pass over the same process is a no-op and a foreign machine's registry is
// never touched. Read-only toward processes: the census REGISTERS, it never
// kills — governance (reaping, resizing) stays with the reaper/broker.
func RunCensus(routerRoot string, procs []CensusProc) []CensusAction {
	var actions []CensusAction
	reg, err := LoadThreadRegistry(routerRoot)
	known := map[int]bool{}
	if err == nil {
		this := MachineID()
		for _, t := range reg.Threads {
			if t == nil || t.Status.IsTerminal() || !SameMachine(t.MachineID, this) {
				continue
			}
			known[t.PID] = true
		}
	}
	for _, p := range procs {
		m, ok := matchCensus(p.Command)
		if !ok {
			continue
		}
		a := CensusAction{Proc: p, AgentID: m.agentID, Surface: m.surface}
		if known[p.PID] {
			a.Outcome = CensusKnown
			actions = append(actions, a)
			continue
		}
		thr, rerr := RegisterThread(routerRoot, &Thread{
			AgentID: m.agentID,
			Surface: m.surface,
			PID:     p.PID,
			Status:  ThreadStatusActive,
		})
		if rerr != nil {
			a.Error = rerr.Error()
		} else {
			a.Outcome = CensusRegistered
			a.Thread = thr.ThreadID
		}
		actions = append(actions, a)
	}
	return actions
}
