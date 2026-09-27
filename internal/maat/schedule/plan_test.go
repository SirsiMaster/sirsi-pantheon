package schedule

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

func TestShouldDefer_PressureGatesFirst_NoCoveringReservation(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	SetPressureFn(func() (guard.PressureLevel, string) { return guard.PressureCritical, "kernel-dispatch" })
	defer SetPressureFn(nil)

	defer_, cur, err := l.ShouldDefer("m1")
	if err != nil {
		t.Fatal(err)
	}
	if !defer_ {
		t.Fatal("Critical memory pressure must defer even with no covering reservation (2026-09-26 stall)")
	}
	if cur != nil {
		t.Fatalf("no reservation covers m1, want nil, got %+v", cur)
	}
}

func TestShouldDefer_NormalPressure_FallsBackToRegimeCheck(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	SetPressureFn(func() (guard.PressureLevel, string) { return guard.PressureNormal, "kernel-dispatch" })
	defer SetPressureFn(nil)

	if _, err := l.Reserve(mkReq("m1", "claude-io", "2026-09-24T09:00:00Z", "2026-09-24T11:00:00Z", RegimeQuiet), false); err != nil {
		t.Fatal(err)
	}
	defer_, cur, err := l.ShouldDefer("m1")
	if err != nil {
		t.Fatal(err)
	}
	if !defer_ || cur == nil || cur.Holder != "claude-io" {
		t.Fatalf("normal pressure must fall back to the existing regime check, got defer=%v cur=%+v", defer_, cur)
	}
}

func TestShouldDefer_NormalPressure_NoReservation_NoDefer(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	SetPressureFn(func() (guard.PressureLevel, string) { return guard.PressureNormal, "kernel-dispatch" })
	defer SetPressureFn(nil)

	defer_, cur, err := l.ShouldDefer("m1")
	if err != nil {
		t.Fatal(err)
	}
	if defer_ || cur != nil {
		t.Fatalf("free machine, normal pressure — want no defer, got defer=%v cur=%+v", defer_, cur)
	}
}
