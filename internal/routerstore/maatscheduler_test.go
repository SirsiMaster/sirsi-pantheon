package routerstore

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

func maatRequest(s *SQLiteStore, id, resource, holder string) schedule.Reservation {
	now := s.clock()
	return schedule.Reservation{ID: id, Resource: resource, Holder: holder,
		Work: "atomic-regression", Regime: schedule.RegimeBuild,
		Start:  now.Add(-time.Second).Format(time.RFC3339),
		EstEnd: now.Add(time.Minute).Format(time.RFC3339), LeaseTTLSec: 120}
}

func TestMaatAtomicConcurrentReservations(t *testing.T) {
	s := newTestStore(t)
	const n = 12
	start := make(chan struct{})
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			out, err := schedule.NewLedger(s).Reserve(maatRequest(s, fmt.Sprintf("r-%d", i), "m5", fmt.Sprintf("lane-%d", i)), false)
			if err == nil && (!out.Granted || out.Reservation == nil) {
				err = fmt.Errorf("reservation %d not granted", i)
			}
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := schedule.NewLedger(s).Status("m5")
	if err != nil || len(rows) != n {
		t.Fatalf("lost reservation: rows=%d err=%v", len(rows), err)
	}
	full := 0
	for _, row := range rows {
		if row.Share == schedule.ShareFull {
			full++
		} else if row.Share != schedule.ShareFloor {
			t.Fatalf("bad share: %+v", row)
		}
	}
	if full != 1 {
		t.Fatalf("want one full grant, got %d", full)
	}
}

func TestMaatAtomicHeartbeatReleaseAndRead(t *testing.T) {
	s := newTestStore(t)
	l := schedule.NewLedger(s)
	for _, id := range []string{"live", "release", "expire"} {
		if _, err := l.Reserve(maatRequest(s, id, id, "lane"), false); err != nil {
			t.Fatal(err)
		}
	}
	// Seed an expired heartbeat through the private transaction adapter only.
	if err := s.withTx(func(tx *txHandle) error {
		state := maatTransactionState{tx}
		raw, _, err := state.GetState(maatReservationsKey)
		if err != nil {
			return err
		}
		var doc struct {
			Reservations []schedule.Reservation `json:"reservations"`
		}
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return err
		}
		for i := range doc.Reservations {
			if doc.Reservations[i].ID == "expire" {
				doc.Reservations[i].HeartbeatAt = s.clock().Add(-time.Hour).Format(time.RFC3339)
			}
		}
		b, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		return state.SetState(maatReservationsKey, string(b))
	}); err != nil {
		t.Fatal(err)
	}
	// Concurrent reads must not save an old snapshot over heartbeat/release.
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := l.Status(""); errs <- err }()
	}
	wg.Add(2)
	go func() { defer wg.Done(); _, err := l.Heartbeat("live"); errs <- err }()
	go func() { defer wg.Done(); _, err := l.Release("release"); errs <- err }()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := l.Status("")
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	for _, r := range rows {
		if r.ID == "release" && r.Status != schedule.StatusReleased {
			t.Fatalf("release lost: %+v", r)
		}
		if r.ID == "live" && r.Status != schedule.StatusActive {
			t.Fatalf("live grant lost: %+v", r)
		}
		if r.ID == "expire" && r.Status != schedule.StatusExpired {
			t.Fatalf("expiry sweep lost: %+v", r)
		}
	}
}

func TestMaatAtomicRemoteRoundTripAndScopedDenial(t *testing.T) {
	_, remote := remoteHarness(t)
	now := time.Now().UTC()
	l := schedule.NewLedger(remote)
	req := schedule.Reservation{ID: "remote-atomic", Resource: "fixture", Holder: remote.agent, Work: "fixture", Regime: schedule.RegimeBuild,
		Start: now.Add(-time.Second).Format(time.RFC3339), EstEnd: now.Add(time.Minute).Format(time.RFC3339)}
	out, err := l.Reserve(req, false)
	if err != nil || !out.Granted {
		t.Fatalf("remote grant: %+v %v", out, err)
	}
	if _, err := l.Heartbeat(req.ID); err != nil {
		t.Fatal(err)
	}
	if row, err := l.WhoIsOn(req.Resource); err != nil || row == nil || row.ID != req.ID {
		t.Fatalf("remote read: %+v %v", row, err)
	}
	if _, err := l.Release(req.ID); err != nil {
		t.Fatal(err)
	}
	f := newScopedFixture(t, []string{"cloud-a"})
	scoped := f.client("cloud-a")
	_, err = scoped.MaatReserve(req, false)
	wantLaneScope(t, "new mutation has no implicit cloud permission", err)
	_, err = scoped.MaatMutate("release", req.ID, "")
	wantLaneScope(t, "new release has no implicit cloud permission", err)
}

func TestMaatAtomicRollbackAndLegacyFence(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetState(maatReservationsKey, `{"reservations":[]}`); err == nil || !strings.Contains(err.Error(), "atomic scheduler") {
		t.Fatalf("legacy overwrite allowed: %v", err)
	}
	if _, err := s.MaatReserve(maatRequest(s, "keep", "m5", "lane"), false); err != nil {
		t.Fatal(err)
	}
	before, _, _ := s.GetState(maatReservationsKey)
	if _, err := s.MaatMutate("unknown", "keep", ""); err == nil {
		t.Fatal("unknown action accepted")
	}
	if _, err := s.MaatRead("unknown", "m5"); err == nil {
		t.Fatal("unknown read accepted")
	}
	if _, err := s.MaatReserve(schedule.Reservation{}, false); err == nil {
		t.Fatal("invalid grant accepted")
	}
	after, _, _ := s.GetState(maatReservationsKey)
	if before != after {
		t.Fatal("failed transaction modified ledger")
	}
}

func TestMaatAtomicConcurrentStudioAliases(t *testing.T) {
	s := newTestStore(t)
	const n = 12
	start := make(chan struct{})
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			out, err := schedule.NewLedger(s).Reserve(maatRequest(s, fmt.Sprintf("r-%d", i), []string{"studio", "m5-studio"}[i%2], fmt.Sprintf("lane-%d", i)), false)
			if err == nil && (!out.Granted || out.Reservation == nil) {
				err = fmt.Errorf("reservation %d not granted", i)
			}
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := schedule.NewLedger(s).Status("studio")
	if err != nil || len(rows) != n {
		t.Fatalf("lost reservation: rows=%d err=%v", len(rows), err)
	}
	full := 0
	for _, row := range rows {
		if row.Share == schedule.ShareFull {
			full++
		} else if row.Share != schedule.ShareFloor {
			t.Fatalf("bad share: %+v", row)
		}
	}
	if full != 1 {
		t.Fatalf("want one full grant, got %d", full)
	}
}
