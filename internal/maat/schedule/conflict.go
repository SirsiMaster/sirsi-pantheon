package schedule

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
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
	known := make(map[string]Reservation, len(holders))
	for _, h := range holders {
		known[h.Holder] = h
	}

	actors, err := getActivityProbe()(machine)
	if err != nil {
		return ConflictReport{}, err
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

// probeProcesses is the default ActivityProbe: it classifies the currently
// running processes that contaminate Thunderbolt/measurement work. Owner
// attribution is best-effort (left "" when a PID cannot be tied to an agent);
// an unattributed intruder is still reported and named by its argv.
func probeProcesses(machine string) ([]Actor, error) {
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
		kind := classifyProc(line)
		if kind == "" {
			continue
		}
		actors = append(actors, Actor{Kind: kind, Detail: firstFields(line, 6)})
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
	switch {
	case containsAny(lc, "tbraw-bench", "tcp-bench", "tbraw ", "rail-bench", "hermes-bench", "iperf"):
		return "bench"
	case containsAny(lc, "go build", "vite build", "xcodebuild", "cargo build", "runner.worker"):
		return "build"
	case containsAny(lc, "mlx_lm", "gemma serve", "sne-runner", "llama", "model-load"):
		return "model"
	}
	return ""
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
