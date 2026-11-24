package routerstore

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func answerFixture() (AnswerAssignment, AnswerOwner, AnswerOwner, time.Time) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	a := AnswerOwner{"lead", "m1", "machine1", "pid1/start1"}
	b := AnswerOwner{"worker", "studio", "machine2", "pid2/start2"}
	return AnswerAssignment{Lane: "mercury", Owner: a, Expires: now.Add(time.Minute).Format(time.RFC3339Nano)}, a, b, now
}
func answerCall(a AnswerAssignment, op string) AnswerHandoffCall {
	return AnswerHandoffCall{Op: op, RequestID: "request-one", ExpectedEpoch: a.Epoch, ExpectedRevision: a.Revision, ExpectedOwner: a.Owner}
}
func TestAnswerHandoffCooperativeSequence(t *testing.T) {
	a, lead, target, now := answerFixture()
	c := answerCall(a, "request")
	c.Target = target
	c.SourceRef = "canonical/request"
	c.TTLSeconds = 60
	var err error
	a, err = transitionAnswer(a, lead, c, now, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Enabled {
		t.Fatal("pending request globally activated fence")
	}
	for _, op := range []string{"accept", "ack-drain", "activate"} {
		c = answerCall(a, op)
		who := target
		if op == "ack-drain" {
			who = lead
			c.DrainRef = "canonical/drain-proof"
		}
		a, err = transitionAnswer(a, who, c, now, true)
		if err != nil {
			t.Fatal(op, err)
		}
	}
	if !a.Enabled || a.Owner != target || a.Epoch != 1 || len(a.History) != 4 {
		t.Fatal(a)
	}
	c = answerCall(a, "renew")
	c.TTLSeconds = 120
	b, err := transitionAnswer(a, target, c, now, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = transitionAnswer(b, target, c, now, true); err == nil {
		t.Fatal("lost-response replay accepted")
	}
	c = answerCall(b, "revoke")
	c.SourceRef = "canonical/revoke"
	b, err = transitionAnswer(b, target, c, now, true)
	if err != nil || !b.Revoked {
		t.Fatal(err, b)
	}
	c = answerCall(b, "renew")
	c.TTLSeconds = 60
	if _, err = transitionAnswer(b, target, c, now, true); err == nil {
		t.Fatal("revoked owner renewed")
	}
}
func TestAnswerHandoffFailClosedControls(t *testing.T) {
	for _, test := range []string{"foreign-thread", "foreign-host", "foreign-generation", "stale-epoch", "stale-revision", "expired", "malformed-expiry", "missing-source", "same-target", "oversized-ttl"} {
		t.Run(test, func(t *testing.T) {
			a, lead, target, now := answerFixture()
			c := answerCall(a, "request")
			c.Target = target
			c.SourceRef = "source"
			c.TTLSeconds = 60
			switch test {
			case "foreign-thread":
				lead.ThreadID = "foreign"
			case "foreign-host":
				lead.Host = "foreign"
			case "foreign-generation":
				lead.Generation = "other"
			case "stale-epoch":
				c.ExpectedEpoch++
			case "stale-revision":
				c.ExpectedRevision++
			case "expired":
				now = now.Add(time.Minute)
			case "malformed-expiry":
				a.Expires = "unknown"
			case "missing-source":
				c.SourceRef = ""
			case "same-target":
				c.Target = lead
			case "oversized-ttl":
				c.TTLSeconds = 301
			}
			if _, err := transitionAnswer(a, lead, c, now, true); err == nil {
				t.Fatal("unsafe transition accepted")
			}
			if len(a.History) != 0 {
				t.Fatal("input history mutated")
			}
		})
	}
	a, lead, target, now := answerFixture()
	c := answerCall(a, "request")
	c.Target = target
	c.SourceRef = "source"
	c.TTLSeconds = 60
	a, _ = transitionAnswer(a, lead, c, now, true)
	c = answerCall(a, "activate")
	if _, e := transitionAnswer(a, target, c, now, true); e == nil {
		t.Fatal("no consent activated")
	}
	c = answerCall(a, "accept")
	a, _ = transitionAnswer(a, target, c, now, true)
	c = answerCall(a, "ack-drain")
	c.DrainRef = "proof"
	if _, e := transitionAnswer(a, lead, c, now, false); e == nil {
		t.Fatal("busy owner overridden")
	}
}
func TestAnswerHandoffGenericStateAccessRefused(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "handoff-generic.db"))
	defer s.Close()
	a, lead, _, _ := answerFixture()
	if e := s.CreateAnswerAssignment(a.Lane, lead, "source", 60); e != nil {
		t.Fatal(e)
	}
	key := answerStatePrefix + a.Lane
	if e := s.SetState(key, `{}`); e == nil {
		t.Fatal("generic SetState overwrote single-answerer authority")
	}
	if _, e := s.CompareAndSwapState(key, "", false, `{}`); e == nil {
		t.Fatal("generic CompareAndSwapState overwrote single-answerer authority")
	}
	if _, _, e := s.GetState(key); e == nil {
		t.Fatal("generic GetState inspected single-answerer candidate identity")
	}
}
func TestAnswerHandoffTransactionCASAndRollback(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "handoff.db"))
	defer s.Close()
	a, lead, target, now := answerFixture()
	s.now = func() time.Time { return now }
	if e := s.CreateAnswerAssignment(a.Lane, lead, "source", 60); e != nil {
		t.Fatal(e)
	}
	if e := s.CreateAnswerAssignment(a.Lane, target, "replacement", 60); e == nil {
		t.Fatal("management replaced owner")
	}
	c := answerCall(a, "request")
	c.Target = target
	c.SourceRef = "source"
	c.TTLSeconds = 60
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.applyAnswerHandoff(a.Lane, lead, c); e == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatal("CAS wins", wins)
	}
	var state AnswerAssignment
	if e := s.withAnswerState(a.Lane, func(_ *txHandle, a *AnswerAssignment) error { state = *a; return nil }); e != nil {
		t.Fatal(e)
	}
	c = answerCall(state, "accept")
	state, e := s.applyAnswerHandoff(a.Lane, target, c)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.exec(`CREATE TRIGGER reject_handoff BEFORE UPDATE ON state BEGIN SELECT RAISE(ABORT,'write refused'); END`); e != nil {
		t.Fatal(e)
	}
	c = answerCall(state, "ack-drain")
	c.DrainRef = "drained"
	if _, e = s.applyAnswerHandoff(a.Lane, lead, c); e == nil {
		t.Fatal("persistence error accepted")
	}
	if _, e = s.exec(`DROP TRIGGER reject_handoff`); e != nil {
		t.Fatal(e)
	}
	if e = s.withAnswerState(a.Lane, func(_ *txHandle, a *AnswerAssignment) error {
		if a.Drained {
			t.Fatal("rollback lost")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}
