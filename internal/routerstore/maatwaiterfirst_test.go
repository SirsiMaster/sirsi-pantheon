package routerstore

import (
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// A waiting lane goes first, decided inside the canonical Maat transaction,
// and the typed refusal (with its detail) survives the HTTP boundary.
func TestMaatWaiterFirstThroughTransactionAndWire(t *testing.T) {
	backend, rs := remoteHarness(t)
	sqlite, ok := backend.(*SQLiteStore)
	if !ok {
		t.Skip("transactional check runs on the SQLite backend")
	}
	first, err := sqlite.MaatReserve(maatRequest(sqlite, "hold-1", "m1", "rep"), false)
	if err != nil || !first.Granted {
		t.Fatalf("first hold: %+v %v", first, err)
	}
	wreq := maatRequest(sqlite, "wait-1", "m1", "waiter")
	wreq.Work = "cells"
	q, err := sqlite.MaatReserve(wreq, true)
	if err != nil || !q.Queued || q.Reservation.QueuedBehind != "rep" {
		t.Fatalf("waiter queue: %+v %v", q, err)
	}
	if _, err := sqlite.MaatMutate("release", "hold-1", ""); err != nil {
		t.Fatal(err)
	}
	rows := func() int {
		r, err := sqlite.MaatRead("status", "m1")
		if err != nil {
			t.Fatal(err)
		}
		return len(r)
	}
	before := rows()

	check := func(name string, err error) {
		t.Helper()
		var wf *schedule.WaiterFirstError
		if !errors.Is(err, schedule.ErrWaiterFirst) || !errors.As(err, &wf) ||
			wf.ReasonCode != schedule.ReasonWaiterFirst || wf.Waiter.Holder != "waiter" || wf.Waiter.Work != "cells" || wf.Waiter.QueuedAt == "" {
			t.Fatalf("%s: want typed waiter-first refusal with detail, got %v", name, err)
		}
		if strings.Contains(strings.ToLower(err.Error()), "http 5") {
			t.Fatalf("%s: refusal must not look like a transport error: %v", name, err)
		}
	}
	_, err = sqlite.MaatReserve(maatRequest(sqlite, "hold-2", "m1", "rep"), false)
	check("transaction", err)
	// Across HTTP: errors.Is holds and the structured detail is rebuilt.
	_, err = rs.MaatReserve(maatRequest(sqlite, "hold-3", "m1", "rep"), false)
	check("remote", err)
	if rows() != before {
		t.Fatal("a refused request must write nothing")
	}
	// The waiter itself is never blocked.
	got, err := sqlite.MaatReserve(maatRequest(sqlite, "wait-2", "m1", "waiter"), false)
	if err != nil || !got.Granted || got.Reservation.Share != schedule.ShareFull {
		t.Fatalf("waiter must obtain the hold: %+v %v", got, err)
	}
}
