package schedule

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// jrow renders one reservation row for a hand-built ledger state.
func jrow(id, holder, status, share, created, behind, start, end string) string {
	return fmt.Sprintf(`{"id":%q,"resource":"m1","holder":%q,"regime":"build","status":%q,"share":%q,"start":%q,"est_end":%q,"created":%q,"heartbeat_at":%q,"lease_ttl_sec":120,"queued_behind":%q}`,
		id, holder, status, share, start, end, created, created, behind)
}

func stateLedger(t *testing.T, now time.Time, rows ...string) *Ledger {
	t.Helper()
	l := NewLedger(newFakeStore()).WithClock(func() time.Time { return now })
	if err := l.store.SetState(stateKey, `{"reservations":[`+strings.Join(rows, ",")+`]}`); err != nil {
		t.Fatal(err)
	}
	return l
}

const (
	pStart = "2026-11-10T10:00:00Z"
	pEnd   = "2026-11-10T11:00:00Z"
)

func pReq(holder string) Reservation {
	return Reservation{Resource: "m1", Holder: holder, Work: holder + "-work", Regime: RegimeBuild, Start: pStart, EstEnd: pEnd, LeaseTTLSec: 120}
}

// W queued behind L; L's hold ends; L queues again behind W; W must still get
// its owed turn (it is the older waiter), then L gets its own after W.
func TestWaiterPrecedenceTwoCycleOwedWaiterProgresses(t *testing.T) {
	f := newWaiterFixture(t, 120) // rep=L holds, wait=W queued behind rep
	f.release(t, f.rep.ID)
	f.now = f.now.Add(10 * time.Second)
	lq, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), true)
	if err != nil || !lq.Queued || lq.Reservation.QueuedBehind != "wait" {
		t.Fatalf("L must queue behind W: %+v %v", lq, err)
	}
	f.now = f.now.Add(10 * time.Second)
	wg, err := f.l.Reserve(f.req("m1", "wait", RegimeBuild), false)
	if err != nil || !wg.Granted || wg.Reservation.Share != ShareFull || wg.Reservation.Status != StatusActive {
		t.Fatalf("owed waiter must be granted FULL: %+v %v", wg, err)
	}
	f.now = f.now.Add(5 * time.Second)
	lq2, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), true)
	if err != nil || lq2.Granted || !lq2.Queued {
		t.Fatalf("L while W holds: want queued: %+v %v", lq2, err)
	}
	f.release(t, wg.Reservation.ID)
	f.now = f.now.Add(5 * time.Second)
	lg, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), false)
	if err != nil || !lg.Granted || lg.Reservation.Share != ShareFull {
		t.Fatalf("after W ran, L must be granted full: %+v %v", lg, err)
	}
}

// A behind B, B behind C, C behind A: only the OLDEST waiter (A) is grantable.
func TestWaiterPrecedenceThreeCycleOldestGrantable(t *testing.T) {
	now := at("2026-11-10T10:00:30Z")
	rows := []string{
		jrow("a", "A", "queued", "", "2026-11-10T10:00:00Z", "B", pStart, pEnd),
		jrow("b", "B", "queued", "", "2026-11-10T10:00:10Z", "C", pStart, pEnd),
		jrow("c", "C", "queued", "", "2026-11-10T10:00:20Z", "A", pStart, pEnd),
	}
	for _, tc := range []struct {
		lane    string
		granted bool
	}{{"A", true}, {"B", false}, {"C", false}} {
		res, err := stateLedger(t, now, rows...).Reserve(pReq(tc.lane), false)
		if tc.granted {
			if err != nil || !res.Granted || res.Reservation.Share != ShareFull {
				t.Fatalf("%s (oldest) must be granted full: %+v %v", tc.lane, res, err)
			}
		} else if !errors.Is(err, ErrWaiterFirst) {
			t.Fatalf("%s must be refused: %+v %v", tc.lane, res, err)
		}
	}
}

// Equal Created is broken by row ID, so a 2-cycle can never deadlock.
func TestWaiterPrecedenceTieBrokenByID(t *testing.T) {
	now := at("2026-11-10T10:00:30Z")
	rows := []string{
		jrow("a1", "A", "queued", "", "2026-11-10T10:00:00Z", "B", pStart, pEnd),
		jrow("b1", "B", "queued", "", "2026-11-10T10:00:00Z", "A", pStart, pEnd),
	}
	if res, err := stateLedger(t, now, rows...).Reserve(pReq("A"), false); err != nil || !res.Granted {
		t.Fatalf("A (smaller id) must be granted: %+v %v", res, err)
	}
	if _, err := stateLedger(t, now, rows...).Reserve(pReq("B"), false); !errors.Is(err, ErrWaiterFirst) {
		t.Fatalf("B must be refused: %v", err)
	}
}

// The requester's own queued row is never a waiter against itself.
func TestWaiterPrecedenceOwnRowNeverCounts(t *testing.T) {
	now := at("2026-11-10T10:00:30Z")
	l := stateLedger(t, now, jrow("a", "A", "queued", "", "2026-11-10T10:00:00Z", "A", pStart, pEnd))
	if res, err := l.Reserve(pReq("A"), false); err != nil || !res.Granted {
		t.Fatalf("own row must not block: %+v %v", res, err)
	}
}

func TestWaiterServedNeedsOverlappingFullGrant(t *testing.T) {
	now := at("2026-11-10T10:01:30Z") // inside the waiter's 120 s lease
	waiter := jrow("w", "W", "queued", "", "2026-11-10T10:00:00Z", "L", pStart, pEnd)
	for _, tc := range []struct {
		name   string
		served string
		want   bool // waiter still owed (L refused)
	}{
		{"non-overlapping full booking", jrow("s", "W", "released", "full", "2026-11-10T10:01:00Z", "", "2026-11-10T12:00:00Z", "2026-11-10T13:00:00Z"), true},
		{"overlapping floor grant", jrow("s", "W", "released", "floor", "2026-11-10T10:01:00Z", "", pStart, pEnd), true},
		{"overlapping full, created before the queue", jrow("s", "W", "released", "full", "2026-11-10T09:59:00Z", "", pStart, pEnd), true},
		{"overlapping full, released", jrow("s", "W", "released", "full", "2026-11-10T10:01:00Z", "", pStart, pEnd), false},
		{"overlapping full, done", jrow("s", "W", "done", "full", "2026-11-10T10:01:00Z", "", pStart, pEnd), false},
		{"overlapping full, invalidated", jrow("s", "W", "invalidated", "full", "2026-11-10T10:01:00Z", "", pStart, pEnd), false},
		{"overlapping full, expired after grant", jrow("s", "W", "expired", "full", "2026-11-10T10:01:00Z", "", pStart, pEnd), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := stateLedger(t, now, waiter, tc.served).Reserve(pReq("L"), false)
			if tc.want != errors.Is(err, ErrWaiterFirst) {
				t.Fatalf("waiter owed=%v, got err %v", tc.want, err)
			}
		})
	}
}

func TestQueuedLeaseCap(t *testing.T) {
	f := newWaiterFixture(t, 120)
	// contention-branch queue with a huge lease
	r := f.req("m1", "other", RegimeBuild)
	r.LeaseTTLSec = 3600
	q, err := f.l.Reserve(r, true)
	if err != nil || !q.Queued || q.Reservation.LeaseTTLSec != maxQueuedLeaseSec || !strings.Contains(q.Reservation.Reason, "capped") {
		t.Fatalf("queued lease must be capped: %+v %v", q.Reservation, err)
	}
	// waiter-first queue branch
	f.release(t, f.rep.ID)
	r = f.req("m1", "rep", RegimeBuild)
	r.LeaseTTLSec = 3600
	q, err = f.l.Reserve(r, true)
	if err != nil || !q.Queued || q.Reservation.LeaseTTLSec != maxQueuedLeaseSec {
		t.Fatalf("waiter-first queued lease must be capped: %+v %v", q.Reservation, err)
	}
	// an active grant is untouched
	g := NewLedger(newFakeStore()).WithClock(func() time.Time { return f.now })
	r = f.req("m2", "x", RegimeBuild)
	r.LeaseTTLSec = 3600
	ar, err := g.Reserve(r, false)
	if err != nil || !ar.Granted || ar.Reservation.LeaseTTLSec != 3600 {
		t.Fatalf("active lease must stay 3600: %+v %v", ar.Reservation, err)
	}
}
