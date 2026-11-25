package routerstore

import (
	"errors"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// Claim is one admission inside the canonical Maat transaction: blocked it keeps
// the row queued, freed it grants exactly once, a repeat returns the same grant,
// and ErrNotQueued survives the HTTP boundary.
func TestMaatClaimThroughTransactionAndWire(t *testing.T) {
	backend, rs := remoteHarness(t)
	sqlite, ok := backend.(*SQLiteStore)
	if !ok {
		t.Skip("transactional check runs on the SQLite backend")
	}
	if r, err := sqlite.MaatReserve(maatRequest(sqlite, "hold-1", "m1", "rep"), false); err != nil || !r.Granted {
		t.Fatalf("first hold: %+v %v", r, err)
	}
	q, err := sqlite.MaatReserve(maatRequest(sqlite, "wait-1", "m1", "waiter"), true)
	if err != nil || !q.Queued {
		t.Fatalf("queue: %+v %v", q, err)
	}
	if got, err := rs.MaatMutate("claim", "wait-1", ""); err != nil || got.Status != schedule.StatusQueued {
		t.Fatalf("blocked claim must stay queued: %+v %v", got, err)
	}
	if _, err := sqlite.MaatMutate("release", "hold-1", ""); err != nil {
		t.Fatal(err)
	}
	got, err := rs.MaatMutate("claim", "wait-1", "")
	if err != nil || got.Status != schedule.StatusActive || got.Share != schedule.ShareFull {
		t.Fatalf("freed claim must grant full: %+v %v", got, err)
	}
	again, err := rs.MaatMutate("claim", "wait-1", "")
	if err != nil || again.Status != schedule.StatusActive || again.Created != got.Created {
		t.Fatalf("repeat claim must return the same grant: %+v %v", again, err)
	}
	live, err := sqlite.MaatRead("live-holders", "m1")
	if err != nil || len(live) != 1 {
		t.Fatalf("exactly one live holder: %+v %v", live, err)
	}
	if _, err := rs.MaatMutate("release", "wait-1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := rs.MaatMutate("claim", "wait-1", ""); !errors.Is(err, schedule.ErrNotQueued) {
		t.Fatalf("a released row is not claimable, over the wire too: %v", err)
	}
}
