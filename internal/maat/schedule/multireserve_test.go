package schedule

import (
	"errors"
	"testing"
	"time"
)

// casCounter counts ledger writes: a multi reserve must be exactly one.
type casCounter struct {
	*fakeStore
	writes int
}

func (c *casCounter) CompareAndSwapState(key, expected string, exists bool, value string) (bool, error) {
	ok, err := c.fakeStore.CompareAndSwapState(key, expected, exists, value)
	if ok {
		c.writes++
	}
	return ok, err
}

func multiFixture() (*Ledger, *casCounter) {
	st := &casCounter{fakeStore: newFakeStore()}
	now := at("2026-11-10T10:00:00Z")
	return NewLedger(st).WithClock(func() time.Time { return now }), st
}

func multiReqs(holder string, resources ...string) []Reservation {
	out := make([]Reservation, len(resources))
	for i, r := range resources {
		out[i] = Reservation{Resource: r, Holder: holder, Work: "w", Regime: RegimeBuild,
			Start: "2026-11-10T10:00:00Z", EstEnd: "2026-11-10T11:00:00Z", LeaseTTLSec: 120}
	}
	return out
}

func TestReserveMultiGrantsAllInOneWrite(t *testing.T) {
	l, st := multiFixture()
	res, err := l.ReserveMulti(multiReqs("a", "m1", "m2", "rail-a"))
	if err != nil || !res.Granted || len(res.Reservations) != 3 {
		t.Fatalf("%+v %v", res, err)
	}
	ids := map[string]bool{}
	for i, m := range res.Reservations {
		if m.Resource != []string{"m1", "m2", "rail-a"}[i] || m.Share != ShareFull || m.Regime != RegimeBuild || m.ID == "" || ids[m.ID] {
			t.Fatalf("row %d: %+v", i, m)
		}
		ids[m.ID] = true
	}
	if st.writes != 1 {
		t.Fatalf("one transaction, one write: got %d writes", st.writes)
	}
	if rows, _ := l.Status(""); len(rows) != 3 {
		t.Fatalf("rows: %d", len(rows))
	}
}

func TestReserveMultiShareIsPerResource(t *testing.T) {
	l, _ := multiFixture()
	if r, err := l.Reserve(multiReqs("other", "m1")[0], false); err != nil || r.Reservation.Share != ShareFull {
		t.Fatal(r, err)
	}
	res, err := l.ReserveMulti(multiReqs("a", "m1", "m2"))
	if err != nil || !res.Granted {
		t.Fatalf("%+v %v", res, err)
	}
	m1, m2 := res.Reservations[0], res.Reservations[1]
	if m1.Share != ShareFloor || m1.Conflict == nil || m1.Conflict.Holder != "other" || m1.Reason == "" || m1.Cores == 0 {
		t.Fatalf("m1 sits beside a live holder: %+v", m1)
	}
	if m2.Share != ShareFull || m2.Conflict != nil {
		t.Fatalf("m2 is free: %+v", m2)
	}
}

// A waiter on ANY resource refuses the whole request and writes nothing.
func TestReserveMultiWaiterFirstOnAnyResourceRefusesAll(t *testing.T) {
	for _, order := range [][]string{{"m1", "m2"}, {"m2", "m1"}} {
		f := newWaiterFixture(t, 120) // "wait" is queued behind "rep" on m1
		f.release(t, f.rep.ID)
		before := f.rows(t)
		res, err := f.l.ReserveMulti(multiReqs("rep", order...))
		var wf *WaiterFirstError
		if !errors.Is(err, ErrWaiterFirst) || !errors.As(err, &wf) || wf.Resource != "m1" || res.Granted || len(res.Reservations) != 0 {
			t.Fatalf("%v: want waiter-first on m1, got %+v %v", order, res, err)
		}
		if f.rows(t) != before {
			t.Fatalf("%v: a refused multi request must write nothing (m2 must not be held)", order)
		}
		out := MultiRefusal(err)
		if out.Granted || out.ReasonCode != ReasonWaiterFirst || out.Error == "" || out.Reservations == nil {
			t.Fatalf("refusal json: %+v", out)
		}
		// negative control: without the waiter the identical request is granted
		f.release(t, f.w.ID)
		if ok, err := f.l.ReserveMulti(multiReqs("rep", order...)); err != nil || !ok.Granted {
			t.Fatalf("%v: control must grant: %+v %v", order, ok, err)
		}
	}
}

func TestReserveMultiRejectsBadRequests(t *testing.T) {
	l, st := multiFixture()
	bad := multiReqs("a", "m1", "m2")
	bad[1].Regime = "nope"
	many := make([]string, maxMultiResources+1)
	for i := range many {
		many[i] = "r" + string(rune('a'+i))
	}
	for name, reqs := range map[string][]Reservation{
		"empty": nil, "duplicate": multiReqs("a", "m1", "m1"), "alias": multiReqs("a", "studio", "m5-studio"),
		"invalid regime": bad, "too many": multiReqs("a", many...),
	} {
		if res, err := l.ReserveMulti(reqs); err == nil || res.Granted {
			t.Fatalf("%s: want a refusal, got %+v %v", name, res, err)
		}
	}
	if st.writes != 0 {
		t.Fatalf("refusals must write nothing: %d writes", st.writes)
	}
}

// A service without the native entry point is an older service: say so, never
// degrade into one write per resource.
func TestReserveMultiAgainstOldServer(t *testing.T) {
	inner, _ := multiFixture()
	l := NewLedger(oldServer{newFakeStore(), inner})
	res, err := l.ReserveMulti(multiReqs("a", "m1", "m2"))
	if !errors.Is(err, ErrMultiUnsupported) || res.Granted {
		t.Fatalf("%+v %v", res, err)
	}
	if out := MultiRefusal(err); out.ReasonCode != ReasonMultiUnsupported {
		t.Fatalf("callers fall back on this code: %+v", out)
	}
	if rows, _ := inner.Status(""); len(rows) != 0 {
		t.Fatalf("nothing may be reserved: %+v", rows)
	}
}
