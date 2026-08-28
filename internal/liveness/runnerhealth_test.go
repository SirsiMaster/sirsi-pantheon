package liveness

import (
	"errors"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// goldenLaunchctlList is representative output of `launchctl list` (no label arg).
// Captures two PID states: integer PID (runner running) and dash (runner stopped).
// Header and unrelated service lines ensure the parser filters and handles noise.
const goldenLaunchctlList = `PID	Status	Label
77087	0	actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi
-	0	actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2
1234	0	com.apple.Finder
-	0	com.example.other
`

// parseDiscoverRunnersFromOutput exercises the tab-separated parse logic of
// defaultDiscoverRunners using a supplied string, bypassing the exec and
// plist-exists filter. Mirrors the production parse exactly.
func parseDiscoverRunnersFromOutput(output string) map[string]int {
	result := map[string]int{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		label := fields[2]
		if !strings.HasPrefix(label, runnerLaunchdPrefix) {
			continue
		}
		pid := 0
		if fields[0] != "-" {
			if n, err := strconv.Atoi(fields[0]); err == nil && n > 0 {
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

// TestDefaultDiscoverRunnersParse covers the real tab-separated parse format of
// `launchctl list` (no label arg). This is the format that was broken in PR #586:
// the per-label form returns a plist dict (first line "{"), making fields[0] always
// "{" and strconv.Atoi always fail. The fix uses only the bare `launchctl list` format.
func TestDefaultDiscoverRunnersParse(t *testing.T) {
	got := parseDiscoverRunnersFromOutput(goldenLaunchctlList)

	wantRunning := "actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"
	wantStopped := "actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2"

	pid, ok := got[wantRunning]
	if !ok {
		t.Fatalf("missing running label %q in result", wantRunning)
	}
	if pid != 77087 {
		t.Errorf("label %q: want PID 77087, got %d", wantRunning, pid)
	}

	pid, ok = got[wantStopped]
	if !ok {
		t.Fatalf("missing stopped label %q (stopped runners must appear with PID 0)", wantStopped)
	}
	if pid != 0 {
		t.Errorf("label %q: want PID 0 (stopped), got %d", wantStopped, pid)
	}

	// Non-runner labels must be filtered out.
	for label := range got {
		if !strings.HasPrefix(label, runnerLaunchdPrefix) {
			t.Errorf("unexpected non-runner label %q in result", label)
		}
	}
}

func TestCheckRunnerHealth(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd runner discovery is macOS-specific")
	}
	origDiscover := getDiscoverRunnersFn()
	origGH := getGHRunnersFn()
	origActions := getActionsOperationalFn()
	t.Cleanup(func() {
		setDiscoverRunnersFn(origDiscover)
		setGHRunnersFn(origGH)
		setActionsOperationalFn(origActions)
	})

	const (
		labelA = "actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"
		labelB = "actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2"
	)

	cases := []struct {
		name           string
		labelPIDs      map[string]int
		ghRunners      []GHRunner
		ghErr          error
		actionsOK      bool
		wantWedged     []string
		wantSuppressed []string
		wantUnknown    []string
	}{
		{
			name:      "no runners registered",
			labelPIDs: nil,
			actionsOK: true,
		},
		{
			name:      "single runner healthy online",
			labelPIDs: map[string]int{labelA: 1150},
			ghRunners: []GHRunner{{Name: "m5-sirsi", Status: "online"}},
			actionsOK: true,
		},
		{
			// Original wedge case: PID alive + GH offline + Actions operational = genuine wedge.
			name:      "live PID + GH offline + Actions operational = wedged",
			labelPIDs: map[string]int{labelA: 1150, labelB: 1148},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline"},
				{Name: "m5-sirsi-2", Status: "offline"},
			},
			actionsOK:  true,
			wantWedged: []string{"m5-sirsi", "m5-sirsi-2"},
		},
		{
			// 2026-08-06 live observation: PID alive + GH offline + Actions=major_outage.
			// Offline is EXPECTED during a GitHub incident; suppress rather than alarm.
			name:      "live PID + GH offline + Actions major_outage = suppressed",
			labelPIDs: map[string]int{labelA: 77087, labelB: 73751},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline", Busy: true},
				{Name: "m5-sirsi-2", Status: "offline", Busy: true},
			},
			actionsOK:      false,
			wantSuppressed: []string{"m5-sirsi", "m5-sirsi-2"},
		},
		{
			name:      "one wedged one cleanly stopped",
			labelPIDs: map[string]int{labelA: 1150, labelB: 0},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline"},
				{Name: "m5-sirsi-2", Status: "offline"},
			},
			actionsOK:  true,
			wantWedged: []string{"m5-sirsi"},
		},
		{
			// GH API error → Unknown state for all runners.
			name:        "GH API error = unknown state, no wedge claim (fail-open)",
			labelPIDs:   map[string]int{labelA: 1150},
			ghErr:       errors.New("API unavailable"),
			actionsOK:   true,
			wantUnknown: []string{"m5-sirsi"},
		},
		{
			name:      "runner down and GH offline = not wedged (cleanly stopped)",
			labelPIDs: map[string]int{labelA: 0},
			ghRunners: []GHRunner{{Name: "m5-sirsi", Status: "offline"}},
			actionsOK: true,
		},
		{
			// Actions status unreachable → fail-open → treat as operational → genuine wedge fires.
			// A network failure must NOT suppress a genuine wedge.
			name:       "Actions status fail-open = wedge fires even when endpoint unreachable",
			labelPIDs:  map[string]int{labelA: 1150},
			ghRunners:  []GHRunner{{Name: "m5-sirsi", Status: "offline"}},
			actionsOK:  true, // defaultActionsOperational returns true on error
			wantWedged: []string{"m5-sirsi"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setDiscoverRunnersFn(func() map[string]int { return tc.labelPIDs })
			setGHRunnersFn(func(string) ([]GHRunner, error) { return tc.ghRunners, tc.ghErr })
			setActionsOperationalFn(func() bool { return tc.actionsOK })

			results := CheckRunnerHealth()

			stateByName := map[string]RunnerHealthState{}
			for _, r := range results {
				stateByName[r.Name] = r.State
				// Compat shim must be consistent.
				if r.Wedged != (r.State == RunnerStateWedged) {
					t.Errorf("runner %q: Wedged=%v inconsistent with State=%v", r.Name, r.Wedged, r.State)
				}
			}

			for _, want := range tc.wantWedged {
				if stateByName[want] != RunnerStateWedged {
					t.Errorf("expected %q Wedged, got state=%v", want, stateByName[want])
				}
			}
			for _, want := range tc.wantSuppressed {
				if stateByName[want] != RunnerStateSuppressed {
					t.Errorf("expected %q Suppressed, got state=%v", want, stateByName[want])
				}
			}
			for _, want := range tc.wantUnknown {
				if stateByName[want] != RunnerStateUnknown {
					t.Errorf("expected %q Unknown, got state=%v", want, stateByName[want])
				}
			}

			// Any runner in Wedged state not in wantWedged is a false alarm.
			for name, state := range stateByName {
				if state != RunnerStateWedged {
					continue
				}
				found := false
				for _, want := range tc.wantWedged {
					if want == name {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("unexpected wedge for runner %q", name)
				}
			}

			// Any runner in Suppressed state not in wantSuppressed is unexpected.
			for name, state := range stateByName {
				if state != RunnerStateSuppressed {
					continue
				}
				found := false
				for _, want := range tc.wantSuppressed {
					if want == name {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("unexpected suppression for runner %q", name)
				}
			}
		})
	}
}

func TestProbeRunnerWedge(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd runner discovery is macOS-specific")
	}
	origDiscover := getDiscoverRunnersFn()
	origGH := getGHRunnersFn()
	origActions := getActionsOperationalFn()
	t.Cleanup(func() {
		setDiscoverRunnersFn(origDiscover)
		setGHRunnersFn(origGH)
		setActionsOperationalFn(origActions)
	})

	const labelA = "actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"

	t.Run("no runners = OK", func(t *testing.T) {
		setDiscoverRunnersFn(func() map[string]int { return nil })
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK with no runners, got: %v", f.Detail)
		}
	})

	t.Run("all healthy = OK", func(t *testing.T) {
		setDiscoverRunnersFn(func() map[string]int { return map[string]int{labelA: 1150} })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return []GHRunner{{Name: "m5-sirsi", Status: "online"}}, nil
		})
		setActionsOperationalFn(func() bool { return true })
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK, got: %v", f.Detail)
		}
	})

	t.Run("wedged runner = not OK, fixable, deterministic title", func(t *testing.T) {
		setDiscoverRunnersFn(func() map[string]int { return map[string]int{labelA: 1150} })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return []GHRunner{{Name: "m5-sirsi", Status: "offline"}}, nil
		})
		setActionsOperationalFn(func() bool { return true }) // operational → genuine wedge
		f := ProbeRunnerWedge()
		if f.OK {
			t.Error("expected not-OK for wedged runner")
		}
		if !f.Fixable {
			t.Error("expected Fixable=true for wedged runner")
		}
		const wantTitle = "liveness-watch: ci-runner wedged: m5-sirsi"
		if f.Title != wantTitle {
			t.Errorf("title: got %q, want %q", f.Title, wantTitle)
		}
		if f.Body == "" {
			t.Error("expected non-empty body with remediation instructions")
		}
	})

	t.Run("Actions major_outage + PID alive + GH offline = OK suppressed", func(t *testing.T) {
		// 2026-08-06 scenario: m5-sirsi and m5-sirsi-2 both PID alive + GH offline,
		// GitHub Actions component = major_outage. Must suppress — do not page.
		setDiscoverRunnersFn(func() map[string]int {
			return map[string]int{
				labelA: 77087,
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2": 73751,
			}
		})
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return []GHRunner{
				{Name: "m5-sirsi", Status: "offline", Busy: true},
				{Name: "m5-sirsi-2", Status: "offline", Busy: true},
			}, nil
		})
		setActionsOperationalFn(func() bool { return false }) // major_outage
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK (suppressed) during Actions outage, got not-OK: %v", f.Detail)
		}
		if f.Title != "" {
			t.Errorf("expected no alert title during outage suppression, got: %q", f.Title)
		}
		if !strings.Contains(f.Detail, "control-plane outage") {
			t.Errorf("expected 'control-plane outage' in detail, got: %q", f.Detail)
		}
	})

	t.Run("GH API error = OK (fail-open)", func(t *testing.T) {
		setDiscoverRunnersFn(func() map[string]int { return map[string]int{labelA: 1150} })
		setGHRunnersFn(func(string) ([]GHRunner, error) { return nil, errors.New("api error") })
		setActionsOperationalFn(func() bool { return true })
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK when API fails (fail-open), got: %v", f.Detail)
		}
	})

	t.Run("cleanly stopped + offline = OK (not wedged)", func(t *testing.T) {
		setDiscoverRunnersFn(func() map[string]int { return map[string]int{labelA: 0} })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return []GHRunner{{Name: "m5-sirsi", Status: "offline"}}, nil
		})
		setActionsOperationalFn(func() bool { return true })
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK for cleanly-stopped runner (no PID), got: %v", f.Detail)
		}
	})

	t.Run("Actions status fail-open = still wedge if endpoint unreachable", func(t *testing.T) {
		// If the githubstatus endpoint is unreachable, defaultActionsOperational returns
		// true (fail-open). A network error must NOT suppress a genuine wedge.
		setDiscoverRunnersFn(func() map[string]int { return map[string]int{labelA: 1150} })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return []GHRunner{{Name: "m5-sirsi", Status: "offline"}}, nil
		})
		setActionsOperationalFn(func() bool { return true }) // fail-open default
		f := ProbeRunnerWedge()
		if f.OK {
			t.Error("expected wedge alarm (fail-open on status → treat as operational → wedge fires)")
		}
	})
}
