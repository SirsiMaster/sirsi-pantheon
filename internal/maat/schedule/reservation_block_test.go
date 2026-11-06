package schedule

import (
	"testing"
	"time"
)

func blockLedger() (*Ledger, *time.Time) {
	now := at("2026-11-05T10:00:00Z")
	return NewLedger(newFakeStore()).WithClock(func() time.Time { return now }), &now
}

func blockReq(holder string, regime Regime) Reservation {
	r := mkReq("m5", holder, "2026-11-05T10:00:00Z", "2026-11-05T13:00:00Z", regime)
	r.LeaseTTLSec = 7200
	return r
}

func mustReserve(t *testing.T, l *Ledger, r Reservation) ReserveResult {
	t.Helper()
	res, err := l.Reserve(r, false)
	if err != nil || !res.Granted {
		t.Fatalf("reserve %s: %+v, %v", r.Holder, res, err)
	}
	return res
}

func TestBuildHoldStopsBlockingAfter120s(t *testing.T) {
	l, now := blockLedger()
	first := mustReserve(t, l, blockReq("builder", RegimeBuild))
	*now = now.Add(time.Hour) // running alone costs nobody: the clock starts at the first block
	if res := mustReserve(t, l, blockReq("a", RegimeBuild)); res.Reservation.Share != ShareFloor {
		t.Fatalf("first blocked request must get the floor, got %+v", res.Reservation)
	}
	*now = now.Add(119 * time.Second)
	if res := mustReserve(t, l, blockReq("b", RegimeBuild)); res.Reservation.Share != ShareFloor {
		t.Fatalf("119s must still block, got %+v", res.Reservation)
	}
	*now = now.Add(time.Second)
	got := mustReserve(t, l, blockReq("c", RegimeBuild))
	if got.Reservation.Share != ShareFull {
		t.Fatalf("120s must stop blocking, got %+v", got.Reservation)
	}
	_, _, rows, _, err := l.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == first.Reservation.ID {
			if r.Share != ShareFloor || r.Reason == "" || r.Status != StatusActive {
				t.Fatalf("holder must keep running, downgraded and told why: %+v", r)
			}
			return
		}
	}
	t.Fatal("holder row missing")
}

func TestProtectedHoldsNeverYield(t *testing.T) {
	withMem := blockReq("holder", RegimeBuild)
	withMem.MemGB = 64
	cases := map[string]struct {
		holder  Reservation
		request Reservation
	}{
		"quiet holder":    {blockReq("holder", RegimeQuiet), blockReq("x", RegimeBuild)},
		"loaded holder":   {blockReq("holder", RegimeLoaded), blockReq("x", RegimeBuild)},
		"memory holder":   {withMem, blockReq("x", RegimeBuild)},
		"quiet requester": {blockReq("holder", RegimeBuild), blockReq("x", RegimeQuiet)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			l, now := blockLedger()
			mustReserve(t, l, c.holder)
			for i := 0; i < 3; i++ {
				r := c.request
				r.Work = name
				r.ID = ""
				r.Holder = c.request.Holder + string(rune('a'+i))
				if res := mustReserve(t, l, r); res.Reservation.Share != ShareFloor {
					t.Fatalf("step %d: want floor, got %+v", i, res.Reservation)
				}
				*now = now.Add(5 * time.Minute)
			}
		})
	}
}
