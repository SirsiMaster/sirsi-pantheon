package router

import (
	"testing"
	"time"
)

func TestNodeStatusPIDTruthIsHostScoped(t *testing.T) {
	oldState, oldStart, oldCmd := getPIDStateFn(), getPIDStartFn(), getPIDCommandFn()
	defer func() { setPIDStateFn(oldState); setPIDStartFn(oldStart); setPIDCommandFn(oldCmd) }()
	probes := 0
	setPIDStateFn(func(int) PIDState { probes++; return PIDGone })
	setPIDStartFn(func(int) string { return "sig" })
	setPIDCommandFn(func(int) string { return "" })
	now := time.Now()
	local := &Thread{ThreadID: "local", AgentID: "ra", Surface: "worker", PID: 66060, Host: "M1", MachineID: "m1-id", Status: ThreadStatusActive, StartedAt: now, LastSeenAt: now}
	foreign := *local
	foreign.ThreadID = "remote"
	foreign.AgentID = "gemma-pantheon"
	foreign.Host = "Mac"
	foreign.MachineID = "m5-id"
	foreign.CurrentItem = "genuine-original"
	l := nodeThreadSummary(local, now, true, "m1-id", "M1")
	if probes != 1 || l.OSState != PIDGone || l.OSScope != "local" || !l.Stale || l.ArmedReason != "os-gone" {
		t.Fatalf("local dead PID not projected honestly: %+v", l)
	}
	f := nodeThreadSummary(&foreign, now, true, "m1-id", "M1")
	if probes != 1 || f.OSState != PIDUnknown || f.OSScope != "remote-unknown" || f.Stale || f.Status != ThreadStatusActive || f.CurrentItem != "genuine-original" || !f.Armed || f.LoopState == "dead" {
		t.Fatalf("foreign same PID queried locally or lost canonical facts: %+v", f)
	}
	if local.Status != ThreadStatusActive || foreign.Status != ThreadStatusActive {
		t.Fatal("observer mutated canonical lifecycle")
	}
	foreign.Host = "M1"
	f = nodeThreadSummary(&foreign, now, true, "m1-id", "M1")
	if probes != 1 || f.OSState != PIDUnknown {
		t.Fatal("host label overrode mismatching stable machine IDs")
	}
}

func TestNodeStatusForeignLoopAndMissingIdentityStayUnknown(t *testing.T) {
	old := getPIDStateFn()
	defer setPIDStateFn(old)
	setPIDStateFn(func(int) PIDState { t.Fatal("unknown or foreign PID probed locally"); return PIDGone })
	now := time.Now()
	foreign := &Thread{ThreadID: "foreign-loop", AgentID: "claude-pantheon", Surface: "claude", PID: 66060, Host: "Mac", MachineID: "m5", Status: ThreadStatusActive, LastSeenAt: now, StartedAt: now}
	f := nodeThreadSummary(foreign, now, true, "m1", "M1")
	if f.OSState != PIDUnknown || f.LoopState != "unknown" || f.ArmedReason != "remote-loop-unverified" || f.Stale {
		t.Fatalf("foreign loop classified locally: %+v", f)
	}
	foreign.MachineID = ""
	foreign.Host = ""
	f = nodeThreadSummary(foreign, now, true, "m1", "M1")
	if f.OSState != PIDUnknown {
		t.Fatal("missing identity treated as local")
	}
}

func TestNodeStatusObserverIdentityPrefersStableID(t *testing.T) {
	if !nodeObserverOwnsThread(&Thread{MachineID: "same", Host: "old-label"}, "same", "new-label") {
		t.Fatal("stable local identity lost after host rename")
	}
	if nodeObserverOwnsThread(&Thread{MachineID: "remote", Host: "same-label"}, "local", "same-label") {
		t.Fatal("different machine accepted by hostname")
	}
	if !nodeObserverOwnsThread(&Thread{Host: "M1"}, "local", "M1") {
		t.Fatal("explicit legacy observing-host match lost")
	}
	if nodeObserverOwnsThread(&Thread{}, "", "") {
		t.Fatal("missing identity treated as local")
	}
}
