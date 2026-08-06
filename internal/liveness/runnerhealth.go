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
	Wedged      bool // true = PID alive + GH says offline + not busy
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

// discoverRunnersFn is the injectable launchd discovery + PID function (Rule A16/A21).
// Returns label → PID for every matching runner label found in launchctl list.
// PID is 0 when the service is loaded but has no live process ("-" in launchctl output).
// macOS only; returns nil on other platforms or exec error.
//
// Using bare `launchctl list` (no label arg) instead of per-label calls: the bare
// form outputs a tab-separated table (PID \t LastExitStatus \t Label) where PID is
// already present. The per-label form outputs a plist-style dict whose first line is
// "{", making field[0] unparseable as an integer. One exec, one format, zero ambiguity.
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

// parseLaunchctlList parses the tab-separated output of `launchctl list` (no
// label arg) and returns label → PID for every runner label matching
// runnerLaunchdPrefix. agentDir, when non-empty, is the LaunchAgents directory;
// labels whose .plist does not exist there are skipped (retired services).
// PID 0 means the service is loaded but has no running process ("-" in output).
//
// Extracted from defaultDiscoverRunners so the test can call the real parse
// logic directly — the test used to duplicate this loop, which meant the
// production parser could drift without breaking the test.
func parseLaunchctlList(out, agentDir string) map[string]int {
	result := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
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

// defaultDiscoverRunners calls `launchctl list` and returns label → PID for
// every service label matching runnerLaunchdPrefix whose .plist exists on disk.
// PID 0 means the service is loaded but not running. Returns nil on non-macOS or exec error.
func defaultDiscoverRunners() map[string]int {
	if runtime.GOOS != "darwin" {
		return nil
	}
	// `launchctl list` (no label arg) outputs: PID \t LastExitStatus \t Label
	// "-" in the PID column means the service has no running process.
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return nil
	}
	home, _ := os.UserHomeDir()
	agentDir := ""
	if home != "" {
		agentDir = home + "/Library/LaunchAgents/"
	}
	return parseLaunchctlList(string(out), agentDir)
}

// CheckRunnerHealth returns one RunnerHealth per self-hosted runner found on
// this host. Wedged = launchd PID alive AND GitHub API reports offline AND
// runner is not busy (busy=true during offline is the GitHub outage signature,
// not a wedge). Exported so callers (node-status, doctor) can surface it
// without re-probing.
func CheckRunnerHealth() []RunnerHealth {
	if runtime.GOOS != "darwin" {
		return nil
	}

	labelPIDs := getDiscoverRunnersFn()()
	if len(labelPIDs) == 0 {
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

	var results []RunnerHealth
	for label, pid := range labelPIDs {
		name := strings.TrimPrefix(label, runnerLaunchdPrefix)
		gh := ghByName[name] // zero value if not found
		h := RunnerHealth{
			Name:        name,
			Label:       label,
			ListenerPID: pid,
			GHStatus:    gh.Status,
			GHBusy:      gh.Busy,
			// Wedged: the local process is alive but GitHub has lost the runner,
			// and the runner is NOT busy. Requiring !Busy eliminates the GitHub
			// control-plane outage signature (offline+busy=true), which is
			// indistinguishable from a wedge on PID+status alone but has a
			// completely different remediation path.
			//
			// Only set when we have positive evidence from BOTH sides:
			//   pid > 0 (launchd has a live process)
			//   GH explicitly says "offline" (not "" — that means API error)
			//   GH does NOT say busy (busy=true → outage, not wedge)
			Wedged: pid > 0 && gh.Status == "offline" && !gh.Busy,
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
			details = append(details, fmt.Sprintf("%s: PID %d alive, GH=offline busy=false (WEDGED)", r.Name, r.ListenerPID))
		case r.GHStatus == "offline" && r.GHBusy:
			// offline+busy is the GH outage signature — informational only.
			details = append(details, fmt.Sprintf("%s: PID %d, GH=offline busy=true (outage/transit, not wedged)", r.Name, r.ListenerPID))
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
			"the Runner.Listener process is alive in launchd but GitHub API reports the runner as offline "+
			"and not busy. CI jobs assigned to this runner sit queued indefinitely with no listener to claim them.\n\n"+
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
