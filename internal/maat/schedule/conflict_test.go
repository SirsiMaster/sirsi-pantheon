package schedule

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

func TestCheckConflicts_ReportsPressureAlongsideCleanReport(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	if _, err := l.Reserve(mkReq("m1", "claude-io", "2026-09-24T09:00:00Z", "2026-09-24T11:00:00Z", RegimeQuiet), false); err != nil {
		t.Fatal(err)
	}
	SetActivityProbe(func(machine string) ([]Actor, error) { return nil, nil })
	defer SetActivityProbe(probeProcesses)
	SetPressureFn(func() (guard.PressureLevel, string) { return guard.PressureCritical, "kernel-dispatch" })
	defer SetPressureFn(nil)

	rep, err := l.CheckConflicts("m1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean {
		t.Fatal("memory pressure is not an Actor/Intruder — a clean process scan must stay Clean")
	}
	if rep.Pressure != guard.PressureCritical || rep.PressureSource != "kernel-dispatch" {
		t.Fatalf("want Pressure/PressureSource surfaced, got %+v", rep)
	}
	if !strings.Contains(rep.Summary, "memory pressure") {
		t.Fatalf("Warn/Critical pressure must be legible in Summary even with no intruders, got %q", rep.Summary)
	}
}

func TestProbeProcesses_RefusesRemoteMachine(t *testing.T) {
	SetLocalMachineLabelFn(func() (string, error) { return "m1", nil })
	defer SetLocalMachineLabelFn(nil)

	if _, err := probeProcesses("m5"); err == nil {
		t.Fatal("probing a machine that isn't this host must error, never silently scan local ps and mislabel it")
	} else if !strings.Contains(err.Error(), "m1") || !strings.Contains(err.Error(), "m5") {
		t.Fatalf("error should name both this host and the requested machine, got %q", err)
	}
}

func TestProbeProcesses_AllowsLocalMachineCaseInsensitive(t *testing.T) {
	SetLocalMachineLabelFn(func() (string, error) { return "m1", nil })
	defer SetLocalMachineLabelFn(nil)

	if _, err := probeProcesses("M1"); err != nil {
		t.Fatalf("probing this host (case-insensitive) must succeed, got %v", err)
	}
}

func TestCheckConflicts_BuildRegimeSkipsPressureRead(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	if _, err := l.Reserve(mkReq("m1", "claude-io", "2026-09-24T09:00:00Z", "2026-09-24T11:00:00Z", RegimeBuild), false); err != nil {
		t.Fatal(err)
	}
	called := false
	SetPressureFn(func() (guard.PressureLevel, string) { called = true; return guard.PressureCritical, "kernel-dispatch" })
	defer SetPressureFn(nil)

	rep, err := l.CheckConflicts("m1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("build-regime reservations tolerate foreign load and early-return before the pressure read")
	}
	if rep.Pressure != guard.PressureUnknown {
		t.Fatalf("build-regime report must not carry a pressure reading, got %+v", rep)
	}
}
