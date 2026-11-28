package ledger

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestFabricLaneReason(t *testing.T) {
	held := "held: attended (no fresh attended session visible in this view; attended claude session t1 on m1: last heartbeat 50h0m0s ago; the session may still be open)"
	cases := []struct {
		name  string
		state string
		r     FabricReach
		rt    *routerstore.LaneOperationalState
		want  string
	}{
		{"undeclared", routerstore.LaneUnroutable, FabricReach{Registry: RegistryUndeclared}, nil, "retired lane (not in registry); open rows left over"},
		{"alias", routerstore.LaneUnroutable, FabricReach{Registry: "alias:codex-apollo"}, nil, "retired; forwards to codex-apollo"},
		{"held attended", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictHeld, Detail: held}, nil, "waiting for an attended session (none seen for 2d2h)"},
		{"held attended no heartbeat", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictHeld, Detail: "held: no_grant (nobody)"}, nil, "waiting for an attended session (no session seen)"},
		{"held other", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictHeld, Detail: "held: window until 10:00:00"}, nil, "held: window until 10:00:00"},
		{"watch only", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictWatchOnly}, nil, "no worker configured; needs a person's session"},
		{"unstaffed", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictUnstaffed}, nil, "no worker path at all; needs a person's session"},
		{"auth", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictAuthRequired}, nil, "the lane's consumer cannot log in; needs a person to sign in"},
		{"unreachable", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictUnreachable}, nil, "its worker stopped reporting"},
		{"quota", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: "PROVIDER_QUOTA"}, nil, "provider quota reached; waiting for it to clear"},
		{"wakeable with failures", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictWakeable}, &routerstore.LaneOperationalState{UnresolvedWakeFailures: 3}, "3 wake failures since the lane last acknowledged work"},
		{"live with failures", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictLive}, &routerstore.LaneOperationalState{UnresolvedWakeFailures: 1}, "1 wake failures since the lane last acknowledged work"},
		{"wakeable no failures", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictWakeable}, nil, "no automated wake path reaches this lane"},
		{"service no verdict", routerstore.LaneUnroutable, FabricReach{Registry: RegistryDeclared}, nil, "no reachability verdict available"},
		// Negative controls: the reason never appears for a lane that is not UNROUTABLE.
		{"working", routerstore.LaneWorking, FabricReach{Registry: RegistryUndeclared}, nil, ""},
		{"idle", routerstore.LaneIdleWithWork, FabricReach{Registry: RegistryDeclared, Verdict: router.VerdictHeld, Detail: held}, nil, ""},
	}
	for _, c := range cases {
		if got := FabricLaneReason(c.state, c.r, c.rt); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestFabricReachForRegistryPresence(t *testing.T) {
	reg := &router.Registry{Agents: map[string]router.AgentConfig{"live": {}}}
	reach := map[string]router.PingResult{"live": {Verdict: router.VerdictLive, Detail: "d"}}
	aliases := map[string]string{"old": "live"}
	if r := fabricReachFor("live", reg, reach, aliases); r.Registry != RegistryDeclared || r.Verdict != router.VerdictLive || r.Detail != "d" {
		t.Fatalf("declared: %+v", r)
	}
	if r := fabricReachFor("old", reg, reach, aliases); r.Registry != "alias:live" || r.Verdict != "" {
		t.Fatalf("alias: %+v", r)
	}
	if r := fabricReachFor("gone", reg, reach, aliases); r.Registry != RegistryUndeclared {
		t.Fatalf("undeclared: %+v", r)
	}
}

// The unroutable flag and the reason must come from the same PingLane result.
func TestFabricFromAttachesReasonFromReach(t *testing.T) {
	snap := Snapshot{GeneratedAt: "2026-11-28T00:00:00Z", Agents: []Agent{{AgentID: "gone"}, {AgentID: "ok"}}}
	ev := FabricEvidence{
		Operational: map[string]routerstore.LaneOperationalState{
			"gone": {Agent: "gone", Classification: routerstore.LaneUnroutable},
			"ok":   {Agent: "ok", Classification: routerstore.LaneComplete},
		},
		Reach: map[string]FabricReach{"gone": {Registry: RegistryUndeclared}, "ok": {Registry: RegistryDeclared, Verdict: router.VerdictLive}},
	}
	b := FabricFrom(snap, nil, "t", ev)
	for _, l := range b.Lanes {
		switch l.Agent {
		case "gone":
			if l.Reason == "" || l.Registry != RegistryUndeclared {
				t.Fatalf("gone lane: %+v", l)
			}
		case "ok":
			if l.Reason != "" || l.ReachVerdict != router.VerdictLive {
				t.Fatalf("ok lane: %+v", l)
			}
		}
	}
}
