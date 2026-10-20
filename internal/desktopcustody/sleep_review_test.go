package desktopcustody

import (
	"context"
	"errors"
	"testing"
)

// Recovery must pass the exact capability to both trusted adapters, including
// an expired historical window: recovery is not a fresh dispatch.
type bindingCustodian struct {
	*testCustodian
	want SleepAdmission
	t    *testing.T
}

func (c bindingCustodian) VerifyReturn(ctx context.Context, a SleepAdmission, w WakeReceipt) (ReturnEvidence, error) {
	if a != c.want {
		c.t.Fatalf("recovered admission=%#v want=%#v", a, c.want)
	}
	return c.testCustodian.VerifyReturn(ctx, a, w)
}

type bindingPower struct {
	*testPower
	want SleepAdmission
	t    *testing.T
}

func (p bindingPower) CancelWake(ctx context.Context, a SleepAdmission, r SleepRequest, w WakeReceipt) error {
	if a != p.want {
		p.t.Fatalf("cleanup admission=%#v want=%#v", a, p.want)
	}
	return p.testPower.CancelWake(ctx, a, r, w)
}
func TestReviewAdmissionRecoveryRoundTrip(t *testing.T) {
	f, s := newFixture(t)
	s.Custodian = bindingCustodian{f.c, f.a, t}
	s.Power = bindingPower{f.p, f.a, t}
	if err := s.Dispatch(context.Background(), f.req); !errors.Is(err, ErrReturnVerificationPending) {
		t.Fatal(err)
	}
	f.now = f.a.NotAfter.Add(1)
	if err := s.VerifyReturn(context.Background(), f.req); err != nil {
		t.Fatal(err)
	}
	phase(t, f, SleepVerified)
}
func TestReviewNativeFailureRetainsUnknownAndWake(t *testing.T) {
	f, s := newFixture(t)
	f.p.sleepErr = errors.New("native invocation returned error")
	if err := s.Dispatch(context.Background(), f.req); !errors.Is(err, ErrSleepUnknown) {
		t.Fatalf("error=%v", err)
	}
	events := phase(t, f, SleepUnknown)
	_, _, sl, ca := f.p.counts()
	if sl != 1 || ca != 0 {
		t.Fatalf("sleep=%d cancellation=%d", sl, ca)
	}
	// The journal records adapter outcome separately from physical sleep proof.
	found := false
	for _, e := range events {
		if string(e.Phase) == "native-returned" && e.NativeError == f.p.sleepErr.Error() {
			found = true
		}
	}
	if !found {
		t.Fatal("missing original native error receipt")
	}
	if err := s.Dispatch(context.Background(), f.req); err == nil {
		t.Fatal("ambiguous request retried")
	}
	_, _, sl, _ = f.p.counts()
	if sl != 1 {
		t.Fatal("native invocation repeated")
	}
}
func TestReviewRejectsAuthorizationSubstitution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*SleepEvent)
	}{
		{"nonce", func(e *SleepEvent) { e.Nonce = "other" }},
		{"window-start", func(e *SleepEvent) { e.NotBefore = e.NotBefore.Add(1) }},
		{"window-end", func(e *SleepEvent) { e.NotAfter = e.NotAfter.Add(1) }},
		{"generation", func(e *SleepEvent) { e.CustodyGeneration = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s := newFixture(t)
			root := s.event(SleepPrepared, f.req, f.a, f.q, WakeReceipt{}, "", "")
			next := s.event(WakeScheduled, f.req, f.a, f.q, f.w, "", "")
			tc.change(&next)
			if _, err := admissionFromEvents([]SleepEvent{root, next}); err == nil {
				t.Fatal("substituted capability accepted")
			}
		})
	}
}

type callbackCustodian struct {
	*testCustodian
	afterErr error
	skip     bool
	repeat   bool
	retained SleepAction
}

func (c *callbackCustodian) AuthorizeSleep(ctx context.Context, _ SleepAdmission, _ QuietReceipt, fn SleepAction) error {
	c.retained = fn
	if c.skip {
		return nil
	}
	if err := fn(ctx); err != nil {
		return err
	}
	if c.repeat {
		return fn(ctx)
	}
	return c.afterErr
}
func TestReviewCustodianOutcomeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name            string
		skip, repeat    bool
		after           error
		want            SleepPhase
		sleeps, cancels int
	}{
		{"no-action", true, false, nil, SleepFailed, 0, 1},
		{"post-action-error", false, false, errors.New("custodian lost receipt"), SleepUnknown, 1, 0},
		{"duplicate-action", false, true, nil, SleepUnknown, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s := newFixture(t)
			c := &callbackCustodian{testCustodian: f.c, afterErr: tc.after, skip: tc.skip, repeat: tc.repeat}
			s.Custodian = c
			err := s.Dispatch(context.Background(), f.req)
			if err == nil {
				t.Fatal("missing failure")
			}
			if tc.want == SleepUnknown && !errors.Is(err, ErrSleepUnknown) {
				t.Fatal(err)
			}
			phase(t, f, tc.want)
			if err := c.retained(context.Background()); err == nil {
				t.Fatal("late action accepted")
			}
			_, _, sl, ca := f.p.counts()
			if sl != tc.sleeps || ca != tc.cancels {
				t.Fatalf("sleep=%d cancel=%d", sl, ca)
			}
		})
	}
}

type failPhaseJournal struct {
	SleepJournal
	fail SleepPhase
}

func (j failPhaseJournal) AppendSleep(e SleepEvent) error {
	if e.Phase == j.fail {
		return errors.New("journal unavailable")
	}
	return j.SleepJournal.AppendSleep(e)
}
func TestReviewNativeReceiptPersistenceFailures(t *testing.T) {
	for _, tc := range []struct {
		phase           SleepPhase
		sleeps, cancels int
		want            SleepPhase
	}{
		{SleepInvoked, 0, 1, SleepFailed}, {SleepNativeReturned, 1, 0, SleepUnknown},
	} {
		t.Run(string(tc.phase), func(t *testing.T) {
			f, s := newFixture(t)
			s.Journal = failPhaseJournal{f.j, tc.phase}
			if err := s.Dispatch(context.Background(), f.req); err == nil {
				t.Fatal("missing error")
			}
			phase(t, f, tc.want)
			_, _, sl, ca := f.p.counts()
			if sl != tc.sleeps || ca != tc.cancels {
				t.Fatalf("sleep=%d cancel=%d", sl, ca)
			}
		})
	}
}

func TestReviewOriginalAdmissionRoundTrip(t *testing.T) {
	f, s := newFixture(t)
	if err := s.Dispatch(context.Background(), f.req); !errors.Is(err, ErrReturnVerificationPending) {
		t.Fatal(err)
	}
	events, err := f.j.ReadSleep(f.req.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, err := admissionFromEvents(events)
	if err != nil {
		t.Fatal(err)
	}
	if a != f.a {
		t.Fatalf("admission loses original nonce/window/generation: got=%#v want=%#v", a, f.a)
	}
}
func TestReviewOriginalNativeFailureUnknown(t *testing.T) {
	f, s := newFixture(t)
	f.p.sleepErr = errors.New("native failure")
	err := s.Dispatch(context.Background(), f.req)
	if !errors.Is(err, ErrSleepUnknown) {
		t.Fatalf("native invocation error classified as denial: %v", err)
	}
	_, _, _, c := f.p.counts()
	if c != 0 {
		t.Fatalf("recovery wake canceled %d", c)
	}
}
