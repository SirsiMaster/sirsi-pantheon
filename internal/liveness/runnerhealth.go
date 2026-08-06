// Package liveness — runnerhealth.go
//
// Wedge detection for Pantheon self-hosted GitHub Actions runners (ADR-042).
//
// A "wedged" runner is one whose Runner.Listener process is alive in launchd
// but whose GitHub registration reads `offline`. The OS table says "running",
// the GH API says "dead", and GitHub's UI says "queued" — none of the three
// surfaces agree, and the jobs assigned to that runner sit waiting forever.
//
// This probe surfaces the mismatch as a first-class unhealthy state so the
// liveness watch can route a remediation item to claude-pantheon before the
// next conduit pass has to notice it manually.
//
// Safety (A32/ADR-040): ProbeRunnerWedge is strictly read-and-report. It never
// sends a signal, calls launchctl kickstart, or touches the runner process.
// Remediation stays with the routed recipient.
package liveness

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// pantheonRunnerRepo is the repo whose runner list we query. Hard-coded to the
// Pantheon fleet; a future generalisation can make this injectable.
const pantheonRunnerRepo = "SirsiMaster/sirsi-pantheon"

// runnerLaunchdPrefix is the launchd label prefix for all Pantheon runners.
const runnerLaunchdPrefix = "actions.runner.SirsiMaster-sirsi-pantheon."

// GHRunner is one GitHub Actions self-hosted runner returned by the API.
type GHRunner struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "online" | "offline"
	Busy   bool   `json:"busy"`
}

// RunnerHealth is the combined OS+API liveness view of one runner.
type RunnerHealth struct {
	Name        string // e.g. "m5-sirsi"
	Label       string // launchd label, e.g. "actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"
	ListenerPID int    // >0 = launchd has a live PID for this service
	GHStatus    string // "online" | "offline" | "" if unknown
	GHBusy      bool
	Wedged      bool // true = PID alive + GH says offline
}

// ghRunnersFn is the injectable GitHub API prober (Rule A16/A21). Default
// calls `gh api` to list runners for pantheonRunnerRepo. Returns nil on error
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

// defaultGHRunners calls `gh api repos/<repo>/actions/runners --jq .runners`
// and returns the list. Fails open when gh is absent or the API is unavailable.
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

// launchctlRunnerPIDFn is the injectable launchd PID prober (Rule A16/A21).
// Returns the PID of the launchd service for the given label, or 0 if the
// service has no live process. Fail-open: returns 0 on any error.
var (
	launchctlRunnerPIDMu sync.RWMutex
	launchctlRunnerPIDFn = defaultLaunchctlRunnerPID
)

func getLaunchctlRunnerPIDFn() func(label string) int {
	launchctlRunnerPIDMu.RLock()
	defer launchctlRunnerPIDMu.RUnlock()
	return launchctlRunnerPIDFn
}

func setLaunchctlRunnerPIDFn(fn func(label string) int) {
	launchctlRunnerPIDMu.Lock()
	defer launchctlRunnerPIDMu.Unlock()
	if fn == nil {
		fn = defaultLaunchctlRunnerPID
	}
	launchctlRunnerPIDFn = fn
}

// defaultLaunchctlRunnerPID reads the PID from `launchctl list <label>`. The
// output is tab-separated: PID \t LastExitStatus \t Label. A dash in the PID
// column means the service has no running process. macOS only — returns 0
// immediately on other platforms.
func defaultLaunchctlRunnerPID(label string) int {
	if runtime.GOOS != "darwin" {
		return 0
	}
	out, err := exec.Command("launchctl", "list", label).Output()
	if err != nil {
		// service not loaded or unknown label
		return 0
	}
	// `launchctl list` without any arguments lists ALL services tab-separated;
	// `launchctl list <label>` prints only that label's entry. We parse the
	// first field of the first output line.
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	fields := strings.Fields(line)
	if len(fields) < 1 || fields[0] == "-" {
		return 0
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// discoverRunnerLabels lists all launchd service labels on the current host
// that match runnerLaunchdPrefix. macOS only; returns nil on other platforms.
var (
	discoverRunnerLabelsMu sync.RWMutex
	discoverRunnerLabelsFn = defaultDiscoverRunnerLabels
)

func getDiscoverRunnerLabelsFn() func() []string {
	discoverRunnerLabelsMu.RLock()
	defer discoverRunnerLabelsMu.RUnlock()
	return discoverRunnerLabelsFn
}

func setDiscoverRunnerLabelsFn(fn func() []string) {
	discoverRunnerLabelsMu.Lock()
	defer discoverRunnerLabelsMu.Unlock()
	if fn == nil {
		fn = defaultDiscoverRunnerLabels
	}
	discoverRunnerLabelsFn = fn
}

// defaultDiscoverRunnerLabels calls `launchctl list` and returns service labels
// matching runnerLaunchdPrefix. Returns nil on non-macOS or exec error.
func defaultDiscoverRunnerLabels() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	// launchctl list (no label arg) outputs: PID \t LastExitStatus \t Label
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return nil
	}
	home, _ := os.UserHomeDir()
	agentDir := ""
	if home != "" {
		agentDir = home + "/Library/LaunchAgents/"
	}

	var labels []string
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
			if _, err := os.Stat(agentDir + label + ".plist"); err != nil {
				continue
			}
		}
		labels = append(labels, label)
	}
	return labels
}

// CheckRunnerHealth returns one RunnerHealth per self-hosted runner found on
// this host. Wedged = launchd PID alive AND GitHub API reports offline. Exported
// so callers (node-status, doctor) can surface it without re-probing.
func CheckRunnerHealth() []RunnerHealth {
	if runtime.GOOS != "darwin" {
		return nil
	}

	labels := getDiscoverRunnerLabelsFn()()
	if len(labels) == 0 {
		return nil
	}

	// Build name → GH status index (fail-open: if the API call fails, we still
	// report PID state but mark GH status as unknown).
	ghByName := map[string]GHRunner{}
	ghRunners, err := getGHRunnersFn()(pantheonRunnerRepo)
	if err == nil {
		for _, r := range ghRunners {
			ghByName[r.Name] = r
		}
	}

	pidFn := getLaunchctlRunnerPIDFn()
	var results []RunnerHealth

	for _, label := range labels {
		name := strings.TrimPrefix(label, runnerLaunchdPrefix)
		pid := pidFn(label)
		gh := ghByName[name] // zero value if not found
		h := RunnerHealth{
			Name:        name,
			Label:       label,
			ListenerPID: pid,
			GHStatus:    gh.Status,
			GHBusy:      gh.Busy,
			// Wedged: the local process is alive but GitHub has lost the runner.
			// Only set when we have positive evidence from BOTH sides —
			// pid > 0 (launchd has a live process) AND GH explicitly says "offline".
			// If the API call failed (gh.Status == ""), we cannot conclude wedged.
			Wedged: pid > 0 && gh.Status == "offline",
		}
		results = append(results, h)
	}
	return results
}

// ProbeRunnerWedge checks Pantheon's self-hosted runners for the wedge state
// and returns a Finding for the liveness watch. macOS only; fail-open (returns
// OK when gh is unavailable or no runners are registered).
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

	var wedged []RunnerHealth
	var details []string
	for _, r := range runners {
		switch {
		case r.Wedged:
			wedged = append(wedged, r)
			details = append(details, fmt.Sprintf("%s: PID %d alive, GH=offline (WEDGED)", r.Name, r.ListenerPID))
		case r.GHStatus == "":
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

	if len(wedged) == 0 {
		f.OK = true
		return f
	}

	// One or more wedged runners. Build a deterministic title (dedup key).
	var names []string
	for _, r := range wedged {
		names = append(names, r.Name)
	}
	f.Title = fmt.Sprintf("liveness-watch: ci-runner wedged: %s", strings.Join(names, ", "))
	f.Fixable = true
	f.Body = fmt.Sprintf(
		"The liveness watch detected %d self-hosted runner(s) in wedge state: "+
			"the Runner.Listener process is alive in launchd but GitHub API reports the runner as offline. "+
			"CI jobs assigned to this runner sit queued indefinitely with no listener to claim them.\n\n"+
			"Affected runner(s): %s\n\n"+
			"Repair (already applied by conduit if noted there):\n"+
			"  launchctl kickstart -k gui/%d/%s\n\n"+
			"Root cause hypothesis: a failed job leaves the listener unrecoverable "+
			"(observed: both wedges followed a 'result: Failed' log entry with nothing after). "+
			"Confirm: check whether the next 'result: Failed' on either runner is again the last "+
			"line in its log before it re-wedges.\n\n"+
			"Refs: ADR-042, PANTHEON_RULES.md Rule A1 (safety), ADR-040 (do no harm).",
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
