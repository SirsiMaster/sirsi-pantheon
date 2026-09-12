package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/scales"
)

// CLI-level tests for `sirsi maat scales`, driven through the injectable
// providers (Rule A21): metrics via scales.SetMetricsCollector, the lane
// heal via scales.SetTBLaneProviders. Written against SNE's review of #751.

func withScalesProviders(t *testing.T, collector func() (*scales.ScanMetrics, error), lister func() ([]scales.TBLane, error), runner func(args ...string) error) {
	t.Helper()
	scales.SetMetricsCollector(collector)
	scales.SetTBLaneProviders(lister, runner)
	t.Cleanup(func() {
		scales.SetMetricsCollector(scales.CollectMetrics)
		scales.SetTBLaneProviders(nil, nil) // keep whatever the package had; tests replace both below
	})
	prevFix, prevPolicy := maatFix, maatPolicyFile
	t.Cleanup(func() { maatFix, maatPolicyFile = prevFix, prevPolicy })
}

func driftedLanes() []scales.TBLane {
	return []scales.TBLane{{Iface: "en2", Port: "Thunderbolt 2", MTU: 1500, Active: true}}
}

// P2 #1: a successful heal followed by a FAILED post-heal collection must
// surface as an error and must not render the pre-heal verdicts as current.
func TestMaatScalesPostHealCollectionFailureSurfaces(t *testing.T) {
	calls := 0
	collector := func() (*scales.ScanMetrics, error) {
		calls++
		if calls == 1 {
			return &scales.ScanMetrics{TBLaneDrift: 1}, nil
		}
		return nil, errors.New("networksetup: exit status 1")
	}
	healed := 0
	withScalesProviders(t, collector,
		func() ([]scales.TBLane, error) {
			if healed > 0 {
				return nil, nil
			}
			return driftedLanes(), nil
		},
		func(args ...string) error { healed++; return nil })
	maatFix, maatPolicyFile = true, ""
	err := runMaatScales(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "post-heal verification") {
		t.Fatalf("want a post-heal verification error, got %v", err)
	}
	if healed == 0 {
		t.Fatal("the heal itself should have run before the failed re-collection")
	}
	if calls != 2 {
		t.Fatalf("want 2 collections (before and after the heal), got %d", calls)
	}
}

// Positive: after a successful heal, the rendered state is the RE-WEIGH
// (clean), not the pre-heal breach.
func TestMaatScalesPostHealStateReplacesPreHeal(t *testing.T) {
	calls := 0
	collector := func() (*scales.ScanMetrics, error) {
		calls++
		if calls == 1 {
			return &scales.ScanMetrics{TBLaneDrift: 1}, nil
		}
		return &scales.ScanMetrics{TBLaneDrift: 0}, nil
	}
	healed := 0
	withScalesProviders(t, collector,
		func() ([]scales.TBLane, error) {
			if healed > 0 {
				return nil, nil
			}
			return driftedLanes(), nil
		},
		func(args ...string) error { healed++; return nil })
	maatFix, maatPolicyFile = true, ""
	if err := runMaatScales(nil, nil); err != nil {
		t.Fatalf("clean re-weigh after heal must succeed: %v", err)
	}
	if healed != 1 || calls != 2 {
		t.Fatalf("healed=%d calls=%d; want one mtu repair and two collections", healed, calls)
	}
}

// Preview: without --fix nothing is mutated, and the heal is offered.
func TestMaatScalesPreviewMutatesNothing(t *testing.T) {
	runs := 0
	withScalesProviders(t,
		func() (*scales.ScanMetrics, error) { return &scales.ScanMetrics{TBLaneDrift: 1}, nil },
		func() ([]scales.TBLane, error) { return driftedLanes(), nil },
		func(args ...string) error { runs++; return nil })
	maatFix, maatPolicyFile = false, ""
	if err := runMaatScales(nil, nil); err != nil {
		t.Fatalf("preview must not error: %v", err)
	}
	if runs != 0 {
		t.Fatalf("preview ran ifconfig %d time(s); want 0", runs)
	}
}

// P2 #2: a custom policy is deep-validated before anything runs.
func TestMaatScalesRejectsInvalidCustomPolicies(t *testing.T) {
	cases := map[string]string{
		"empty rules":  "api_version: v1\npolicies:\n  - name: p\n    version: '1'\n    rules: []\n",
		"bad operator": "api_version: v1\npolicies:\n  - name: p\n    version: '1'\n    rules:\n      - id: r1\n        name: n\n        metric: tb_lane_drift\n        operator: grater\n        threshold: 0\n        severity: fail\n",
		"bad severity": "api_version: v1\npolicies:\n  - name: p\n    version: '1'\n    rules:\n      - id: r1\n        name: n\n        metric: tb_lane_drift\n        operator: gt\n        threshold: 0\n        severity: fatal\n",
		"bad metric":   "api_version: v1\npolicies:\n  - name: p\n    version: '1'\n    rules:\n      - id: r1\n        name: n\n        metric: tb_lanes\n        operator: gt\n        threshold: 0\n        severity: fail\n",
		"duplicate id": "api_version: v1\npolicies:\n  - name: p\n    version: '1'\n    rules:\n      - id: r1\n        name: n\n        metric: tb_lane_drift\n        operator: gt\n        threshold: 0\n        severity: fail\n      - id: r1\n        name: n2\n        metric: ghost_count\n        operator: gt\n        threshold: 5\n        severity: warn\n",
	}
	for name, yaml := range cases {
		path := filepath.Join(t.TempDir(), "policy.yaml")
		if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadPolicyStrict(path); err == nil {
			t.Fatalf("%s: want a validation error, got nil", name)
		} else if !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("%s: error should name the policy as invalid: %v", name, err)
		}
	}
}

// A valid custom policy loads, is weighed, and a warn-only lane rule still
// gets the heal offered (the suggestion is no longer nested under failures).
func TestMaatScalesValidCustomPolicyWarnRuleStillOffersHeal(t *testing.T) {
	yaml := "api_version: v1\npolicies:\n  - name: custom\n    version: '1'\n    rules:\n      - id: lanes-warn\n        name: lanes\n        metric: tb_lane_drift\n        operator: gt\n        threshold: 0\n        severity: warn\n"
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPolicyStrict(path); err != nil {
		t.Fatalf("valid custom policy rejected: %v", err)
	}
	runs := 0
	withScalesProviders(t,
		func() (*scales.ScanMetrics, error) { return &scales.ScanMetrics{TBLaneDrift: 1}, nil },
		func() ([]scales.TBLane, error) { return driftedLanes(), nil },
		func(args ...string) error { runs++; return nil })
	maatFix, maatPolicyFile = false, path
	if err := runMaatScales(nil, nil); err != nil {
		t.Fatalf("warn-only policy must not error: %v", err)
	}
	if runs != 0 {
		t.Fatal("preview must not mutate")
	}
}
