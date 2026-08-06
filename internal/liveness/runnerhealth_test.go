package liveness

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckRunnerHealth(t *testing.T) {
	// Restore injectable functions after each test.
	origDiscover := getDiscoverRunnerLabelsFn()
	origPID := getLaunchctlRunnerPIDFn()
	origGH := getGHRunnersFn()
	origActions := getActionsOperationalFn()
	t.Cleanup(func() {
		setDiscoverRunnerLabelsFn(origDiscover)
		setLaunchctlRunnerPIDFn(origPID)
		setGHRunnersFn(origGH)
		setActionsOperationalFn(origActions)
	})

	cases := []struct {
		name           string
		labels         []string
		pids           map[string]int // label → PID (0 = not running)
		ghRunners      []GHRunner
		ghErr          error
		actionsOK      bool     // true = operational, false = degraded/outage
		wantWedged     []string // runner names expected to be Wedged
		wantSuppressed []string // runner names expected to be Suppressed
		wantHealthy    []string // runner names expected to be healthy
	}{
		{
			name:      "no runners registered",
			labels:    nil,
			actionsOK: true,
		},
		{
			name:   "single runner healthy online",
			labels: []string{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"},
			pids:   map[string]int{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi": 1150},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "online", Busy: false},
			},
			actionsOK:   true,
			wantHealthy: []string{"m5-sirsi"},
		},
		{
			// Original wedge case: PID alive + GH offline + Actions operational = genuine wedge.
			name: "runner with live PID + GH offline + Actions operational = wedged",
			labels: []string{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi",
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2",
			},
			pids: map[string]int{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi":   1150,
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2": 1148,
			},
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
			name: "runner PID alive + GH offline + Actions major_outage = suppressed",
			labels: []string{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi",
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2",
			},
			pids: map[string]int{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi":   77087,
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2": 73751,
			},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline", Busy: true},
				{Name: "m5-sirsi-2", Status: "offline", Busy: true},
			},
			actionsOK:      false, // major_outage
			wantSuppressed: []string{"m5-sirsi", "m5-sirsi-2"},
		},
		{
			name: "one wedged one healthy",
			labels: []string{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi",
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2",
			},
			pids: map[string]int{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi":   1150, // alive
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2": 0,    // not running
			},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline"}, // wedged
				{Name: "m5-sirsi-2", Status: "offline"},
			},
			actionsOK:   true,
			wantWedged:  []string{"m5-sirsi"},
			wantHealthy: []string{},
		},
		{
			// GH API error → unknown state; actionsOperational is never called.
			name: "GH API error means unknown state, no wedge claim",
			labels: []string{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi",
			},
			pids:           map[string]int{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi": 1150},
			ghErr:          errors.New("API unavailable"),
			actionsOK:      true,
			wantWedged:     nil,
			wantSuppressed: nil,
		},
		{
			name: "runner down and GH offline — not wedged (expected state)",
			labels: []string{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi",
			},
			pids: map[string]int{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi": 0},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline"},
			},
			actionsOK:  true,
			wantWedged: nil, // no PID = cleanly stopped, not wedged
		},
		{
			// Actions status endpoint unreachable → fail-open → treat as operational.
			// We simulate with actionsOK=true (what defaultActionsOperational returns on error).
			// A network failure must NOT suppress a genuine wedge.
			name: "Actions status endpoint unreachable = fail-open = wedge fires",
			labels: []string{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi",
			},
			pids: map[string]int{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi": 1150},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline"},
			},
			actionsOK:  true, // fail-open default
			wantWedged: []string{"m5-sirsi"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setDiscoverRunnerLabelsFn(func() []string { return tc.labels })
			setLaunchctlRunnerPIDFn(func(label string) int { return tc.pids[label] })
			setGHRunnersFn(func(repo string) ([]GHRunner, error) {
				return tc.ghRunners, tc.ghErr
			})
			setActionsOperationalFn(func() bool { return tc.actionsOK })

			results := CheckRunnerHealth()

			stateMap := map[string]RunnerHealthState{}
			for _, r := range results {
				stateMap[r.Name] = r.State
			}

			for _, want := range tc.wantWedged {
				if stateMap[want] != RunnerStateWedged {
					t.Errorf("expected %q to be Wedged, got state=%v (results=%+v)", want, stateMap[want], results)
				}
			}
			for _, want := range tc.wantSuppressed {
				if stateMap[want] != RunnerStateSuppressed {
					t.Errorf("expected %q to be Suppressed, got state=%v (results=%+v)", want, stateMap[want], results)
				}
			}

			// Any runner in Wedged state that isn't in wantWedged is a false alarm.
			for name, state := range stateMap {
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

			// Any runner in Suppressed state that isn't in wantSuppressed is unexpected.
			for name, state := range stateMap {
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
	origDiscover := getDiscoverRunnerLabelsFn()
	origPID := getLaunchctlRunnerPIDFn()
	origGH := getGHRunnersFn()
	origActions := getActionsOperationalFn()
	t.Cleanup(func() {
		setDiscoverRunnerLabelsFn(origDiscover)
		setLaunchctlRunnerPIDFn(origPID)
		setGHRunnersFn(origGH)
		setActionsOperationalFn(origActions)
	})

	t.Run("no runners = OK", func(t *testing.T) {
		setDiscoverRunnerLabelsFn(func() []string { return nil })
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK with no runners, got: %v", f.Detail)
		}
	})

	t.Run("all healthy = OK", func(t *testing.T) {
		setDiscoverRunnerLabelsFn(func() []string {
			return []string{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"}
		})
		setLaunchctlRunnerPIDFn(func(string) int { return 1150 })
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
		setDiscoverRunnerLabelsFn(func() []string {
			return []string{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"}
		})
		setLaunchctlRunnerPIDFn(func(string) int { return 1150 })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return []GHRunner{{Name: "m5-sirsi", Status: "offline"}}, nil
		})
		setActionsOperationalFn(func() bool { return true }) // Actions operational → genuine wedge
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
		// 2026-08-06 17:28Z scenario: m5-sirsi and m5-sirsi-2 both PID alive +
		// GH offline, GitHub Actions component = major_outage.
		// Must suppress — do not page during a GitHub incident.
		setDiscoverRunnerLabelsFn(func() []string {
			return []string{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi",
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2",
			}
		})
		setLaunchctlRunnerPIDFn(func(label string) int {
			pids := map[string]int{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi":   77087,
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2": 73751,
			}
			return pids[label]
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
		setDiscoverRunnerLabelsFn(func() []string {
			return []string{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"}
		})
		setLaunchctlRunnerPIDFn(func(string) int { return 1150 })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return nil, errors.New("api error")
		})
		setActionsOperationalFn(func() bool { return true })
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK when API fails (fail-open), got: %v", f.Detail)
		}
	})

	t.Run("check is launchd-disconnected from CI — down+offline not wedged", func(t *testing.T) {
		setDiscoverRunnerLabelsFn(func() []string {
			return []string{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"}
		})
		setLaunchctlRunnerPIDFn(func(string) int { return 0 }) // not running
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
		// If the githubstatus endpoint is unreachable, defaultActionsOperational
		// returns true (fail-open). Simulate by injecting true.
		// A network error must NOT suppress a genuine wedge.
		setDiscoverRunnerLabelsFn(func() []string {
			return []string{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"}
		})
		setLaunchctlRunnerPIDFn(func(string) int { return 1150 })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return []GHRunner{{Name: "m5-sirsi", Status: "offline"}}, nil
		})
		setActionsOperationalFn(func() bool { return true }) // fail-open default
		f := ProbeRunnerWedge()
		if f.OK {
			t.Error("expected wedge alarm (fail-open on status → treat as operational)")
		}
	})
}
