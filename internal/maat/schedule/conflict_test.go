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

func TestCheckConflicts_ExemptPIDCoversItsOwnDescendants(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	req := mkReq("m1", "claude-io", "2026-09-24T09:00:00Z", "2026-09-24T11:00:00Z", RegimeQuiet)
	req.ExemptPID = 100 // the reservation's own guarded-run process (e.g. maat-run-guard)
	if _, err := l.Reserve(req, false); err != nil {
		t.Fatal(err)
	}
	// 100 (exempt root) -> 200 (some wrapper) -> 300 (the ssh launcher that
	// carries a remote bench tool's name in its argv, same shape as the
	// 2026-09-28 hermes false positive) plus an unrelated PID 999 that must
	// still be reported.
	SetProcessAncestryFn(func() (map[int]int, error) {
		return map[int]int{200: 100, 300: 200, 999: 1}, nil
	})
	defer SetProcessAncestryFn(nil)
	SetActivityProbe(func(machine string) ([]Actor, error) {
		return []Actor{
			{Kind: "bench", Detail: "ssh ... tbraw-bench ...", PID: 300},
			{Kind: "bench", Detail: "iperf", PID: 999},
		}, nil
	})
	defer SetActivityProbe(probeProcesses)

	rep, err := l.CheckConflicts("m1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean {
		t.Fatal("PID 999 is a genuine intruder — the report must not read clean")
	}
	if len(rep.Intruders) != 1 || rep.Intruders[0].PID != 999 {
		t.Fatalf("want only PID 999 reported (PID 300 is a descendant of ExemptPID 100), got %+v", rep.Intruders)
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
