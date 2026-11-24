package schedule

import (
	"errors"
	"testing"
)

// SSA 20261122-004747: only a well-formed, non-empty older queued window that
// overlaps a NON-EMPTY req∩waiter interval waives a waiter. Empty, reversed,
// unparseable and (for req/waiter) malformed windows fail closed.
func TestOwnOlderQueuedNeedsWellFormedWindows(t *testing.T) {
	now := at("2026-11-10T10:00:30Z")
	const mid = "2026-11-10T10:30:00Z"
	for _, tc := range []struct {
		name               string
		oldStart, oldEnd   string
		reqStart, reqEnd   string
		waitStart, waitEnd string
		waives             bool
	}{
		{"valid overlap waives", pStart, pEnd, pStart, pEnd, pStart, pEnd, true},
		{"valid partial overlap waives", mid, "2026-11-10T12:00:00Z", pStart, pEnd, pStart, pEnd, true},
		{"older open-ended (empty est_end) waives", pStart, "", pStart, pEnd, pStart, pEnd, true},
		{"older empty window inside contested", mid, mid, pStart, pEnd, pStart, pEnd, false},
		{"older reversed window", "2026-11-10T10:45:00Z", "2026-11-10T10:15:00Z", pStart, pEnd, pStart, pEnd, false},
		{"older unparseable start", "garbage", pEnd, pStart, pEnd, pStart, pEnd, false},
		{"older empty start", "", pEnd, pStart, pEnd, pStart, pEnd, false},
		{"request empty window", pStart, pEnd, mid, mid, pStart, pEnd, false},
		{"request reversed window", pStart, pEnd, "2026-11-10T10:45:00Z", "2026-11-10T10:15:00Z", pStart, pEnd, false},
		{"request unparseable start", pStart, pEnd, "garbage", pEnd, pStart, pEnd, false},
		{"waiter empty window", pStart, pEnd, pStart, pEnd, mid, mid, false},
		{"waiter reversed window", pStart, pEnd, pStart, pEnd, "2026-11-10T10:45:00Z", "2026-11-10T10:15:00Z", false},
		{"empty intersection", pStart, "2026-11-10T13:00:00Z", pStart, pEnd, "2026-11-10T12:00:00Z", "2026-11-10T13:00:00Z", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := pReq("A")
			req.Start, req.EstEnd = tc.reqStart, tc.reqEnd
			w := Reservation{ID: "b-owed", Resource: "m1", Holder: "B", Status: StatusQueued, Start: tc.waitStart, EstEnd: tc.waitEnd, Created: "2026-11-10T10:00:10Z", LeaseTTLSec: 120, QueuedBehind: "A"}
			o := Reservation{ID: "a-old", Resource: "m1", Holder: "A", Status: StatusQueued, Start: tc.oldStart, EstEnd: tc.oldEnd, Created: "2026-11-10T10:00:00Z", HeartbeatAt: "2026-11-10T10:00:00Z", LeaseTTLSec: 120, QueuedBehind: "X"}
			if got := ownOlderQueued([]Reservation{o, w}, req, w, at(w.Created), now); got != tc.waives {
				t.Fatalf("waives=%v want %v", got, tc.waives)
			}
		})
	}
}

// End to end: SSA's counterexample older rows still leave the waiter its turn.
func TestWaiterFirstEmptyReversedOlderRowDoesNotWaive(t *testing.T) {
	now := at("2026-11-10T10:00:30Z")
	owed := jrow("b-owed", "B", "queued", "", "2026-11-10T10:00:10Z", "A", pStart, pEnd)
	for name, w := range map[string][2]string{
		"empty":    {"2026-11-10T10:30:00Z", "2026-11-10T10:30:00Z"},
		"reversed": {"2026-11-10T10:45:00Z", "2026-11-10T10:15:00Z"},
	} {
		old := jrow("a-old", "A", "queued", "", "2026-11-10T10:00:00Z", "X", w[0], w[1])
		if _, err := stateLedger(t, now, old, owed).Reserve(pReq("A"), false); !errors.Is(err, ErrWaiterFirst) {
			t.Fatalf("%s older row must not waive: %v", name, err)
		}
	}
}
