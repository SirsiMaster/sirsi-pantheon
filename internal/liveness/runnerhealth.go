// Package liveness — runnerhealth.go
//
// Wedge detection for Pantheon self-hosted GitHub Actions runners (ADR-042).
//
// A "wedged" runner is one whose Runner.Listener process is alive in launchd
// but whose GitHub registration reads `offline`. The OS table says "running",
// the GH API says "dead", and GitHub's UI says "queued" — none of the three
// surfaces agree, and the jobs assigned to that runner sit waiting forever.
//
// # Three distinguishable states (not two)
//
// This probe implements three honest states (ADR-057: do not collapse "cannot
// tell" into "unhealthy"):
//
//  1. Healthy     — runner status is consistent (online, or stopped with no PID).
//  2. Wedged      — PID alive + GH=offline + Actions component=operational.
//     A genuine process wedge; remediation warranted.
//  3. Suppressed  — PID alive + GH=offline + Actions component=degraded/major_outage.
//     Offline status is expected during an incident; suppressing prevents false pages
//     at the exact moment the fleet can do nothing about them.
//
// The distinguishing signal (githubstatus.com/api/v2/components.json) is cheap
// and remains reachable precisely when you need it — the "API Requests" component
// stayed operational through the 2026-08-06 major_outage even as "Actions" degraded.
//
// Safety (A32/ADR-040): ProbeRunnerWedge is strictly read-and-report. It never
// sends a signal, calls launchctl kickstart, or touches the runner process.
// Remediation stays with the routed recipient.
package liveness

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// pantheonRunnerRepo is the repo whose runner list we query.
const pantheonRunnerRepo = "SirsiMaster/sirsi-pantheon"

// runnerLaunchdPrefix is the launchd label prefix for all Pantheon runners.
const runnerLaunchdPrefix = "actions.runner.SirsiMaster-sirsi-pantheon."

// githubStatusURL is the GitHub status component API — reachable via "API
// Requests" even during an Actions-only outage.
const githubStatusURL = "https://www.githubstatus.com/api/v2/components.json"

// GHRunner is one GitHub Actions self-hosted runner returned by the API.
type GHRunner struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "online" | "offline"
	Busy   bool   `json:"busy"`
}

// RunnerHealthState is the three-valued liveness verdict for one runner.
type RunnerHealthState int

const (
	// RunnerStateUnknown — GH API unavailable; cannot make a verdict.
	RunnerStateUnknown RunnerHealthState = iota
	// RunnerStateHealthy — runner status is consistent (online, or stopped).
	RunnerStateHealthy
	// RunnerStateWedged — PID alive + GH=offline + Actions=operational.
	// Genuine wedge: remediation warranted.
	RunnerStateWedged
	// RunnerStateSuppressed — PID alive + GH=offline + Actions=degraded/outage.
	// Offline is expected during an incident; suppress the alarm.
	RunnerStateSuppressed
)

// RunnerHealth is the combined OS+API liveness view of one runner.
type RunnerHealth struct {
	Name        string
	Label       string
	ListenerPID int    // >0 = launchd has a live process
	GHStatus    string // "online" | "offline" | "" if unknown
	GHBusy      bool
	// State is the three-valued verdict.
	State RunnerHealthState
	// Wedged is true when State == RunnerStateWedged (compat shim for callers
	// that predated the three-state model; new callers should switch on State).
	Wedged bool
}

// ghStatusComponent is one entry from the githubstatus components response.
type ghStatusComponent struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "operational" | "degraded_performance" | "partial_outage" | "major_outage"
}

type ghStatusResponse struct {
	Components []ghStatusComponent `json:"components"`
}

// discoverRunnersFn returns label → PID for every matching runner label found
// by `launchctl list`. Using bare `launchctl list` (no label arg): the bare form
// outputs a tab-separated table (PID \t LastExitStatus \t Label) where PID is
// already present. The per-label form outputs a plist-style dict whose first
// line is "{" — strconv.Atoi("{") always fails. One exec, one format, zero ambiguity.
var (
	discoverRunnersMu sync.RWMutex
	discoverRunnersFn = defaultDiscoverRunners
)

func getDiscoverRunnersFn() func() map[string]int {
	discoverRunnersMu.RLock()
	defer discoverRunnersMu.RUnlock()
	return discoverRunnersFn
}

func setDiscoverRunnersFn(fn func() map[string]int) {
	discoverRunnersMu.Lock()
	defer discoverRunnersMu.Unlock()
	if fn == nil {
		fn = defaultDiscoverRunners
	}
	discoverRunnersFn = fn
}

// defaultDiscoverRunners calls `launchctl list` and returns label → PID for
// every service label matching runnerLaunchdPrefix whose .plist exists on disk.
// PID 0 means the service is loaded but not running. Returns nil on non-macOS
// or exec error.
func defaultDiscoverRunners() map[string]int {
	if runtime.GOOS != "darwin" {
		return nil
	}
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return nil
	}
	home, _ := os.UserHomeDir()
	agentDir := ""
	if home != "" {
		agentDir = home + "/Library/LaunchAgents/"
	}

	result := map[string]int{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		label := fields[2]
		if !strings.HasPrefix(label, runnerLaunchdPrefix) {
			continue
		}
		// Only include labels whose .plist exists on disk (alive, not retired).
		if agentDir != "" {
			if _, statErr := os.Stat(agentDir + label + ".plist"); statErr != nil {
				continue
			}
		}
		pid := 0
		if fields[0] != "-" {
			if n, atoiErr := strconv.Atoi(fields[0]); atoiErr == nil && n > 0 {
				pid = n
			}
		}
		result[label] = pid
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// ghRunnersFn is the injectable GitHub API prober (Rule A16/A21). Default calls
// `gh api` to list runners for pantheonRunnerRepo. Returns nil on error
// (fail-open — don't alarm without data).
var (
	ghRunnersMu sync.RWMutex
	ghRunnersFn = defaultGHRunners
)

func getGHRunnersFn() func(repo string) ([]GHRunner, error) {
	ghRunnersMu.RLock()
	defer ghRunnersMu.RUnlock()
	return ghRunnersFn
}

func setGHRunnersFn(fn func(repo string) ([]GHRunner, error)) {
	ghRunnersMu.Lock()
	defer ghRunnersMu.Unlock()
	if fn == nil {
		fn = defaultGHRunners
	}
	ghRunnersFn = fn
}

func defaultGHRunners(repo string) ([]GHRunner, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found")
	}
	out, err := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/actions/runners", repo),
		"--jq", ".runners").Output()
	if err != nil {
		return nil, fmt.Errorf("gh api runners: %w", err)
	}
	var runners []GHRunner
	if err := json.Unmarshal(out, &runners); err != nil {
		return nil, fmt.Errorf("parse runners json: %w", err)
	}
	return runners, nil
}

// actionsOperationalFn probes the GitHub Actions component status (Rule A16/A21).
// Returns true when the Actions component is fully operational, false when
// degraded or in outage. Fail-open: returns true on any error so a network
// failure does NOT suppress a genuine wedge alarm.
var (
	actionsOperationalMu sync.RWMutex
	actionsOperationalFn = defaultActionsOperational
)

func getActionsOperationalFn() func() bool {
	actionsOperationalMu.RLock()
	defer actionsOperationalMu.RUnlock()
	return actionsOperationalFn
}

func setActionsOperationalFn(fn func() bool) {
	actionsOperationalMu.Lock()
	defer actionsOperationalMu.Unlock()
	if fn == nil {
		fn = defaultActionsOperational
	}
	actionsOperationalFn = fn
}

// defaultActionsOperational fetches githubstatus.com and returns true iff the
// "Actions" component reports "operational". Fail-open (returns true) on any
// fetch or parse error — a network failure must NOT suppress a genuine wedge.
func defaultActionsOperational() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(githubStatusURL)
	if err != nil {
		return true // fail-open
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return true
	}
	var sr ghStatusResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return true
	}
	for _, c := range sr.Components {
		if c.Name == "Actions" {
			return c.Status == "operational"
		}
	}
	return true // component not found — fail-open
}

// CheckRunnerHealth returns one RunnerHealth per self-hosted runner found on
// this host. The State field carries the three-valued verdict.
// Exported so callers (node-status, doctor) can surface it without re-probing.
func CheckRunnerHealth() []RunnerHealth {
	if runtime.GOOS != "darwin" {
		return nil
	}

	labelPIDs := getDiscoverRunnersFn()()
	if len(labelPIDs) == 0 {
		return nil
	}

	ghByName := map[string]GHRunner{}
	ghAPIAvailable := false
	ghRunners, err := getGHRunnersFn()(pantheonRunnerRepo)
	if err == nil {
		ghAPIAvailable = true
		for _, r := range ghRunners {
			ghByName[r.Name] = r
		}
	}

	// ponytail: single fetch shared across all runners in one pass; only fired
	// when the first offline-with-PID runner is encountered.
	actionsOKKnown := false
	actionsOK := true // default fail-open

	var results []RunnerHealth
	for label, pid := range labelPIDs {
		name := strings.TrimPrefix(label, runnerLaunchdPrefix)
		gh := ghByName[name] // zero value if not in GH response

		var state RunnerHealthState
		switch {
		case !ghAPIAvailable:
			// Cannot make a verdict without the control-plane view.
			state = RunnerStateUnknown
		case pid > 0 && gh.Status == "offline":
			// Potential wedge — need the Actions component status to decide.
			if !actionsOKKnown {
				actionsOK = getActionsOperationalFn()()
				actionsOKKnown = true
			}
			if actionsOK {
				state = RunnerStateWedged
			} else {
				state = RunnerStateSuppressed
			}
		default:
			state = RunnerStateHealthy
		}

		results = append(results, RunnerHealth{
			Name:        name,
			Label:       label,
			ListenerPID: pid,
			GHStatus:    gh.Status,
			GHBusy:      gh.Busy,
			State:       state,
			Wedged:      state == RunnerStateWedged,
		})
	}
	return results
}

// ProbeRunnerWedge checks Pantheon's self-hosted runners for the wedge state
// and returns a Finding for the liveness watch. macOS only; fail-open (returns
// OK when gh is unavailable or no runners are registered).
//
// Three outcomes:
//   - OK: all runners healthy or GH API unavailable (fail-open).
//   - OK + detail "control-plane outage": runners offline but Actions degraded —
//     suppressed; not a genuine wedge.
//   - Not-OK: one or more genuine wedges (Actions operational, PID alive, GH offline).
func ProbeRunnerWedge() Finding {
	f := Finding{Check: "ci-runner-wedge"}

	if runtime.GOOS != "darwin" {
		f.OK, f.Detail = true, "not macOS"
		return f
	}

	runners := CheckRunnerHealth()
	if len(runners) == 0 {
		f.OK, f.Detail = true, "no self-hosted runner labels found"
		return f
	}

	var wedged, suppressed []RunnerHealth
	var details []string
	for _, r := range runners {
		switch r.State {
		case RunnerStateWedged:
			wedged = append(wedged, r)
			details = append(details, fmt.Sprintf("%s: PID %d alive, GH=offline, Actions=operational (WEDGED)", r.Name, r.ListenerPID))
		case RunnerStateSuppressed:
			suppressed = append(suppressed, r)
			details = append(details, fmt.Sprintf("%s: PID %d alive, GH=offline, Actions=degraded/outage (suppressed)", r.Name, r.ListenerPID))
		case RunnerStateUnknown:
			details = append(details, fmt.Sprintf("%s: PID %d, GH=unknown (API unavailable)", r.Name, r.ListenerPID))
		default:
			busy := ""
			if r.GHBusy {
				busy = " busy=true"
			}
			details = append(details, fmt.Sprintf("%s: PID %d, GH=%s%s", r.Name, r.ListenerPID, r.GHStatus, busy))
		}
	}

	f.Detail = strings.Join(details, "; ")

	if len(suppressed) > 0 && len(wedged) == 0 {
		// Runners appear offline but Actions is degraded — suppress, don't alarm.
		var names []string
		for _, r := range suppressed {
			names = append(names, r.Name)
		}
		f.OK = true
		f.Detail = fmt.Sprintf("control-plane outage: Actions component degraded/major_outage; "+
			"%d runner(s) offline status suppressed: %s — %s",
			len(suppressed), strings.Join(names, ", "), f.Detail)
		return f
	}

	if len(wedged) == 0 {
		f.OK = true
		return f
	}

	// One or more genuine wedged runners. Build a deterministic title (dedup key).
	var names []string
	for _, r := range wedged {
		names = append(names, r.Name)
	}
	f.Title = fmt.Sprintf("liveness-watch: ci-runner wedged: %s", strings.Join(names, ", "))
	f.Fixable = true
	f.Body = fmt.Sprintf(
		"The liveness watch detected %d self-hosted runner(s) in wedge state: "+
			"the Runner.Listener process is alive in launchd but GitHub API reports the runner as offline, "+
			"and the GitHub Actions component is currently operational (not in an outage). "+
			"CI jobs assigned to this runner sit queued indefinitely with no listener to claim them.\n\n"+
			"Affected runner(s): %s\n\n"+
			"Repair (already applied by conduit if noted there):\n"+
			"  launchctl kickstart -k gui/%d/%s\n\n"+
			"Root cause hypothesis: a failed job leaves the listener unrecoverable "+
			"(observed: both wedges followed a 'result: Failed' log entry with nothing after). "+
			"Confirm: check whether the next 'result: Failed' on either runner is again the last "+
			"line in its log before it re-wedges.\n\n"+
			"Refs: ADR-042, PANTHEON_RULES.md Rule A1 (safety), ADR-040 (do no harm), ADR-057 (honest states).",
		len(wedged),
		strings.Join(names, ", "),
		os.Getuid(),
		strings.Join(func() []string {
			ls := make([]string, len(wedged))
			for i, r := range wedged {
				ls[i] = r.Label
			}
			return ls
		}(), "\n  launchctl kickstart -k gui/"+strconv.Itoa(os.Getuid())+"/"),
	)
	return f
}
