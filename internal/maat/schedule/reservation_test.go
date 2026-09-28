package schedule

import (
	"sync"
	"testing"
	"time"
)

func TestClassifyProc_IdleRunnerServiceIsNotLoad(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want string
	}{
		{"idle listener", "/Users/x/actions-runner/bin/Runner.Listener run", ""},
		{"launchd wrapper", "/Users/x/actions-runner/runsvc.sh", ""},
		{"job in flight", "/Users/x/actions-runner/bin/Runner.Worker spawner.pipe", "build"},
		{"plain go build", "go build ./...", "build"},
	}
	for _, c := range cases {
		if got := classifyProc(c.cmd); got != c.want {
			t.Errorf("%s: classifyProc(%q) = %q, want %q", c.name, c.cmd, got, c.want)
		}
	}
}

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

func TestReserve_OverlappingForeignHoldGrantsFloor_NeverRefuses(t *testing.T) {
	l := fixedLedger("2026-09-24T10:00:00Z")
	if _, err := l.Reserve(mkReq("rail-a", "claude-io", "2026-09-24T10:00:00Z", "2026-09-24T10:30:00Z", RegimeQuiet), false); err != nil {
		t.Fatal(err)
	}
	res, err := l.Reserve(mkReq("rail-a", "sne", "2026-09-24T10:15:00Z", "2026-09-24T10:45:00Z", RegimeQuiet), false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Granted {
		t.Fatal("Ma'at never locks a lane out — an overlapping foreign hold must still grant a floor share")
	}
	if res.Reservation.Share != ShareFloor || res.Reservation.Cores != 1 { // rail-a has no configured capacity → floor 1
		t.Fatalf("want a floor grant of 1 core, got %+v", res.Reservation)
	}
	if res.Conflict == nil || res.Conflict.Holder != "claude-io" {
		t.Fatalf("the floor grant must still name the conflicting holder, got %+v", res.Conflict)
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
	capped, err := l.Reserve(mkReq("m5", "intruder", "2026-09-24T10:01:00Z", "2026-09-24T10:31:00Z", RegimeQuiet), false)
	if err != nil {
		t.Fatal(err)
	}
	if !capped.Granted || capped.Reservation.Share != ShareFloor {
		t.Fatalf("a heartbeated lease still caps a foreign reservation to the floor (never refuses), got %+v", capped)
	}
	if capped.Conflict == nil || capped.Conflict.Holder != "holder" {
		t.Fatalf("the floor grant must name the live holder, got %+v", capped.Conflict)
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

// --- Ma'at never locks a lane out (owner directive 2026-09-26) ---

func TestReserve_TwoLanesOnM5_FirstFullSecondFloor(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	first, err := l.Reserve(mkReq("m5", "hermes", "2026-09-26T10:00:00Z", "2026-09-26T10:30:00Z", RegimeQuiet), false)
	if err != nil || !first.Granted || first.Reservation.Share != ShareFull {
		t.Fatalf("first lane must get the full grant, got %v %+v", err, first)
	}
	second, err := l.Reserve(mkReq("m5", "sne", "2026-09-26T10:00:00Z", "2026-09-26T10:30:00Z", RegimeQuiet), false)
	if err != nil || !second.Granted {
		t.Fatalf("second lane must never be refused, got %v %+v", err, second)
	}
	if second.Reservation.Share != ShareFloor || second.Reservation.Cores != 4 { // m5 default 18/4
		t.Fatalf("second lane on m5 must get the floor share (4 cores), got %+v", second.Reservation)
	}
	if second.Conflict == nil || second.Conflict.Holder != "hermes" {
		t.Fatalf("the floor grant must name the first lane, got %+v", second.Conflict)
	}
}

func TestCheckConflicts_FloorHolderIsSharedNotInvalidated(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	full, _ := l.Reserve(mkReq("m5", "hermes", "2026-09-26T09:30:00Z", "2026-09-26T11:00:00Z", RegimeQuiet), false)
	floor, _ := l.Reserve(mkReq("m5", "sne", "2026-09-26T09:30:00Z", "2026-09-26T11:00:00Z", RegimeQuiet), false)
	if full.Reservation.Share != ShareFull || floor.Reservation.Share != ShareFloor {
		t.Fatalf("setup: want full+floor, got %+v %+v", full.Reservation, floor.Reservation)
	}

	SetActivityProbe(func(string) ([]Actor, error) {
		return []Actor{
			{Kind: "bench", Detail: "hermes H6", Owner: "hermes"}, // the primary holder's own work
			{Kind: "model", Detail: "sne-runner", Owner: "sne"},   // a lane holding a GRANTED floor share
		}, nil
	})
	defer SetActivityProbe(probeProcesses)

	rep, err := l.CheckConflicts("m5", "m5")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean {
		t.Fatalf("a granted floor-share lane must never be reported as an intruder, got %+v", rep)
	}
	if len(rep.Intruders) != 0 {
		t.Fatalf("want zero intruders, got %+v", rep.Intruders)
	}
	if len(rep.Shared) != 1 || rep.Shared[0] != "shared: sne floor 4 cores" {
		t.Fatalf("want the floor lane recorded as shared, got %+v", rep.Shared)
	}

	// Invalidate must never be triggered for a clean report (mirrors the CLI's
	// own gate: it only calls Invalidate when !rep.Clean).
	if rep.Reservation != full.Reservation.ID {
		t.Fatalf("primary holder in the report should be the full-share lane, got %s", rep.Reservation)
	}
}

func TestCapacity_DefaultsAndSet(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	m1, err := l.Capacity("m1")
	if err != nil || m1 != 10 {
		t.Fatalf("want default m1 capacity 10, got %d %v", m1, err)
	}
	m5, err := l.Capacity("m5")
	if err != nil || m5 != 18 {
		t.Fatalf("want default m5 capacity 18, got %d %v", m5, err)
	}
	floorM1, _ := l.FloorShare("m1")
	if floorM1 != 2 {
		t.Fatalf("want m1 floor share 2 (10/4), got %d", floorM1)
	}
	floorM5, _ := l.FloorShare("m5")
	if floorM5 != 4 {
		t.Fatalf("want m5 floor share 4 (18/4), got %d", floorM5)
	}
	unlisted, _ := l.FloorShare("rail-z")
	if unlisted != 1 {
		t.Fatalf("an unlisted resource must still floor to 1, got %d", unlisted)
	}

	if err := l.SetCapacity("m1", 20); err != nil {
		t.Fatal(err)
	}
	m1After, _ := l.Capacity("m1")
	if m1After != 20 {
		t.Fatalf("want capacity 20 after SetCapacity, got %d", m1After)
	}
	floorAfter, _ := l.FloorShare("m1")
	if floorAfter != 5 {
		t.Fatalf("want floor share 5 (20/4) after SetCapacity, got %d", floorAfter)
	}

	if err := l.SetCapacity("m1", 0); err == nil {
		t.Fatal("capacity below 1 must be rejected")
	}
}

func TestFloorShareMemGB_DefaultsToOneUntilConfigured(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	// No owner-provided defaultMemCapacityGB yet — every resource floors to 1,
	// never 0 (an unbounded ask is the exact failure mode this closes).
	memoryBefore, err := l.MemoryCapacityGB("m1")
	if err != nil || memoryBefore != 0 {
		t.Fatalf("want unconfigured memory capacity 0, got %d %v", memoryBefore, err)
	}
	floorM1, err := l.FloorShareMemGB("m1")
	if err != nil || floorM1 != 1 {
		t.Fatalf("want unconfigured mem floor 1, got %d %v", floorM1, err)
	}

	if err = l.SetCapacityMemGB("m1", 32); err != nil {
		t.Fatal(err)
	}
	memory, err := l.MemoryCapacityGB("m1")
	if err != nil || memory != 32 {
		t.Fatalf("want explicit memory capacity 32 GiB, got %d %v", memory, err)
	}
	floorAfter, err := l.FloorShareMemGB("m1")
	if err != nil || floorAfter != 8 {
		t.Fatalf("want mem floor 8 (32/4) after SetCapacityMemGB, got %d %v", floorAfter, err)
	}

	if err = l.SetCapacityMemGB("m1", 0); err == nil {
		t.Fatal("mem capacity below 1 must be rejected")
	}
}

func TestReserve_NeverRefusesExceptInvalidInput(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")

	// Free resource.
	if res, err := l.Reserve(mkReq("m1", "a", "2026-09-26T10:00:00Z", "2026-09-26T10:30:00Z", RegimeQuiet), false); err != nil || !res.Granted {
		t.Fatalf("free resource must grant, got %v %+v", err, res)
	}
	// Overlapping foreign holder, no queue.
	if res, err := l.Reserve(mkReq("m1", "b", "2026-09-26T10:00:00Z", "2026-09-26T10:30:00Z", RegimeQuiet), false); err != nil || !res.Granted {
		t.Fatalf("conflicting resource must still grant (floor), got %v %+v", err, res)
	}
	// A third lane with a pending cede addressed to it.
	c, _ := l.RequestCede(mkCede("m1", "a", "c", "machine", 10, "need it"))
	if res, err := l.Reserve(mkReq("m1", "c", "2026-09-26T10:00:00Z", "2026-09-26T10:30:00Z", RegimeQuiet), false); err != nil || !res.Granted {
		t.Fatalf("a pending cede must still grant (floor), got %v %+v", err, res)
	}
	l.WithdrawCede(c.ID, "a")

	// Invalid input is the ONLY way to come back empty-handed, and it comes
	// back as an error, not a Granted:false result.
	if _, err := l.Reserve(mkReq("BAD RESOURCE", "x", "2026-09-26T10:00:00Z", "", RegimeQuiet), false); err == nil {
		t.Fatal("invalid input must still error")
	}

	// --queue is the one explicit opt-in wait, not a lockout: the caller asked to wait.
	if res, err := l.Reserve(mkReq("m1", "d", "2026-09-26T10:00:00Z", "2026-09-26T10:30:00Z", RegimeQuiet), true); err != nil || res.Granted != false || !res.Queued {
		t.Fatalf("queue is the only Granted=false path for valid input, and only because it was requested, got %v %+v", err, res)
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
