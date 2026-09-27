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
