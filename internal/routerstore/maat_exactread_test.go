package routerstore

import (
	"encoding/json"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"strings"
	"testing"
)

func TestMaatExactReadLargeHistoryRemote(t *testing.T) {
	store, remote := remoteHarness(t)
	s := store.(*SQLiteStore)
	rows := make([]schedule.Reservation, 9000)
	for i := range rows {
		rows[i] = schedule.Reservation{ID: "historical", Resource: "m5", Status: schedule.StatusReleased, Work: strings.Repeat("x", 1024)}
	}
	rows = append(rows, schedule.Reservation{ID: "exact805f", Resource: "m5", Status: schedule.StatusReleased, Work: "retained release"})
	b, _ := json.Marshal(map[string]any{"reservations": rows})
	if len(b) <= 8<<20 {
		t.Fatal("fixture must exceed operator buffer")
	}
	if err := s.withTx(func(tx *txHandle) error { return (maatTransactionState{tx}).SetState(maatReservationsKey, string(b)) }); err != nil {
		t.Fatal(err)
	}
	got, err := schedule.NewLedger(remote).ExactReservation("exact805f")
	if err != nil || len(got) != 1 || got[0].Status != schedule.StatusReleased {
		t.Fatalf("exact remote release: %+v %v", got, err)
	}
	reply, _ := json.Marshal(got)
	if len(reply) > 64<<10 {
		t.Fatal("unbounded wire output")
	}
	missing, err := schedule.NewLedger(remote).ExactReservation("missing")
	if err != nil || len(missing) != 0 {
		t.Fatal("missing must not look released", err)
	}
	for _, id := range []string{"", " exact805f", "historical"} {
		if _, err := schedule.NewLedger(remote).ExactReservation(id); err == nil {
			t.Fatalf("invalid/duplicate accepted %q", id)
		}
	}
	after, _, err := s.GetState(maatReservationsKey)
	if err != nil || after != string(b) {
		t.Fatal("read mutated reservation state", err)
	}
	f := newScopedFixture(t, []string{"cloud-a"})
	_, err = f.client("cloud-a").MaatRead("reservation", "exact805f")
	wantLaneScope(t, "no new scoped read grant", err)
}
