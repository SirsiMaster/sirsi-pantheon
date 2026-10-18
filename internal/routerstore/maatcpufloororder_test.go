package routerstore

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"testing"
)

func TestMaatAtomicCPUFloorDoesNotStrandNextModel(t *testing.T) {
	s := newTestStore(t)
	l := schedule.NewLedger(s)
	full, err := l.Reserve(maatRequest(s, "initial-full", "m5", "builder"), false)
	if err != nil || full.Reservation.Share != schedule.ShareFull {
		t.Fatalf("initial full: %+v %v", full, err)
	}
	floor, err := l.Reserve(maatRequest(s, "cpu-floor", "m5", "cpu"), false)
	if err != nil || floor.Reservation.Share != schedule.ShareFloor {
		t.Fatalf("CPU floor: %+v %v", floor, err)
	}
	if _, err := l.Release("initial-full"); err != nil {
		t.Fatal(err)
	}
	model := maatRequest(s, "model-full", "m5", "apollo")
	model.Regime = schedule.RegimeLoaded
	out, err := l.Reserve(model, false)
	if err != nil {
		t.Fatal(err)
	}
	if out.Reservation.Share != schedule.ShareFull {
		t.Fatalf("CPU floor stranded model after full release: %+v", out)
	}
	other := maatRequest(s, "other-model", "m5", "other")
	other.Regime = schedule.RegimeLoaded
	denied, err := l.Reserve(other, false)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Reservation.Share != schedule.ShareFloor || denied.Conflict == nil || denied.Conflict.ID != "model-full" {
		t.Fatalf("foreign model lost exclusion: %+v", denied)
	}
}
