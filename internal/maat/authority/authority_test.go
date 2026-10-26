package authority

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// fakeStore is a minimal in-memory StateStore + atomic CAS store, one per
// Ledger (so "local" and "cloud" never share state in a test).
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

func (f *fakeStore) CompareAndSwapState(key, expected string, exists bool, value string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	old, found := f.kv[key]
	if old != expected || found != exists {
		return false, nil
	}
	f.kv[key] = value
	return true, nil
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func mkReq(resource, holder, start, end string) schedule.Reservation {
	return schedule.Reservation{Resource: resource, Holder: holder, Start: start, EstEnd: end, Regime: schedule.RegimeQuiet, LeaseTTLSec: 120}
}

// harness wires a local ledger, a cloud ledger (with pod's delegation already
// live), and an Authority over both, all pinned to the same fixed clock.
func harness(t *testing.T, now string, notAfter string) (local, cloud *schedule.Ledger, auth *Authority) {
	t.Helper()
	clock := func() time.Time { return at(now) }
	local = schedule.NewLedger(newFakeStore()).WithClock(clock)
	cloud = schedule.NewLedger(newFakeStore()).WithClock(clock)
	d, err := cloud.Delegate(schedule.Delegation{
		Pod: "studio-m5", AuthorityMachine: "M5-Studio", Resources: []string{"studio", "m5"},
		IssuedAt: now, NotAfter: notAfter,
	})
	if err != nil {
		t.Fatalf("seed delegation: %v", err)
	}
	auth, err = New(d, local, cloud, NewMemJournal())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	auth.WithClock(clock)
	return local, cloud, auth
}

func TestReserve_AdmitsJournalsAndWritesThrough(t *testing.T) {
	_, cloud, auth := harness(t, "2026-10-22T10:00:00Z", "2026-10-22T22:00:00Z")
	res, err := auth.Reserve(mkReq("studio", "claude-io", "2026-10-22T10:00:00Z", "2026-10-22T10:30:00Z"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Granted || res.Reservation == nil {
		t.Fatalf("want granted, got %+v", res)
	}
	if res.Reservation.Authority != "pod:studio-m5" || res.Reservation.Epoch != 1 {
		t.Fatalf("want stamped authority/epoch, got %+v", res.Reservation)
	}
	if res.Reservation.Seq == 0 {
		t.Fatal("want a nonzero journal seq")
	}
	rows, err := cloud.Status("studio")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != res.Reservation.ID {
		t.Fatalf("want the grant written through to the cloud ledger, got %+v", rows)
	}
	d, err := cloud.GetDelegation("studio-m5")
	if err != nil {
		t.Fatal(err)
	}
	if d.AckedSeq != res.Reservation.Seq {
		t.Fatalf("want acked_seq advanced to %d, got %d", res.Reservation.Seq, d.AckedSeq)
	}
}

func TestReserve_ExpiredFencePreventsGrant(t *testing.T) {
	// not_after is already in the past relative to "now".
	_, cloud, auth := harness(t, "2026-10-22T22:00:01Z", "2026-10-22T22:00:00Z")
	if !auth.Fenced() {
		t.Fatal("want an authority past its delegation window to report Fenced")
	}
	res, err := auth.Reserve(mkReq("studio", "claude-io", "2026-10-22T22:00:01Z", "2026-10-22T22:30:00Z"), false)
	if err == nil || !strings.Contains(err.Error(), "fenced") {
		t.Fatalf("want a fenced refusal, got res=%+v err=%v", res, err)
	}
	rows, err := cloud.Status("studio")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("an expired authority must grant nothing, got %+v", rows)
	}
}

// A release through the pod authority must journal and write through
// exactly like a grant (ADR-120 §5: "release... included"), and the cloud
// copy must actually show the released state, not be forced back to
// Active by ReserveAsAuthority's grant-path status handling.
func TestRelease_JournalsAndWritesThroughReleasedStatus(t *testing.T) {
	_, cloud, auth := harness(t, "2026-10-22T10:00:00Z", "2026-10-22T22:00:00Z")
	res, err := auth.Reserve(mkReq("studio", "claude-io", "2026-10-22T10:00:00Z", "2026-10-22T10:30:00Z"), false)
	if err != nil || !res.Granted {
		t.Fatalf("seed grant: res=%+v err=%v", res, err)
	}
	released, err := auth.Release(res.Reservation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if released.Status != schedule.StatusReleased {
		t.Fatalf("want a released reservation, got %+v", released)
	}
	if released.Seq <= res.Reservation.Seq {
		t.Fatalf("want a journal seq after the grant's, got %d (grant was %d)", released.Seq, res.Reservation.Seq)
	}
	rows, err := cloud.Status("studio")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != released.ID || rows[0].Status != schedule.StatusReleased {
		t.Fatalf("want the cloud copy showing released (not re-forced active), got %+v", rows)
	}
	d, err := cloud.GetDelegation("studio-m5")
	if err != nil {
		t.Fatal(err)
	}
	if d.AckedSeq != released.Seq {
		t.Fatalf("want acked_seq advanced to %d, got %d", released.Seq, d.AckedSeq)
	}
}

func TestRelease_FencedRefuses(t *testing.T) {
	// not_after is already in the past relative to "now".
	_, _, auth := harness(t, "2026-10-22T22:00:01Z", "2026-10-22T22:00:00Z")
	if _, err := auth.Release("whatever"); err == nil || !strings.Contains(err.Error(), "fenced") {
		t.Fatalf("want a fenced refusal, got err=%v", err)
	}
}

func TestRenew_SupersededEpochFencesPermanently(t *testing.T) {
	_, cloud, auth := harness(t, "2026-10-22T10:00:00Z", "2026-10-22T22:00:00Z")
	// A fresh Delegate() for the same pod bumps the epoch out from under the
	// running Authority instance (ADR-120 §1/§3): a new authority took over.
	// Reissuing the same pod is itself fenced until the old delegation's
	// window plus the handback margin elapses (delegationHandbackMargin), so
	// the reissue must happen on or after that boundary, not at the original
	// harness "now".
	cloud.WithClock(func() time.Time { return at("2026-10-22T22:05:01Z") })
	if _, err := cloud.Delegate(schedule.Delegation{
		Pod: "studio-m5", AuthorityMachine: "M5-Studio-2", Resources: []string{"studio", "m5"},
		IssuedAt: "2026-10-22T22:05:01Z", NotAfter: "2026-10-23T04:05:01Z",
	}); err != nil {
		t.Fatalf("reissue: %v", err)
	}
	err := auth.Renew()
	if err == nil || !strings.Contains(err.Error(), "fenced") {
		t.Fatalf("want a fenced error from Renew against a superseded epoch, got %v", err)
	}
	if !auth.Fenced() {
		t.Fatal("want the old-epoch instance permanently fenced after a superseded Renew")
	}
	if _, err := auth.Reserve(mkReq("studio", "claude-io", "2026-10-22T10:10:00Z", "2026-10-22T10:30:00Z"), false); err == nil {
		t.Fatal("a fenced authority must refuse to grant after Renew sees itself superseded")
	}
}

func TestRenew_RevokedFencesPermanently(t *testing.T) {
	_, cloud, auth := harness(t, "2026-10-22T10:00:00Z", "2026-10-22T22:00:00Z")
	if _, err := cloud.Revoke("studio-m5"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := auth.Renew(); err == nil || !strings.Contains(err.Error(), "fenced") {
		t.Fatalf("want a fenced error from Renew against a revoked delegation, got %v", err)
	}
	if !auth.Fenced() {
		t.Fatal("want the instance permanently fenced after a revoked Renew")
	}
}

func TestCatchUp_CrashBetweenUpsertAndAck_NoDuplicateRow(t *testing.T) {
	_, cloud, auth := harness(t, "2026-10-22T10:00:00Z", "2026-10-22T22:00:00Z")
	req := mkReq("studio", "claude-io", "2026-10-22T10:00:00Z", "2026-10-22T10:30:00Z")
	local, err := auth.Local.Reserve(req, false)
	if err != nil || !local.Granted {
		t.Fatalf("local admission: res=%+v err=%v", local, err)
	}
	stamped := *local.Reservation
	stamped.Authority = "pod:studio-m5"
	stamped.Epoch = 1
	entry, err := auth.journal.Append(stamped)
	if err != nil {
		t.Fatalf("journal append: %v", err)
	}
	// Simulate the upsert having already reached the cloud once, but the
	// crash happened before AckDelegation (or the in-memory ackedSeq bump)
	// persisted -- auth.ackedSeq is still 0, same as it would be after a
	// process restart that reread an un-advanced acked_seq from the cloud.
	stamped.Seq = entry.Seq
	if _, err := cloud.ReserveAsAuthority(stamped); err != nil {
		t.Fatalf("simulated pre-crash upsert: %v", err)
	}
	// CatchUp now replays the same (unacked, from its point of view) entry.
	if err := auth.CatchUp(); err != nil {
		t.Fatalf("CatchUp: %v", err)
	}
	rows, err := cloud.Status("studio")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want exactly one row after a replayed catch-up, got %d: %+v", len(rows), rows)
	}
	if rows[0].ID != stamped.ID {
		t.Fatalf("want the same reservation id rewritten in place, got %q want %q", rows[0].ID, stamped.ID)
	}
	d, err := cloud.GetDelegation("studio-m5")
	if err != nil {
		t.Fatal(err)
	}
	if d.AckedSeq != entry.Seq {
		t.Fatalf("want acked_seq advanced to %d after catch-up, got %d", entry.Seq, d.AckedSeq)
	}
}

func TestCatchUp_NothingToReplayIsANoop(t *testing.T) {
	_, _, auth := harness(t, "2026-10-22T10:00:00Z", "2026-10-22T22:00:00Z")
	if err := auth.CatchUp(); err != nil {
		t.Fatalf("an empty journal must catch up cleanly: %v", err)
	}
}

// A grant journaled while the cloud is unreachable must not be skipped by a
// later grant's ack: acking N while N-1 is still unacked would tell the
// cloud it has seen everything through N, permanently losing N-1 and risking
// a double booking (ADR-120 review P1-a).
func TestReserve_OutOfOrderAckDoesNotLoseCatchUp(t *testing.T) {
	local := schedule.NewLedger(newFakeStore()).WithClock(func() time.Time { return at("2026-10-22T10:00:00Z") })
	cloud := schedule.NewLedger(newFakeStore()).WithClock(func() time.Time { return at("2026-10-22T10:00:00Z") })
	d, err := cloud.Delegate(schedule.Delegation{
		Pod: "studio-m5", AuthorityMachine: "M5-Studio", Resources: []string{"studio", "m5"},
		IssuedAt: "2026-10-22T10:00:00Z", NotAfter: "2026-10-22T22:00:00Z",
	})
	if err != nil {
		t.Fatalf("seed delegation: %v", err)
	}
	auth, err := New(d, local, nil, NewMemJournal())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	auth.WithClock(func() time.Time { return at("2026-10-22T10:00:00Z") })

	// Grant 1 (seq 1) journals only -- the cloud is unreachable.
	first, err := auth.Reserve(mkReq("studio", "claude-io", "2026-10-22T10:00:00Z", "2026-10-22T10:30:00Z"), false)
	if err != nil || !first.Granted {
		t.Fatalf("grant 1: res=%+v err=%v", first, err)
	}

	// The cloud returns, and grant 2 (seq 2) writes through directly.
	auth.SetCloud(cloud)
	second, err := auth.Reserve(mkReq("m5", "claude-io", "2026-10-22T10:05:00Z", "2026-10-22T10:35:00Z"), false)
	if err != nil || !second.Granted {
		t.Fatalf("grant 2: res=%+v err=%v", second, err)
	}

	studioRows, err := cloud.Status("studio")
	if err != nil {
		t.Fatal(err)
	}
	m5Rows, err := cloud.Status("m5")
	if err != nil {
		t.Fatal(err)
	}
	if len(studioRows) != 1 || len(m5Rows) != 1 {
		t.Fatalf("want both grants written through to the cloud, got studio=%+v m5=%+v", studioRows, m5Rows)
	}
	d2, err := cloud.GetDelegation("studio-m5")
	if err != nil {
		t.Fatal(err)
	}
	if d2.AckedSeq != 2 {
		t.Fatalf("want acked_seq advanced to 2 once both entries reconcile, got %d", d2.AckedSeq)
	}
}

// A journal append failure after Local admission must release the local
// hold rather than leave the resource blocked with no record anywhere
// until its lease TTL expires (ADR-120 review P1-b).
func TestReserve_JournalFailureReleasesLocalHold(t *testing.T) {
	_, cloud, auth := harness(t, "2026-10-22T10:00:00Z", "2026-10-22T22:00:00Z")
	auth.journal = failingJournal{}
	if _, err := auth.Reserve(mkReq("studio", "claude-io", "2026-10-22T10:00:00Z", "2026-10-22T10:30:00Z"), false); err == nil {
		t.Fatal("want the journal failure surfaced as an error")
	}
	rows, err := auth.Local.Status("studio")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Status == schedule.StatusActive {
			t.Fatalf("want no live local hold after a journal failure, got %+v", r)
		}
	}
	if rows, err := cloud.Status("studio"); err != nil || len(rows) != 0 {
		t.Fatalf("a journal failure must write through nothing, got rows=%+v err=%v", rows, err)
	}
}

type failingJournal struct{}

func (failingJournal) Append(schedule.Reservation) (Entry, error) {
	return Entry{}, fmt.Errorf("simulated journal failure")
}
func (failingJournal) Since(int64) ([]Entry, error) { return nil, nil }
func (failingJournal) Head() (int64, error)         { return 0, nil }
