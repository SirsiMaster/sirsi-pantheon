package scales

import (
	"errors"
	"strings"
	"testing"
)

const hwPortsOut = `Hardware Port: Wi-Fi
Device: en0
Ethernet Address: aa:bb

Hardware Port: Thunderbolt Bridge
Device: bridge0
Ethernet Address: cc:dd

Hardware Port: Thunderbolt 1
Device: en1
Ethernet Address: 11:11

Hardware Port: Thunderbolt 2
Device: en2
Ethernet Address: 22:22
`

func TestParseHardwarePortsSkipsTheBridgeService(t *testing.T) {
	got := parseHardwarePorts(hwPortsOut)
	if len(got) != 2 || got["Thunderbolt 1"] != "en1" || got["Thunderbolt 2"] != "en2" {
		t.Fatalf("got %v", got)
	}
}

func TestParseIfconfigAndBridge(t *testing.T) {
	mtu, active := parseIfconfig("en2: flags=8863<UP,BROADCAST> mtu 1500\n\tether 22:22\n\tstatus: active\n")
	if mtu != 1500 || !active {
		t.Fatalf("mtu=%d active=%v", mtu, active)
	}
	mtu, active = parseIfconfig("en3: flags=8863<UP> mtu 65518\n\tstatus: inactive\n")
	if mtu != 65518 || active {
		t.Fatalf("mtu=%d active=%v", mtu, active)
	}
	m := parseBridgeMembers("bridge0: flags=8863<UP> mtu 65518\n\tmember: en6 flags=3<LEARNING,DISCOVER>\n\tmember: en2 flags=3<LEARNING,DISCOVER>\n")
	if !m["en6"] || !m["en2"] || m["en1"] {
		t.Fatalf("members %v", m)
	}
}

func TestTBLaneDriftedOnlyCountsActiveLinks(t *testing.T) {
	cases := []struct {
		l    TBLane
		want bool
	}{
		{TBLane{MTU: 65518, Active: true}, false},
		{TBLane{MTU: 1500, Active: true}, true},
		{TBLane{MTU: 65518, Active: true, InBridge: true}, true},
		{TBLane{MTU: 1500, Active: false}, false}, // unplugged is idle, not drift
	}
	for _, c := range cases {
		if got := c.l.Drifted(); got != c.want {
			t.Fatalf("%+v drifted=%v want %v", c.l, got, c.want)
		}
	}
}

// The M5 as found on 2026-09-12: all three lanes active, MTU 1500, in bridge0.
func m5Drifted() []TBLane {
	return []TBLane{
		{Iface: "en6", Port: "Thunderbolt 2", MTU: 1500, Active: true, InBridge: true},
		{Iface: "en2", Port: "Thunderbolt 3", MTU: 1500, Active: true, InBridge: true},
		{Iface: "en1", Port: "Thunderbolt 1", MTU: 1500, Active: true, InBridge: true},
		{Iface: "en4", Port: "Thunderbolt 4", MTU: 1500, Active: false},
	}
}

func TestCollectTBLaneDriftCountsTheM5Drift(t *testing.T) {
	SetTBLaneProviders(func() ([]TBLane, error) { return m5Drifted(), nil }, nil)
	t.Cleanup(func() { SetTBLaneProviders(listTBLanes, runIfconfig) })
	d, lanes, err := CollectTBLaneDrift()
	if err != nil || d != 3 || len(lanes) != 4 {
		t.Fatalf("drifted=%d lanes=%d err=%v", d, len(lanes), err)
	}
}

func TestFixTBLanesDryRunTouchesNothing(t *testing.T) {
	var calls []string
	SetTBLaneProviders(func() ([]TBLane, error) { return m5Drifted(), nil },
		func(args ...string) error { calls = append(calls, strings.Join(args, " ")); return nil })
	t.Cleanup(func() { SetTBLaneProviders(listTBLanes, runIfconfig) })
	fixed, remaining, err := FixTBLanes(true)
	if err != nil || fixed != 3 || remaining != nil || len(calls) != 0 {
		t.Fatalf("fixed=%d remaining=%v calls=%v err=%v", fixed, remaining, calls, err)
	}
}

func TestFixTBLanesHealsAndReListsClean(t *testing.T) {
	var calls []string
	n := 0
	SetTBLaneProviders(func() ([]TBLane, error) {
		n++
		if n == 1 {
			return m5Drifted(), nil
		}
		healed := m5Drifted()
		for i := range healed {
			healed[i].MTU, healed[i].InBridge = TBLaneMTU, false
		}
		return healed, nil
	}, func(args ...string) error { calls = append(calls, strings.Join(args, " ")); return nil })
	t.Cleanup(func() { SetTBLaneProviders(listTBLanes, runIfconfig) })
	fixed, remaining, err := FixTBLanes(false)
	if err != nil || fixed != 3 || len(remaining) != 0 {
		t.Fatalf("fixed=%d remaining=%v err=%v", fixed, remaining, err)
	}
	// deletem + mtu for each of the three active drifted lanes; the inactive one untouched.
	want := []string{"bridge0 deletem en6", "en6 mtu 65518", "bridge0 deletem en2", "en2 mtu 65518", "bridge0 deletem en1", "en1 mtu 65518"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls %v", calls)
	}
}

func TestFixTBLanesSurfacesARunnerError(t *testing.T) {
	SetTBLaneProviders(func() ([]TBLane, error) { return m5Drifted(), nil },
		func(args ...string) error { return errors.New("sudo: a password is required") })
	t.Cleanup(func() { SetTBLaneProviders(listTBLanes, runIfconfig) })
	if _, _, err := FixTBLanes(false); err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("want the runner error surfaced, got %v", err)
	}
}

func TestEvaluateRuleTBLaneDrift(t *testing.T) {
	rule := DefaultPolicy().Policies[0].Rules[0]
	if rule.Metric != "tb_lane_drift" {
		t.Fatalf("default policy rule 0 is %s", rule.Metric)
	}
	if v := evaluateRule(rule, &ScanMetrics{TBLaneDrift: 3}); v.Passed || v.Severity != SeverityFail || v.ActualValue != 3 {
		t.Fatalf("drifted: %+v", v)
	}
	if v := evaluateRule(rule, &ScanMetrics{TBLaneDrift: 0}); !v.Passed {
		t.Fatalf("clean: %+v", v)
	}
}
