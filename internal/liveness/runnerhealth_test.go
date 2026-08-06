package liveness

import (
	"errors"
	"testing"
)

func TestCheckRunnerHealth(t *testing.T) {
	// Restore injectable functions after each test.
	origDiscover := getDiscoverRunnerLabelsFn()
	origPID := getLaunchctlRunnerPIDFn()
	origGH := getGHRunnersFn()
	t.Cleanup(func() {
		setDiscoverRunnerLabelsFn(origDiscover)
		setLaunchctlRunnerPIDFn(origPID)
		setGHRunnersFn(origGH)
	})

	cases := []struct {
		name        string
		labels      []string
		pids        map[string]int // label → PID (0 = not running)
		ghRunners   []GHRunner
		ghErr       error
		wantWedged  []string // runner names expected to be Wedged
		wantHealthy []string // runner names expected to be healthy (online, not wedged)
	}{
		{
			name:   "no runners registered",
			labels: nil,
		},
		{
			name:   "single runner healthy online",
			labels: []string{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"},
			pids:   map[string]int{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi": 1150},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "online", Busy: false},
			},
			wantHealthy: []string{"m5-sirsi"},
		},
		{
			name: "runner with live PID + GH offline = wedged",
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
			wantWedged: []string{"m5-sirsi", "m5-sirsi-2"},
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
			wantWedged:  []string{"m5-sirsi"},
			wantHealthy: []string{},
		},
		{
			name: "GH API error means no wedge claim",
			labels: []string{
				"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi",
			},
			pids:       map[string]int{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi": 1150},
			ghErr:      errors.New("API unavailable"),
			wantWedged: nil, // cannot conclude wedged without API confirmation
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
			wantWedged: nil, // no PID means it's cleanly stopped, not wedged
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setDiscoverRunnerLabelsFn(func() []string { return tc.labels })
			setLaunchctlRunnerPIDFn(func(label string) int { return tc.pids[label] })
			setGHRunnersFn(func(repo string) ([]GHRunner, error) {
				return tc.ghRunners, tc.ghErr
			})

			results := CheckRunnerHealth()

			wedgedNames := map[string]bool{}
			for _, r := range results {
				if r.Wedged {
					wedgedNames[r.Name] = true
				}
			}

			for _, want := range tc.wantWedged {
				if !wedgedNames[want] {
					t.Errorf("expected %q to be wedged, got results=%+v", want, results)
				}
			}
			// Any name in results that's wedged but not in wantWedged is a false alarm.
			for name := range wedgedNames {
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
		})
	}
}

func TestProbeRunnerWedge(t *testing.T) {
	origDiscover := getDiscoverRunnerLabelsFn()
	origPID := getLaunchctlRunnerPIDFn()
	origGH := getGHRunnersFn()
	t.Cleanup(func() {
		setDiscoverRunnerLabelsFn(origDiscover)
		setLaunchctlRunnerPIDFn(origPID)
		setGHRunnersFn(origGH)
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

	t.Run("GH API error = OK (fail-open)", func(t *testing.T) {
		setDiscoverRunnerLabelsFn(func() []string {
			return []string{"actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"}
		})
		setLaunchctlRunnerPIDFn(func(string) int { return 1150 })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return nil, errors.New("api error")
		})
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
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK for cleanly-stopped runner (no PID), got: %v", f.Detail)
		}
	})
}
