package routerstore

import (
	"errors"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// Multi reserve through the canonical transaction and over HTTP: all granted in
// one call, a waiter-first refusal on any resource refuses the whole request with
// its typed detail and writes nothing, and an older service (no such method)
// maps to ErrMultiUnsupported.
func TestMaatReserveMultiThroughTransactionAndWire(t *testing.T) {
	backend, rs := remoteHarness(t)
	sqlite, ok := backend.(*SQLiteStore)
	if !ok {
		t.Skip("transactional check runs on the SQLite backend")
	}
	req := func(id, resource, holder string) schedule.Reservation {
		r := maatRequest(sqlite, id, resource, holder)
		r.ID = ""
		return r
	}
	got, err := rs.MaatReserveMulti([]schedule.Reservation{req("", "m1", "a"), req("", "m2", "a")})
	if err != nil || !got.Granted || len(got.Reservations) != 2 || got.Reservations[0].Share != schedule.ShareFull {
		t.Fatalf("grant over the wire: %+v %v", got, err)
	}

	// m3 has a waiter queued behind "rep"; a multi request by rep naming m3 and m4 is refused whole.
	if r, err := sqlite.MaatReserve(maatRequest(sqlite, "hold-3", "m3", "rep"), false); err != nil || !r.Granted {
		t.Fatal(r, err)
	}
	if q, err := sqlite.MaatReserve(maatRequest(sqlite, "wait-3", "m3", "waiter"), true); err != nil || !q.Queued {
		t.Fatal(q, err)
	}
	if _, err := sqlite.MaatMutate("release", "hold-3", ""); err != nil {
		t.Fatal(err)
	}
	before, _ := sqlite.MaatRead("status", "")
	_, err = rs.MaatReserveMulti([]schedule.Reservation{req("", "m4", "rep"), req("", "m3", "rep")})
	var wf *schedule.WaiterFirstError
	if !errors.Is(err, schedule.ErrWaiterFirst) || !errors.As(err, &wf) || wf.Resource != "m3" || wf.Waiter.Holder != "waiter" {
		t.Fatalf("want a typed waiter-first refusal naming m3: %v", err)
	}
	if after, _ := sqlite.MaatRead("status", ""); len(after) != len(before) {
		t.Fatalf("a refused request must write nothing (m4 held?): %d -> %d rows", len(before), len(after))
	}

	// An older service has no such method.
	notServed["MaatReserveMulti"] = true
	t.Cleanup(func() { delete(notServed, "MaatReserveMulti") })
	if _, err := rs.MaatReserveMulti([]schedule.Reservation{req("", "m5", "a")}); !errors.Is(err, schedule.ErrMultiUnsupported) {
		t.Fatalf("old service must map to ErrMultiUnsupported: %v", err)
	}
}
