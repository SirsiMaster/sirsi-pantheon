package desktopcustody

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

type testVerifier struct {
	a   SleepAdmission
	err error
}

func (v testVerifier) VerifySleep(context.Context, SleepRequest) (SleepAdmission, error) {
	return v.a, v.err
}

type testCustodian struct {
	mu                                        sync.Mutex
	q                                         QuietReceipt
	quietCalls, failQuietAt, actions, returns int
	authorizeErr, returnErr                   error
	evidence                                  ReturnEvidence
}

func (c *testCustodian) CurrentQuiet(context.Context, SleepAdmission) (QuietReceipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.quietCalls++
	if c.quietCalls == c.failQuietAt {
		return QuietReceipt{}, errors.New("active Mercury write")
	}
	return c.q, nil
}
func (c *testCustodian) AuthorizeSleep(ctx context.Context, _ SleepAdmission, _ QuietReceipt, fn SleepAction) error {
	c.mu.Lock()
	c.actions++
	err := c.authorizeErr
	c.mu.Unlock()
	if err != nil {
		return err
	}
	return fn(ctx)
}
func (c *testCustodian) VerifyReturn(context.Context, SleepAdmission, WakeReceipt) (ReturnEvidence, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.returns++
	return c.evidence, c.returnErr
}

type testPower struct {
	mu                                        sync.Mutex
	w                                         WakeReceipt
	scheduleErr, readErr, sleepErr, cancelErr error
	schedules, reads, sleeps, cancels         int
}

func (p *testPower) ScheduleWake(context.Context, SleepAdmission, SleepRequest) (WakeReceipt, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.schedules++
	return p.w, p.scheduleErr
}
func (p *testPower) ReadWake(context.Context, SleepAdmission, SleepRequest, WakeReceipt) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	return p.readErr
}
func (p *testPower) Sleep(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sleeps++
	return p.sleepErr
}
func (p *testPower) CancelWake(context.Context, SleepAdmission, SleepRequest, WakeReceipt) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancels++
	return p.cancelErr
}
func (p *testPower) counts() (int, int, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.schedules, p.reads, p.sleeps, p.cancels
}

type fixture struct {
	now time.Time
	req SleepRequest
	a   SleepAdmission
	q   QuietReceipt
	w   WakeReceipt
	j   *FileSleepJournal
	c   *testCustodian
	p   *testPower
}

func newFixture(t *testing.T) (*fixture, SleepService) {
	t.Helper()
	now := time.Date(2026, 10, 15, 17, 20, 0, 0, time.UTC)
	req := SleepRequest{ID: "m5-sleep-1", Host: "m5", WakeAt: now.Add(3 * time.Minute)}
	a := SleepAdmission{ID: "admission-1", Issuer: "ssa-enrolled", Host: "m5", Owner: "cylton", PlatformUUID: "78233ff3-1b63-55db-be02-6ed4662f2413", BootID: "d40f1ac6-ed15-4133-bd43-421a431c55c7", Operation: SleepOperation, RequestID: req.ID, Nonce: "nonce-1", ReviewDigest: "review-1", NotBefore: now.Add(-time.Minute), NotAfter: now.Add(10 * time.Minute), CustodyGeneration: "generation-1"}
	q := QuietReceipt{ID: "mercury-quiet-1", Host: "m5", BootID: a.BootID, Operation: SleepOperation, ExpiresAt: now.Add(5 * time.Minute), Digest: "quiet-1"}
	w := WakeReceipt{ID: "wake-1", Host: "m5", WakeAt: req.WakeAt, OwnerTag: "pantheon-sleep-" + req.ID, Readback: "native-readback"}
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	j, err := NewFileSleepJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	c := &testCustodian{q: q, evidence: ReturnEvidence{RequestID: req.ID, Host: "m5", PlatformUUID: a.PlatformUUID, BootID: a.BootID, SleepRecordID: "sleep-record", WakeRecordID: w.ID, FullWake: true, ObservedAt: req.WakeAt.Add(time.Second), ObserverReceiptDigest: "m1-observer", Digest: "return-1"}}
	p := &testPower{w: w}
	f := &fixture{now: req.WakeAt.Add(-3 * time.Minute), req: req, a: a, q: q, w: w, j: j, c: c, p: p}
	s := SleepService{Verifier: testVerifier{a: a}, Custodian: c, Power: p, Journal: j, Now: func() time.Time { return f.now }}
	return f, s
}
func phase(t *testing.T, f *fixture, want SleepPhase) []SleepEvent {
	t.Helper()
	e, err := f.j.ReadSleep(f.req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(e) == 0 || e[len(e)-1].Phase != want {
		t.Fatalf("events=%#v want terminal %s", e, want)
	}
	return e
}

func TestSleepDispatchAndVerifiedReturn(t *testing.T) {
	f, s := newFixture(t)
	if err := s.Dispatch(context.Background(), f.req); !errors.Is(err, ErrReturnVerificationPending) {
		t.Fatalf("dispatch=%v", err)
	}
	e := phase(t, f, ReturnPending)
	if e[0].QuietDigest != f.q.Digest || e[0].Owner != f.a.Owner {
		t.Fatalf("prepared row missing typed quiet/owner binding: %#v", e[0])
	}
	_, _, sl, ca := f.p.counts()
	if sl != 1 || ca != 0 {
		t.Fatalf("sleep=%d cancel=%d", sl, ca)
	}
	f.now = f.req.WakeAt.Add(time.Second)
	if err := s.VerifyReturn(context.Background(), f.req); err != nil {
		t.Fatal(err)
	}
	phase(t, f, SleepVerified)
	_, _, _, ca = f.p.counts()
	if ca != 1 {
		t.Fatalf("cancel=%d", ca)
	}
}

func TestSleepDenyPathsNeverActuate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		set    func(*fixture, SleepService) SleepService
		want   SleepPhase
		cancel int
	}{
		{"active-write", func(f *fixture, s SleepService) SleepService { f.c.failQuietAt = 2; return s }, SleepFailed, 1},
		{"owner-return", func(f *fixture, s SleepService) SleepService {
			f.c.authorizeErr = errors.New("owner returned")
			return s
		}, SleepFailed, 1},
		{"schedule", func(f *fixture, s SleepService) SleepService {
			f.p.scheduleErr = errors.New("schedule failed")
			return s
		}, SleepFailed, 0},
		{"readback", func(f *fixture, s SleepService) SleepService { f.p.readErr = errors.New("readback failed"); return s }, SleepFailed, 1},
		{"cleanup-unknown", func(f *fixture, s SleepService) SleepService {
			f.p.readErr = errors.New("readback failed")
			f.p.cancelErr = errors.New("cancel failed")
			return s
		}, SleepUnknown, 1},
		{"missing-issuer", func(f *fixture, s SleepService) SleepService { s.Verifier = nil; return s }, "", 0},
		{"expired-admission", func(f *fixture, s SleepService) SleepService {
			f.a.NotAfter = f.now
			s.Verifier = testVerifier{a: f.a}
			return s
		}, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s := newFixture(t)
			s = tc.set(f, s)
			if err := s.Dispatch(context.Background(), f.req); err == nil {
				t.Fatal("dispatch unexpectedly succeeded")
			}
			if tc.want != "" {
				phase(t, f, tc.want)
			}
			_, _, sl, ca := f.p.counts()
			if sl != 0 || ca != tc.cancel {
				t.Fatalf("sleep=%d cancel=%d", sl, ca)
			}
		})
	}
}
func TestSleepInvalidWakeProofBecomesUnknown(t *testing.T) {
	f, s := newFixture(t)
	if err := s.Dispatch(context.Background(), f.req); !errors.Is(err, ErrReturnVerificationPending) {
		t.Fatal(err)
	}
	f.now = f.req.WakeAt.Add(time.Second)
	f.c.evidence.FullWake = false
	if err := s.VerifyReturn(context.Background(), f.req); !errors.Is(err, ErrSleepUnknown) {
		t.Fatalf("verify=%v", err)
	}
	phase(t, f, SleepUnknown)
	_, _, _, ca := f.p.counts()
	if ca != 0 {
		t.Fatalf("unverified wake was canceled=%d", ca)
	}
}
func TestSleepJournalRejectsOwnerSubstitution(t *testing.T) {
	f, s := newFixture(t)
	if err := f.j.BeginSleep(s.event(SleepPrepared, f.req, f.a, f.q, WakeReceipt{}, "", "")); err != nil {
		t.Fatal(err)
	}
	e := s.event(WakeScheduled, f.req, f.a, f.q, f.w, "", "")
	e.Owner = "other-owner"
	if err := f.j.AppendSleep(e); err != nil {
		t.Fatal(err)
	}
	if _, err := f.j.ReadSleep(f.req.ID); err == nil {
		t.Fatal("owner-substituted journal accepted")
	}
}
func TestSleepConcurrentDispatchCreatesOneTransaction(t *testing.T) {
	f, s := newFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.Dispatch(context.Background(), f.req) }()
	}
	wg.Wait()
	close(errs)
	ok := 0
	for err := range errs {
		if errors.Is(err, ErrReturnVerificationPending) {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("successful dispatches=%d want 1", ok)
	}
	_, _, sl, _ := f.p.counts()
	if sl != 1 {
		t.Fatalf("sleeps=%d want 1", sl)
	}
}
