package schedule

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type interleavedStore struct {
	*fakeStore
	beforeCAS func()
	afterRead func()
	casError  error
	reject    bool
	calls     int
}

func (s *interleavedStore) GetState(key string) (string, bool, error) {
	raw, exists, err := s.fakeStore.GetState(key)
	if key == stateKey && s.afterRead != nil {
		fn := s.afterRead
		s.afterRead = nil
		fn()
	}
	return raw, exists, err
}
func (s *interleavedStore) CompareAndSwapState(key, expected string, exists bool, value string) (bool, error) {
	s.calls++
	if s.beforeCAS != nil {
		fn := s.beforeCAS
		s.beforeCAS = nil
		fn()
	}
	if s.casError != nil {
		return false, s.casError
	}
	if s.reject {
		return false, nil
	}
	return s.fakeStore.CompareAndSwapState(key, expected, exists, value)
}
func atomicFixture() (*interleavedStore, *Ledger, *Ledger) {
	s := &interleavedStore{fakeStore: newFakeStore()}
	now := at("2026-10-18T10:00:00Z")
	return s, NewLedger(s).WithClock(func() time.Time { return now }), NewLedger(s.fakeStore).WithClock(func() time.Time { return now })
}
func atomicReq(holder string) Reservation {
	return mkReq("m5", holder, "2026-10-18T09:00:00Z", "2026-10-18T12:00:00Z", RegimeBuild)
}
func TestAtomicReserveRederivesConflict(t *testing.T) {
	for _, queue := range []bool{false, true} {
		t.Run(fmt.Sprint(queue), func(t *testing.T) {
			s, a, b := atomicFixture()
			s.beforeCAS = func() {
				if _, err := b.Reserve(atomicReq("first"), false); err != nil {
					t.Fatal(err)
				}
			}
			got, err := a.Reserve(atomicReq("second"), queue)
			if err != nil {
				t.Fatal(err)
			}
			if got.Conflict == nil || got.Conflict.Holder != "first" {
				t.Fatalf("stale admission: %+v", got)
			}
			if queue && !got.Queued || !queue && got.Reservation.Share != ShareFloor {
				t.Fatalf("incorrect rederived grant: %+v", got)
			}
			rows, err := a.Status("")
			if err != nil || len(rows) != 2 {
				t.Fatalf("lost record: %v %+v", err, rows)
			}
		})
	}
}
func TestAtomicMutationsDoNotLoseOtherChanges(t *testing.T) {
	for _, op := range []string{"heartbeat", "release", "extend", "invalidate"} {
		t.Run(op, func(t *testing.T) {
			s, a, b := atomicFixture()
			first, err := a.Reserve(atomicReq("first"), false)
			if err != nil {
				t.Fatal(err)
			}
			s.beforeCAS = func() {
				if _, err := b.Reserve(atomicReq("other"), false); err != nil {
					t.Fatal(err)
				}
			}
			switch op {
			case "heartbeat":
				_, err = a.Heartbeat(first.Reservation.ID)
			case "release":
				_, err = a.Release(first.Reservation.ID)
			case "extend":
				_, err = a.Extend(first.Reservation.ID, "2026-10-18T13:00:00Z")
			case "invalidate":
				_, err = a.Invalidate(first.Reservation.ID, "intruder")
			}
			if err != nil {
				t.Fatal(err)
			}
			rows, err := a.Status("")
			if err != nil || len(rows) != 2 {
				t.Fatalf("lost record: %v %+v", err, rows)
			}
		})
	}
}
func TestAtomicHeartbeatCannotReviveConcurrentRelease(t *testing.T) {
	s, a, b := atomicFixture()
	r, err := a.Reserve(atomicReq("first"), false)
	if err != nil {
		t.Fatal(err)
	}
	s.beforeCAS = func() {
		if _, err := b.Release(r.Reservation.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Heartbeat(r.Reservation.ID); err == nil {
		t.Fatal("heartbeat revived released record")
	}
	rows, err := a.Status("")
	if err != nil || len(rows) != 1 || rows[0].Status != StatusReleased {
		t.Fatalf("release lost: %v %+v", err, rows)
	}
}
func TestAtomicReadExpiryNeverWritesStaleSnapshot(t *testing.T) {
	for _, op := range []string{"status", "who", "live", "coverage"} {
		t.Run(op, func(t *testing.T) {
			s, a, b := atomicFixture()
			expired := atomicReq("expired")
			expired.LeaseTTLSec = 1
			r, err := b.Reserve(expired, false)
			if err != nil {
				t.Fatal(err)
			}
			a.WithClock(func() time.Time { return at("2026-10-18T10:00:02Z") })
			s.afterRead = func() {
				if _, err := b.Heartbeat(r.Reservation.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := b.Reserve(atomicReq("fresh"), false); err != nil {
					t.Fatal(err)
				}
			}
			switch op {
			case "status":
				_, err = a.Status("")
			case "who":
				_, err = a.WhoIsOn("m5")
			case "live":
				_, err = a.LiveHolders("m5")
			case "coverage":
				_, err = a.Coverage("m5", 24)
			}
			if err != nil {
				t.Fatal(err)
			}
			rows, err := b.Status("")
			if err != nil || len(rows) != 2 {
				t.Fatalf("fresh grant vanished: %v %+v", err, rows)
			}
			if s.calls != 0 {
				t.Fatal("expiry read wrote state")
			}
		})
	}
}
func TestAtomicConcurrentLifecycle(t *testing.T) {
	s := newFakeStore()
	now := at("2026-10-18T10:00:00Z")
	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l := NewLedger(s).WithClock(func() time.Time { return now })
			r, err := l.Reserve(atomicReq(fmt.Sprintf("holder-%d", i)), false)
			if err == nil {
				_, err = l.Heartbeat(r.Reservation.ID)
			}
			if err == nil {
				_, err = l.LiveHolders("m5")
			}
			if err == nil {
				_, err = l.Release(r.Reservation.ID)
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := NewLedger(s).WithClock(func() time.Time { return now }).Status("")
	if err != nil || len(rows) != workers {
		t.Fatalf("lost lifecycle records: %v %d", err, len(rows))
	}
	for _, r := range rows {
		if r.Status != StatusReleased {
			t.Fatalf("lost release: %+v", r)
		}
	}
}
func TestAtomicFailuresDoNotRetryUnknownOutcomes(t *testing.T) {
	s, a, _ := atomicFixture()
	s.casError = errors.New("outcome unknown")
	if _, err := a.Reserve(atomicReq("first"), false); !errors.Is(err, s.casError) {
		t.Fatalf("lost transport error: %v", err)
	}
	if s.calls != 1 {
		t.Fatalf("blind transport retry: %d", s.calls)
	}
	s.casError = nil
	s.reject = true
	s.calls = 0
	if _, err := a.Reserve(atomicReq("first"), false); !errors.Is(err, errLedgerConflict) {
		t.Fatalf("missing bounded conflict: %v", err)
	}
	if s.calls != maxLedgerAttempts {
		t.Fatalf("unbounded/wrong attempts: %d", s.calls)
	}
}
func TestAtomicPreservesHistoryAndFutureFields(t *testing.T) {
	s, a, _ := atomicFixture()
	raw := `{"future":"kept","reservations":[{"id":"old","resource":"m5","holder":"old","status":"released","est_end":"2020-01-01T00:00:00Z","future_row":{"x":1}}]}`
	if err := s.fakeStore.SetState(stateKey, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Reserve(atomicReq("new"), false); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.GetState(stateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"future":"kept"`) || !strings.Contains(got, `"future_row":{"x":1}`) || !strings.Contains(got, `"id":"old"`) {
		t.Fatalf("record/fields dropped: %s", got)
	}
}

func TestAtomicSameSecondIDsAndDuplicateRefusal(t *testing.T) {
	_, a, _ := atomicFixture()
	one, err := a.Reserve(atomicReq("same-holder"), false)
	if err != nil {
		t.Fatal(err)
	}
	two, err := a.Reserve(atomicReq("same-holder"), false)
	if err != nil {
		t.Fatal(err)
	}
	if one.Reservation.ID == two.Reservation.ID {
		t.Fatal("same-second reservations share an identity")
	}
	duplicate := atomicReq("same-holder")
	duplicate.ID = one.Reservation.ID
	if _, err := a.Reserve(duplicate, false); err == nil {
		t.Fatal("explicit duplicate accepted")
	}
}

type legacyReservationStore struct{ data map[string]string }

func (s *legacyReservationStore) GetState(key string) (string, bool, error) {
	v, ok := s.data[key]
	return v, ok, nil
}
func (s *legacyReservationStore) SetState(key, value string) error { s.data[key] = value; return nil }
func TestAtomicRefusesLegacyAndCorruptStores(t *testing.T) {
	legacy := &legacyReservationStore{data: map[string]string{}}
	if _, err := NewLedger(legacy).Reserve(atomicReq("holder"), false); err == nil {
		t.Fatal("unfenced fallback accepted")
	}
	if len(legacy.data) != 0 {
		t.Fatal("legacy fallback wrote state")
	}
	for _, raw := range []string{`{`, `null`, `{"reservations":"bad"}`} {
		s, a, _ := atomicFixture()
		if err := s.fakeStore.SetState(stateKey, raw); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Reserve(atomicReq("holder"), false); err == nil {
			t.Fatalf("corrupt snapshot accepted: %s", raw)
		}
		got, _, _ := s.GetState(stateKey)
		if got != raw {
			t.Fatal("corrupt state overwritten")
		}
	}
}
