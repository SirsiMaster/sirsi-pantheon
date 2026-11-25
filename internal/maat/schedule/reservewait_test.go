package schedule

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func (f *waiterFixture) claim(t *testing.T, id string) *Reservation {
	t.Helper()
	r, err := f.l.Claim(id)
	if err != nil {
		t.Fatalf("claim %s: %v", id, err)
	}
	return r
}

func (f *waiterFixture) row(t *testing.T, id string) Reservation {
	t.Helper()
	rows, err := f.l.ExactReservation(id)
	if err != nil || len(rows) != 1 {
		t.Fatalf("row %s: %v %v", id, rows, err)
	}
	return rows[0]
}

func TestClaimKeepsWaitingAndHeartbeatsWhileHolderLive(t *testing.T) {
	f := newWaiterFixture(t, 120)
	f.now = f.now.Add(60 * time.Second)
	got := f.claim(t, f.w.ID)
	if got.Status != StatusQueued {
		t.Fatalf("holder still live: must stay queued, got %s", got.Status)
	}
	if got.HeartbeatAt != f.now.Format(time.RFC3339) || got.QueuedBehind != "rep" {
		t.Fatalf("claim must refresh the lease and keep its place: %+v", got)
	}
	if f.rows(t) != 2 {
		t.Fatal("a claim that does not pass must not add rows")
	}
}

func TestClaimGrantsFullInPlaceAndIsIdempotent(t *testing.T) {
	f := newWaiterFixture(t, 120)
	f.release(t, f.rep.ID)
	f.now = f.now.Add(10 * time.Minute) // waited ten minutes
	got := f.claim(t, f.w.ID)
	if got.ID != f.w.ID || got.Status != StatusActive || got.Share != ShareFull || got.QueuedBehind != "" {
		t.Fatalf("want a full grant on the same id: %+v", got)
	}
	// the window starts now and keeps its length: a grant is never born spent
	if got.Start != f.now.Format(time.RFC3339) || got.EstEnd != f.now.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("window not moved to the grant: %s -> %s", got.Start, got.EstEnd)
	}
	before := f.rows(t)
	again := f.claim(t, f.w.ID) // lost reply: ask again
	if again.Status != StatusActive || again.Created != got.Created || f.rows(t) != before {
		t.Fatalf("re-claim must return the same grant and add nothing: %+v rows %d->%d", again, before, f.rows(t))
	}
}

func TestClaimNeverDoubleGrants(t *testing.T) {
	f := newWaiterFixture(t, 120)
	second := f.req("m1", "wait2", RegimeBuild)
	q, err := f.l.Reserve(second, true)
	if err != nil || !q.Queued {
		t.Fatalf("second waiter: %+v %v", q, err)
	}
	f.release(t, f.rep.ID)
	first := f.claim(t, f.w.ID)
	loser := f.claim(t, q.Reservation.ID)
	if first.Status != StatusActive || loser.Status != StatusQueued {
		t.Fatalf("exactly one grant: first %s, second %s", first.Status, loser.Status)
	}
	live, _ := f.l.LiveHolders("m1")
	if len(live) != 1 || live[0].ID != f.w.ID {
		t.Fatalf("one live holder expected: %+v", live)
	}
}

func TestClaimHonorsWaiterFirstThenGrantsAfterTheWaiterRan(t *testing.T) {
	f := newWaiterFixture(t, 120)
	f.release(t, f.rep.ID)
	q, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), true) // repeater lines up behind the waiter
	if err != nil || !q.Queued {
		t.Fatalf("repeater queue: %+v %v", q, err)
	}
	if got := f.claim(t, q.Reservation.ID); got.Status != StatusQueued {
		t.Fatalf("repeater must not jump the waiter: %+v", got)
	}
	if got := f.claim(t, f.w.ID); got.Status != StatusActive {
		t.Fatalf("waiter goes first: %+v", got)
	}
	f.release(t, f.w.ID)
	if got := f.claim(t, q.Reservation.ID); got.Status != StatusActive || got.Share != ShareFull {
		t.Fatalf("repeater runs after the waiter: %+v", got)
	}
}

func TestClaimRefusesRowsThatAreNotQueued(t *testing.T) {
	f := newWaiterFixture(t, 120)
	f.release(t, f.w.ID)
	for _, id := range []string{f.w.ID, "no-such-id"} {
		if _, err := f.l.Claim(id); !errors.Is(err, ErrNotQueued) {
			t.Fatalf("claim %s: want ErrNotQueued, got %v", id, err)
		}
	}
}

// A dead waiter (no claims) holds its turn for one lease and no longer; a live
// one (claims inside the lease) holds it for as long as it keeps claiming.
func TestDeadWaiterStopsHoldingItsTurnWithinOneLease(t *testing.T) {
	for _, tc := range []struct {
		after time.Duration
		claim bool
		want  bool // repeater refused
	}{{119 * time.Second, false, true}, {121 * time.Second, false, false}, {600 * time.Second, true, true}} {
		f := newWaiterFixture(t, 120)
		f.release(t, f.rep.ID)
		if tc.claim { // alive: claims every 30 s, holder is gone so make the repeater the only claimant-free party
			for d := 30 * time.Second; d < tc.after; d += 30 * time.Second {
				f.now = f.now.Add(30 * time.Second)
				// keep the row queued: simulate a still-live holder by re-using Heartbeat semantics via claim on a blocked row
				f.heartbeatQueued(t, f.w.ID)
			}
		} else {
			f.now = f.now.Add(tc.after)
		}
		_, err := f.l.Reserve(f.req("m1", "rep", RegimeBuild), false)
		if refused := errors.Is(err, ErrWaiterFirst); refused != tc.want {
			t.Fatalf("after %v claims=%v: refused=%v want %v (%v)", tc.after, tc.claim, refused, tc.want, err)
		}
	}
}

// heartbeatQueued renews a queued row's lease the way Claim does while blocked.
func (f *waiterFixture) heartbeatQueued(t *testing.T, id string) {
	t.Helper()
	blocker, err := f.l.Reserve(f.req("m1", "blocker", RegimeBuild), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.claim(t, id); got.Status != StatusQueued {
		t.Fatalf("blocked claim must stay queued: %+v", got)
	}
	f.release(t, blocker.Reservation.ID)
}

// fake sleeper: advances the fixture clock and runs the scripted event for that tick.
func (f *waiterFixture) sleeper(onTick func(n int)) (func(context.Context, time.Duration) error, *int) {
	n := new(int)
	return func(ctx context.Context, d time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		*n++
		f.now = f.now.Add(d)
		if onTick != nil {
			onTick(*n)
		}
		return nil
	}, n
}

func (f *waiterFixture) waitReq() Reservation {
	r := f.req("m1", "wait", RegimeBuild)
	r.Work = "cells"
	r.ID = f.w.ID // the id the fixture already queued under
	return r
}

func TestReserveWaitReturnsGrantWhenResourceFrees(t *testing.T) {
	f := newWaiterFixture(t, 120)
	sleep, n := f.sleeper(func(n int) {
		if n == 3 {
			f.release(t, f.rep.ID)
		}
	})
	res, err := f.l.reserveWait(context.Background(), f.waitReq(), 10*time.Minute, sleep)
	if err != nil || !res.Granted || res.Reservation.ID != f.w.ID || res.Reservation.Share != ShareFull {
		t.Fatalf("want the queued row granted: %+v %v", res, err)
	}
	if *n != 3 {
		t.Fatalf("returned on the first claim after the release: %d ticks", *n)
	}
}

func TestReserveWaitTimeoutCancelsTheQueuedRow(t *testing.T) {
	f := newWaiterFixture(t, 120)
	sleep, _ := f.sleeper(nil) // holder never frees
	res, err := f.l.reserveWait(context.Background(), f.waitReq(), time.Minute, sleep)
	if !errors.Is(err, ErrWaitTimeout) || res.ReasonCode != ReasonWaitTimeout || res.Granted {
		t.Fatalf("want wait-timeout: %+v %v", res, err)
	}
	if got := f.row(t, f.w.ID); got.Status != StatusReleased {
		t.Fatalf("the abandoned queue row must be canceled, is %s", got.Status)
	}
}

func TestReserveWaitSignalCancelsTheQueuedRow(t *testing.T) {
	f := newWaiterFixture(t, 120)
	ctx, cancel := context.WithCancel(context.Background())
	sleep, _ := f.sleeper(func(n int) {
		if n == 2 {
			cancel() // SIGINT/SIGTERM in the CLI
		}
	})
	// the 3rd sleep sees the canceled context
	_, err := f.l.reserveWait(ctx, f.waitReq(), time.Hour, sleep)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want cancellation, got %v", err)
	}
	if got := f.row(t, f.w.ID); got.Status != StatusReleased {
		t.Fatalf("cancel must drop the queued row, is %s", got.Status)
	}
}

// Reply lost to the first send: the row exists, the retry (same id) must find
// it rather than fail on "duplicate id" or queue a second row.
func TestReserveWaitRecoversALostQueueReply(t *testing.T) {
	f := newWaiterFixture(t, 120)
	sleep, _ := f.sleeper(func(n int) {
		if n == 1 {
			f.release(t, f.rep.ID)
		}
	})
	before := f.rows(t)
	res, err := f.l.reserveWait(context.Background(), f.waitReq(), time.Hour, sleep)
	if err != nil || !res.Granted || res.Reservation.ID != f.w.ID || f.rows(t) != before {
		t.Fatalf("recover by id: %+v %v rows %d->%d", res, err, before, f.rows(t))
	}
}

// Reply lost to the GRANT: the row is already active; the retry returns it.
func TestReserveWaitRecoversALostGrantReply(t *testing.T) {
	f := newWaiterFixture(t, 120)
	f.release(t, f.rep.ID)
	f.claim(t, f.w.ID) // granted; the client never heard
	res, err := f.l.reserveWait(context.Background(), f.waitReq(), time.Minute, nil)
	if err != nil || !res.Granted || res.Reservation.Status != StatusActive {
		t.Fatalf("lost grant must be recovered, not lost: %+v %v", res, err)
	}
}

func TestReserveWaitRequiresPositiveWait(t *testing.T) {
	f := newWaiterFixture(t, 120)
	if _, err := f.l.ReserveWait(context.Background(), f.waitReq(), 0); err == nil {
		t.Fatal("a zero wait is not a wait")
	}
}

// An old server has no "claim": the waiter must say so, and leave no row behind.
type oldServer struct {
	*fakeStore
	inner *Ledger
}

func (o oldServer) MaatReserve(r Reservation, q bool) (ReserveResult, error) {
	return o.inner.Reserve(r, q)
}

func (o oldServer) MaatMutate(action, id, value string) (*Reservation, error) {
	if action == "release" {
		return o.inner.Release(id)
	}
	return nil, errors.New(`routerstore: invalid input: unknown Ma'at mutation "` + action + `"`)
}

func (o oldServer) MaatRead(kind, id string) ([]Reservation, error) {
	return o.inner.ExactReservation(id)
}

func TestReserveWaitAgainstServerWithoutClaim(t *testing.T) {
	f := newWaiterFixture(t, 120)
	clock := func() time.Time { return f.now }
	l := NewLedger(oldServer{newFakeStore(), f.l}).WithClock(clock)
	sleep, _ := f.sleeper(nil)
	_, err := l.reserveWait(context.Background(), f.waitReq(), time.Minute, sleep)
	if err == nil || !strings.Contains(err.Error(), "does not support --wait") {
		t.Fatalf("want a clear unsupported error, got %v", err)
	}
	if got := f.row(t, f.w.ID); got.Status != StatusReleased {
		t.Fatalf("no queued row may be left behind: %s", got.Status)
	}
}
