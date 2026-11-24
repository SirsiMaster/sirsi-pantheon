package schedule

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// waiterFixture: lane "rep" holds m1 (active); lane "wait" has queued behind it.
type waiterFixture struct {
	l   *Ledger
	now time.Time
	rep Reservation // rep's first hold
	w   Reservation // the waiter's queued row
}

func newWaiterFixture(t *testing.T, waiterLease int) *waiterFixture {
	t.Helper()
	f := &waiterFixture{now: at("2026-11-10T10:00:00Z")}
	f.l = NewLedger(newFakeStore()).WithClock(func() time.Time { return f.now })
	first, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), false)
	if err != nil || !first.Granted || first.Reservation.Share != ShareFull {
		t.Fatalf("first hold: %+v %v", first, err)
	}
	f.rep = *first.Reservation
	wr := f.req("m1", "wait", RegimeBuild)
	wr.Work, wr.LeaseTTLSec = "cells", waiterLease
	q, err := f.l.Reserve(wr, true)
	if err != nil || !q.Queued || q.Reservation.QueuedBehind != "rep" {
		t.Fatalf("waiter queue: %+v %v", q, err)
	}
	f.w = *q.Reservation
	return f
}

func (f *waiterFixture) req(resource, holder string, regime Regime) Reservation {
	return Reservation{Resource: resource, Holder: holder, Work: holder + "-work", Regime: regime,
		Start: "2026-11-10T10:00:00Z", EstEnd: "2026-11-10T11:00:00Z", LeaseTTLSec: 120}
}

func (f *waiterFixture) release(t *testing.T, id string) {
	t.Helper()
	if _, err := f.l.Release(id); err != nil {
		t.Fatal(err)
	}
}

func (f *waiterFixture) rows(t *testing.T) int {
	t.Helper()
	r, err := f.l.Status("")
	if err != nil {
		t.Fatal(err)
	}
	return len(r)
}

func TestWaiterFirstRefusesRepeaterInEveryRegime(t *testing.T) {
	for _, regime := range []Regime{RegimeQuiet, RegimeLoaded, RegimeBuild} {
		t.Run(string(regime), func(t *testing.T) {
			f := newWaiterFixture(t, 120)
			f.release(t, f.rep.ID)
			before := f.rows(t)
			res, err := f.l.Reserve(f.req("m1", "rep", regime), false)
			if !errors.Is(err, ErrWaiterFirst) {
				t.Fatalf("want ErrWaiterFirst, got %+v %v", res, err)
			}
			var wf *WaiterFirstError
			if !errors.As(err, &wf) || wf.ReasonCode != ReasonWaiterFirst ||
				wf.Waiter != (WaiterInfo{Holder: "wait", Work: "cells", QueuedAt: f.w.Created}) {
				t.Fatalf("detail: %+v", wf)
			}
			if res.Granted || res.Reservation != nil || res.ReasonCode != ReasonWaiterFirst || res.Waiter == nil || res.Waiter.Holder != "wait" {
				t.Fatalf("result: %+v", res)
			}
			if f.rows(t) != before {
				t.Fatal("a refusal must save nothing")
			}
		})
	}
}

func TestWaiterFirstQueuesRepeaterWhenAskedTo(t *testing.T) {
	f := newWaiterFixture(t, 120)
	f.release(t, f.rep.ID)
	res, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), true)
	if err != nil || res.Granted || !res.Queued || res.Reservation == nil || res.Reservation.Status != StatusQueued {
		t.Fatalf("want queued: %+v %v", res, err)
	}
	if res.ReasonCode != ReasonWaiterFirst || res.Waiter == nil || res.Waiter.Holder != "wait" || res.Waiter.QueuedAt != f.w.Created {
		t.Fatalf("detail: %+v", res)
	}
	if res.Reservation.QueuedBehind != "wait" {
		t.Fatalf("queued_behind must name the waiter it lines up behind: %+v", res.Reservation)
	}
}

func TestWaiterBoundReasonAfter120s(t *testing.T) {
	for _, tc := range []struct {
		after time.Duration
		want  string
	}{{60 * time.Second, ReasonWaiterFirst}, {120 * time.Second, ReasonWaiterFirst}, {121 * time.Second, ReasonWaiterBound}} {
		f := newWaiterFixture(t, 600) // lease must outlive the 120 s bound
		f.release(t, f.rep.ID)
		f.now = f.now.Add(tc.after)
		for _, queue := range []bool{false, true} {
			res, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), queue)
			code := res.ReasonCode
			if !queue {
				var wf *WaiterFirstError
				if !errors.As(err, &wf) {
					t.Fatalf("%v queue=%v: %v", tc.after, queue, err)
				}
				code = wf.ReasonCode
			} else if err != nil {
				t.Fatal(err)
			}
			if code != tc.want {
				t.Fatalf("after %v queue=%v: want %s got %s", tc.after, queue, tc.want, code)
			}
		}
	}
}

func TestWaiterFirstControlsStillGrantFull(t *testing.T) {
	legacy := `{"reservations":[{"id":"old","resource":"m1","holder":"wait","regime":"build","status":"queued","start":"2026-11-10T10:00:00Z","est_end":"2026-11-10T11:00:00Z","created":"2026-11-10T10:00:00Z","heartbeat_at":"2026-11-10T10:00:00Z","lease_ttl_sec":120}]}`
	cases := []struct {
		name  string
		setup func(t *testing.T, f *waiterFixture) Reservation
	}{
		{"waiter lease expired", func(t *testing.T, f *waiterFixture) Reservation {
			f.now = f.now.Add(121 * time.Second)
			r := f.req("m1", "rep", RegimeBuild)
			r.Start, r.EstEnd = "2026-11-10T10:02:00Z", "2026-11-10T11:00:00Z"
			return r
		}},
		{"waiter on another resource", func(t *testing.T, f *waiterFixture) Reservation { return f.req("m2", "rep", RegimeBuild) }},
		{"waiter window not overlapping", func(t *testing.T, f *waiterFixture) Reservation {
			r := f.req("m1", "rep", RegimeBuild)
			r.Start, r.EstEnd = "2026-11-10T12:00:00Z", "2026-11-10T13:00:00Z"
			return r
		}},
		{"third lane the waiter never queued behind", func(t *testing.T, f *waiterFixture) Reservation { return f.req("m1", "third", RegimeBuild) }},
		{"waiter itself", func(t *testing.T, f *waiterFixture) Reservation {
			f.release(t, f.rep.ID)
			return f.req("m1", "wait", RegimeQuiet)
		}},
		{"waiter already served", func(t *testing.T, f *waiterFixture) Reservation {
			f.release(t, f.rep.ID)
			f.now = f.now.Add(10 * time.Second)
			got, err := f.l.Reserve(f.req("m1", "wait", RegimeBuild), false)
			if err != nil || got.Reservation.Share != ShareFull {
				t.Fatalf("waiter must get the hold: %+v %v", got, err)
			}
			f.release(t, got.Reservation.ID)
			f.now = f.now.Add(10 * time.Second)
			return f.req("m1", "rep", RegimeBuild)
		}},
		{"no waiter at all", func(t *testing.T, f *waiterFixture) Reservation {
			f.release(t, f.rep.ID)
			if err := f.l.store.SetState(stateKey, `{"reservations":[]}`); err != nil {
				t.Fatal(err)
			}
			return f.req("m1", "rep", RegimeQuiet)
		}},
		{"legacy waiter row without queued_behind", func(t *testing.T, f *waiterFixture) Reservation {
			if err := f.l.store.SetState(stateKey, legacy); err != nil {
				t.Fatal(err)
			}
			return f.req("m1", "rep", RegimeQuiet)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newWaiterFixture(t, 120)
			req := tc.setup(t, f)
			// rep still holds in cases that did not release; a third lane then
			// takes the ordinary contention path, so give it a free machine view.
			res, err := f.l.Reserve(req, false)
			if err != nil {
				t.Fatalf("must not be refused: %v", err)
			}
			if tc.name == "third lane the waiter never queued behind" {
				// rep still holds m1, so the ordinary floor grant applies — the
				// point is that it was NOT refused for the waiter.
				if !res.Granted || res.ReasonCode != "" {
					t.Fatalf("third lane: %+v", res)
				}
				return
			}
			if !res.Granted || res.Reservation.Share != ShareFull || res.ReasonCode != "" {
				t.Fatalf("want a plain full grant: %+v", res)
			}
		})
	}
}

// A third lane arriving after rep released is ordinary contention, not chaining.
func TestWaiterFirstThirdLaneFullWhenFree(t *testing.T) {
	f := newWaiterFixture(t, 120)
	f.release(t, f.rep.ID)
	res, err := f.l.Reserve(f.req("m1", "third", RegimeBuild), false)
	if err != nil || !res.Granted || res.Reservation.Share != ShareFull {
		t.Fatalf("third lane: %+v %v", res, err)
	}
}

func TestWaiterFirstClosesSameHolderAdditiveRefresh(t *testing.T) {
	f := newWaiterFixture(t, 120) // rep's first hold is still ACTIVE
	_, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), false)
	if !errors.Is(err, ErrWaiterFirst) {
		t.Fatalf("an overlapping own hold must not bypass the waiter: %v", err)
	}
}

func TestWaiterFirstNamesOldestWaiter(t *testing.T) {
	f := newWaiterFixture(t, 120) // "wait" queued at 10:00:00
	f.now = f.now.Add(30 * time.Second)
	if q, err := f.l.Reserve(f.req("m1", "later", RegimeBuild), true); err != nil || !q.Queued || q.Reservation.QueuedBehind != "rep" {
		t.Fatalf("second waiter: %+v %v", q, err)
	}
	f.release(t, f.rep.ID)
	f.now = f.now.Add(10 * time.Second)
	_, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), false)
	var wf *WaiterFirstError
	if !errors.As(err, &wf) || wf.Waiter.Holder != "wait" {
		t.Fatalf("the oldest waiter must be named: %v", err)
	}
}

func TestWaiterFirstQueuedRowIsDurableAndAdditive(t *testing.T) {
	f := newWaiterFixture(t, 120)
	raw, _, err := f.l.store.GetState(stateKey)
	if err != nil || !strings.Contains(raw, `"queued_behind":"rep"`) {
		t.Fatalf("queued_behind not persisted: %s %v", raw, err)
	}
	if strings.Count(raw, "queued_behind") != 1 {
		t.Fatalf("queued_behind must exist only on the queued row: %s", raw)
	}
}

func TestWaiterFirstErrorTextIsNotATransportError(t *testing.T) {
	f := newWaiterFixture(t, 600)
	f.release(t, f.rep.ID)
	forbidden := []string{"outcome unknown", "http 5", "service unreachable", "connection lost", "connection reset", "timeout awaiting"}
	for _, after := range []time.Duration{0, 200 * time.Second} {
		f.now = at("2026-11-10T10:00:00Z").Add(after)
		_, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), false)
		if err == nil {
			t.Fatal("want refusal")
		}
		low := strings.ToLower(err.Error())
		for _, bad := range forbidden {
			if strings.Contains(low, bad) {
				t.Fatalf("refusal text contains %q: %s", bad, err)
			}
		}
		if !strings.Contains(err.Error(), "wait has been waiting for m1 since") || !strings.Contains(err.Error(), "goes next") {
			t.Fatalf("text must name who goes next: %s", err)
		}
	}
}
