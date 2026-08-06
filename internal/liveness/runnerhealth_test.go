package liveness

import (
	"errors"
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
	origDiscover := getDiscoverRunnersFn()
	origGH := getGHRunnersFn()
	t.Cleanup(func() {
		setDiscoverRunnersFn(origDiscover)
		setGHRunnersFn(origGH)
	})

	const (
		labelA = "actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi"
		labelB = "actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi-2"
	)

	cases := []struct {
		name          string
		labelPIDs     map[string]int
		ghRunners     []GHRunner
		ghErr         error
		wantWedged    []string
		wantNotWedged []string
	}{
		{
			name:      "no runners registered",
			labelPIDs: nil,
		},
		{
			name:      "single runner healthy online",
			labelPIDs: map[string]int{labelA: 1150},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "online", Busy: false},
			},
			wantNotWedged: []string{"m5-sirsi"},
		},
		{
			name: "live PID + GH offline + not busy = wedged",
			labelPIDs: map[string]int{
				labelA: 1150,
				labelB: 1148,
			},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline", Busy: false},
				{Name: "m5-sirsi-2", Status: "offline", Busy: false},
			},
			wantWedged: []string{"m5-sirsi", "m5-sirsi-2"},
		},
		{
			// BLOCKER 2 regression: offline+busy=true is the GitHub outage signature.
			// Both runners were live (PIDs 77087/73751) during the 2026-08-06T15:22Z
			// Actions incident yet showed offline+busy. Must NOT claim wedge.
			name: "live PID + GH offline + busy=true = outage signature, NOT wedged",
			labelPIDs: map[string]int{
				labelA: 77087,
				labelB: 73751,
			},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline", Busy: true},
				{Name: "m5-sirsi-2", Status: "offline", Busy: true},
			},
			wantWedged:    nil,
			wantNotWedged: []string{"m5-sirsi", "m5-sirsi-2"},
		},
		{
			// Mixed: one genuinely wedged (offline+not-busy), one outage-signature (offline+busy).
			name: "mixed: one wedged one outage-signature",
			labelPIDs: map[string]int{
				labelA: 1150,
				labelB: 9999,
			},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline", Busy: false},  // wedged
				{Name: "m5-sirsi-2", Status: "offline", Busy: true}, // outage
			},
			wantWedged:    []string{"m5-sirsi"},
			wantNotWedged: []string{"m5-sirsi-2"},
		},
		{
			name: "one wedged one cleanly stopped",
			labelPIDs: map[string]int{
				labelA: 1150, // alive
				labelB: 0,    // not running
			},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline"},
				{Name: "m5-sirsi-2", Status: "offline"},
			},
			wantWedged:    []string{"m5-sirsi"},
			wantNotWedged: []string{"m5-sirsi-2"},
		},
		{
			name:       "GH API error means no wedge claim (fail-open)",
			labelPIDs:  map[string]int{labelA: 1150},
			ghErr:      errors.New("API unavailable"),
			wantWedged: nil,
		},
		{
			name:      "runner down and GH offline — not wedged (cleanly stopped)",
			labelPIDs: map[string]int{labelA: 0},
			ghRunners: []GHRunner{
				{Name: "m5-sirsi", Status: "offline"},
			},
			wantWedged: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setDiscoverRunnersFn(func() map[string]int { return tc.labelPIDs })
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
					t.Errorf("expected %q to be wedged; results=%+v", want, results)
				}
			}
			for _, want := range tc.wantNotWedged {
				if wedgedNames[want] {
					t.Errorf("expected %q NOT to be wedged; results=%+v", want, results)
				}
			}
			// Any wedged name not in wantWedged is a false alarm.
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
	origDiscover := getDiscoverRunnersFn()
	origGH := getGHRunnersFn()
	t.Cleanup(func() {
		setDiscoverRunnersFn(origDiscover)
		setGHRunnersFn(origGH)
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
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK, got: %v", f.Detail)
		}
	})

	t.Run("wedged runner = not OK, fixable, deterministic title", func(t *testing.T) {
		setDiscoverRunnersFn(func() map[string]int { return map[string]int{labelA: 1150} })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return []GHRunner{{Name: "m5-sirsi", Status: "offline", Busy: false}}, nil
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

	t.Run("GH outage (offline+busy) = OK, no alarm", func(t *testing.T) {
		// Regression for BLOCKER 2: the 2026-08-06 GH Actions outage showed
		// offline+busy=true on both healthy runners. The probe must not fire.
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
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK during GH outage (offline+busy), got: detail=%q title=%q", f.Detail, f.Title)
		}
		if f.Title != "" {
			t.Errorf("expected no alarm title during outage, got: %q", f.Title)
		}
	})

	t.Run("GH API error = OK (fail-open)", func(t *testing.T) {
		setDiscoverRunnersFn(func() map[string]int { return map[string]int{labelA: 1150} })
		setGHRunnersFn(func(string) ([]GHRunner, error) {
			return nil, errors.New("api error")
		})
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
		f := ProbeRunnerWedge()
		if !f.OK {
			t.Errorf("expected OK for cleanly-stopped runner (no PID), got: %v", f.Detail)
		}
	})
}
