package schedule

import (
	"sync"
	"testing"
	"time"
)

// fakeStore is an in-memory StateStore for tests.
type fakeStore struct {
	mu sync.Mutex
	kv map[string]string
}

func newFakeStore() *fakeStore { return &fakeStore{kv: map[string]string{}} }

func (f *fakeStore) GetState(key string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.kv[key]
	return v, ok, nil
}

func (f *fakeStore) SetState(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.kv[key] = value
	return nil
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func fixedLedger(now string) *Ledger {
	l := NewLedger(newFakeStore())
	t := at(now)
	return l.WithClock(func() time.Time { return t })
}

func mkReq(resource, holder, start, end string, regime Regime) Reservation {
	return Reservation{Resource: resource, Holder: holder, Start: start, EstEnd: end, Regime: regime, LeaseTTLSec: 120}
}

func TestReserve_GrantsFreeResource(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	res, err := l.Reserve(mkReq("m1", "claude-io", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeQuiet), false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Granted || res.Reservation == nil || res.Reservation.Status != StatusActive {
		t.Fatalf("want granted active, got %+v", res)
	}
}

func TestReserve_RefusesOverlappingForeignHold(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	if _, err := l.Reserve(mkReq("rail-a", "claude-io", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeQuiet), false); err != nil {
		t.Fatal(err)
	}
	res, err := l.Reserve(mkReq("rail-a", "sne", "2026-09-24T10:15:00Z", "2026-09-24T10:45:00Z", RegimeQuiet), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Granted {
		t.Fatal("overlapping foreign reservation must be refused")
	}
	if res.Conflict == nil || res.Conflict.Holder != "claude-io" {
		t.Fatalf("refusal must name the holder, got %+v", res.Conflict)
	}
}

func TestReserve_NonOverlappingSameResource_BothGranted(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	a, _ := l.Reserve(mkReq("rail-a", "claude-io", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeQuiet), false)
	b, _ := l.Reserve(mkReq("rail-a", "sne", "2026-09-24T10:30:00Z", "2026-09-24T11:00:00Z", RegimeQuiet), false)
	if !a.Granted || !b.Granted {
		t.Fatalf("adjacent (non-overlapping) windows must both grant: a=%v b=%v", a.Granted, b.Granted)
	}
}

func TestReserve_DifferentResources_NoConflict(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	a, _ := l.Reserve(mkReq("rail-a", "claude-io", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeQuiet), false)
	b, _ := l.Reserve(mkReq("rail-b", "sne", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeQuiet), false)
	if !a.Granted || !b.Granted {
		t.Fatal("different resources must not conflict")
	}
}

func TestReserve_ExpiredLeaseFreesResource_NoStaleLocks(t *testing.T) {
	l := NewLedger(newFakeStore())
	base := at("2026-09-24T10:00:00Z")
	clock := base
	l.WithClock(func() time.Time { return clock })

	r := mkReq("m5", "dead-holder", "2026-09-24T10:00:00Z", "2026-09-24T12:00:00Z", RegimeQuiet)
	r.LeaseTTLSec = 60
	if res, err := l.Reserve(r, false); err != nil || !res.Granted {
		t.Fatalf("initial reserve: %v %+v", err, res)
	}
	// Holder stops heartbeating; clock advances past the lease TTL.
	clock = base.Add(5 * time.Minute)
	res, err := l.Reserve(mkReq("m5", "next-holder", "2026-09-24T10:05:00Z", "2026-09-24T10:35:00Z", RegimeQuiet), false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Granted {
		t.Fatalf("a lapsed lease must free the resource (no stale locks), got %+v", res)
	}
}

func TestHeartbeat_KeepsLeaseAlive(t *testing.T) {
	l := NewLedger(newFakeStore())
	base := at("2026-09-24T10:00:00Z")
	clock := base
	l.WithClock(func() time.Time { return clock })

	r := mkReq("m5", "holder", "2026-09-24T10:00:00Z", "2026-09-24T12:00:00Z", RegimeQuiet)
	r.LeaseTTLSec = 60
	res, _ := l.Reserve(r, false)
	clock = base.Add(50 * time.Second)
	if _, err := l.Heartbeat(res.Reservation.ID); err != nil {
		t.Fatal(err)
	}
	clock = base.Add(90 * time.Second) // 40s since heartbeat < 60s TTL → still live
	blocked, err := l.Reserve(mkReq("m5", "intruder", "2026-09-24T10:01:00Z", "2026-09-24T10:31:00Z", RegimeQuiet), false)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Granted {
		t.Fatal("a heartbeated lease must still block a foreign reservation")
	}
}

func TestReserve_Queue_RecordsBehindHolder(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	l.Reserve(mkReq("rail-c", "claude-io", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeQuiet), false)
	res, err := l.Reserve(mkReq("rail-c", "sne", "2026-09-24T10:10:00Z", "2026-09-24T10:40:00Z", RegimeQuiet), true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Granted || !res.Queued || res.Conflict.Holder != "claude-io" {
		t.Fatalf("want queued behind claude-io, got %+v", res)
	}
}

func TestRelease_And_WhoIsOn(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	res, _ := l.Reserve(mkReq("m1", "claude-io", "2026-09-24T09:30:00Z", "2026-09-24T11:00:00Z", RegimeLoaded), false)
	who, _ := l.WhoIsOn("m1")
	if who == nil || who.Holder != "claude-io" {
		t.Fatalf("who-is-on should be claude-io, got %+v", who)
	}
	if _, err := l.Release(res.Reservation.ID); err != nil {
		t.Fatal(err)
	}
	who, _ = l.WhoIsOn("m1")
	if who != nil {
		t.Fatalf("after release the resource is free, got %+v", who)
	}
}

func TestInvalidate_MarksBlockAndIntruder(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	res, _ := l.Reserve(mkReq("rail-a", "claude-io", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeQuiet), false)
	inv, err := l.Invalidate(res.Reservation.ID, "bench:sr-A-fwd(unknown)")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != StatusInvalidated || inv.InvalidatedBy == "" {
		t.Fatalf("want invalidated with intruder, got %+v", inv)
	}
}

func TestCoverage_UtilizationAndConflicts(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	l.Reserve(mkReq("m1", "claude-io", "2026-09-24T10:00:00Z", "2026-09-24T11:00:00Z", RegimeQuiet), false) // 60 min
	bad, _ := l.Reserve(mkReq("m1", "sne", "2026-09-24T12:00:00Z", "2026-09-24T12:30:00Z", RegimeQuiet), false)
	l.Invalidate(bad.Reservation.ID, "build:vite")
	cov, err := l.Coverage("m1", 24)
	if err != nil {
		t.Fatal(err)
	}
	if cov.Current == nil || cov.Current.Holder != "claude-io" {
		t.Fatalf("current holder should be claude-io, got %+v", cov.Current)
	}
	if cov.ReservedMinutes != 60 {
		t.Fatalf("want 60 reserved minutes, got %d", cov.ReservedMinutes)
	}
	if cov.ConflictsCaught != 1 {
		t.Fatalf("want 1 conflict caught, got %d", cov.ConflictsCaught)
	}
}

func TestShouldDefer_QuietReservationDefersRunner(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	l.Reserve(mkReq("m5", "claude-io", "2026-09-24T09:00:00Z", "2026-09-24T11:00:00Z", RegimeQuiet), false)
	defer1, cur, err := l.ShouldDefer("m5")
	if err != nil {
		t.Fatal(err)
	}
	if !defer1 || cur == nil {
		t.Fatalf("a quiet reservation must defer the runner, got defer=%v cur=%+v", defer1, cur)
	}
	// build regime does not force a deferral
	l2 := fixedLedger("2026-09-24T10:00:00Z")
	l2.Reserve(mkReq("m5", "ci", "2026-09-24T09:00:00Z", "2026-09-24T11:00:00Z", RegimeBuild), false)
	d2, _, _ := l2.ShouldDefer("m5")
	if d2 {
		t.Fatal("build regime must not force a runner deferral")
	}
}

func TestCheckConflicts_ForeignIntruderDuringQuiet(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	l.Reserve(mkReq("rail-a", "claude-io", "2026-09-24T09:30:00Z", "2026-09-24T11:00:00Z", RegimeQuiet), false)

	SetActivityProbe(func(machine string) ([]Actor, error) {
		return []Actor{
			{Kind: "bench", Detail: "tbraw-bench sr-A-fwd", Owner: ""}, // foreign, unattributed
			{Kind: "bench", Detail: "hermes H6", Owner: "claude-io"},   // the holder's own — expected
		}, nil
	})
	defer SetActivityProbe(probeProcesses)

	rep, err := l.CheckConflicts("rail-a", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean {
		t.Fatal("a foreign bench during a quiet reservation must be a conflict")
	}
	if len(rep.Intruders) != 1 || rep.Intruders[0].Detail != "tbraw-bench sr-A-fwd" {
		t.Fatalf("must name exactly the foreign intruder, got %+v", rep.Intruders)
	}
}

func TestCheckConflicts_BuildRegimeToleratesForeignLoad(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	l.Reserve(mkReq("m5", "ci", "2026-09-24T09:30:00Z", "2026-09-24T11:00:00Z", RegimeBuild), false)
	SetActivityProbe(func(string) ([]Actor, error) { return []Actor{{Kind: "bench", Detail: "x", Owner: "other"}}, nil })
	defer SetActivityProbe(probeProcesses)
	rep, _ := l.CheckConflicts("m5", "m5")
	if !rep.Clean {
		t.Fatal("build regime tolerates foreign load")
	}
}

func TestValidate_RejectsBadInput(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	if _, err := l.Reserve(mkReq("RAIL A", "x", "2026-09-24T10:00:00Z", "", RegimeQuiet), false); err == nil {
		t.Fatal("bad resource must be rejected")
	}
	if _, err := l.Reserve(mkReq("m1", "", "2026-09-24T10:00:00Z", "", RegimeQuiet), false); err == nil {
		t.Fatal("empty holder must be rejected")
	}
	if _, err := l.Reserve(mkReq("m1", "x", "not-a-time", "", RegimeQuiet), false); err == nil {
		t.Fatal("bad start must be rejected")
	}
}
