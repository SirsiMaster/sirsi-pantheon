package routerstore

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDirectTaskClaimAcknowledgesPendingExactSource(t *testing.T) {
	for _, next := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "next"}[next], func(t *testing.T) {
			s := newTestStore(t)
			for _, v := range []struct{ a, id string }{{"lane", "A"}, {"lane", "B"}, {"other", "A"}} {
				if err := s.AddTask(Task{Agent: v.a, TaskID: v.id, Subject: "real work"}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.db.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,status,created,updated) VALUES('failure','failure','lane','ledger_task','A','old terminal failure','terminal_failed','2020-01-01T00:00:00Z','2020-01-01T00:00:00Z')`); err != nil {
				t.Fatal(err)
			}
			before, err := s.ListWakeEvents("lane")
			if err != nil {
				t.Fatal(err)
			}
			var l *TaskLease
			if next {
				l, err = s.ClaimNextTask("lane", "w", "thr", time.Minute)
			} else {
				l, err = s.ClaimTask("lane", "A", "w", "thr", time.Minute)
			}
			if err != nil {
				t.Fatal(err)
			}
			events, err := s.ListWakeEvents("lane")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, e := range events {
				if e.EventID == "failure" {
					if e.Status != "terminal_failed" || e.AckRef != "" {
						t.Fatal("terminal history changed")
					}
					continue
				}
				if e.SourceKind == "ledger_task" && e.SourceID == l.TaskID {
					found = true
					if e.Status != "acked" || e.AckRef != "task-lease:lane:"+l.TaskID+":"+l.Token || e.LeaseToken != "" || e.LeaseExpires != "" {
						t.Fatalf("direct exact source not ACKed: status=%s", e.Status)
					}
				} else {
					for _, b := range before {
						if b.EventID == e.EventID && (b.Status != e.Status || b.AckRef != e.AckRef) {
							t.Fatal("wrong source changed")
						}
					}
				}
			}
			if !found {
				t.Fatal("missing exact source ACK")
			}
			foreign, err := s.ListWakeEvents("other")
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range foreign {
				if e.Status != "pending" {
					t.Fatal("foreign source changed")
				}
			}
		})
	}
}

func TestRejectedTaskClaimDoesNotAcknowledgeWake(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "lane", TaskID: "blocked", Subject: "blocked", BlockedBy: "external-block"}); err != nil {
		t.Fatal(err)
	}
	// Explicitly emitted pending wake is a negative fixture for an ineligible task.
	if _, err := s.db.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,status,created,updated) VALUES('pending','pending','lane','ledger_task','blocked','fixture','pending','2020-01-01T00:00:00Z','2020-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimTask("lane", "blocked", "w", "thr", time.Minute); !errors.Is(err, ErrNoClaimableTask) {
		t.Fatalf("blocked claim error=%v", err)
	}
	if _, err := s.ClaimTask("lane", "missing", "w", "thr", time.Minute); !errors.Is(err, ErrNoClaimableTask) {
		t.Fatalf("missing claim error=%v", err)
	}
	events, err := s.ListWakeEvents("lane")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Status != "pending" || e.AckRef != "" {
			t.Fatal("failed claim ACKed")
		}
	}
}

func TestConcurrentDirectTaskClaimsOnlyWinnerAcknowledges(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "lane", TaskID: "A", Subject: "work"}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan *TaskLease, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			l, e := s.ClaimTask("lane", "A", "worker", "thread", time.Minute)
			results <- l
			errs <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	var winner *TaskLease
	wins := 0
	for l := range results {
		if l != nil {
			winner = l
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("claims=%d", wins)
	}
	for e := range errs {
		if e != nil && !errors.Is(e, ErrNoClaimableTask) {
			t.Fatal(e)
		}
	}
	events, err := s.ListWakeEvents("lane")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.SourceKind == "ledger_task" && e.SourceID == "A" && e.AckRef != "task-lease:lane:A:"+winner.Token {
			t.Fatal("ACK does not name winner")
		}
	}
}

func TestDirectTaskCompletionResolvesOlderLaneFailure(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 10, 20, 7, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.MarkRequirementAudit("lane", "audit://current"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,status,created,updated) VALUES('old','old','lane','reconcile','lane','failed','terminal_failed',?,?)`, now.Add(-time.Minute).Format(time.RFC3339), now.Add(-time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "lane", TaskID: "A", Subject: "real work"}); err != nil {
		t.Fatal(err)
	}
	l, err := s.ClaimTask("lane", "A", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if err = s.CompleteTaskLease("lane", "A", l.Token, "canonical://actual-result"); err != nil {
		t.Fatal(err)
	}
	state, err := s.ClassifyLane("lane", true, time.Minute)
	if err != nil || state.TerminalWakeFailures != 1 || state.UnresolvedWakeFailures != 0 || state.Classification != LaneComplete {
		t.Fatalf("real direct completion not recognized: %+v %v", state, err)
	}
}

func TestDirectTaskWakeAckFailureRollsBackClaim(t *testing.T) {
	s := newTestStore(t)
	if !s.dialect().hasPragmas {
		t.Skip("SQLite-specific injected ABORT; portable claim/ACK tests run on both backends")
	}
	if err := s.AddTask(Task{Agent: "lane", TaskID: "A", Subject: "work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_test_wake_ack BEFORE UPDATE OF status ON wake_events WHEN NEW.status='acked' BEGIN SELECT RAISE(ABORT,'injected ACK failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimTask("lane", "A", "worker", "thread", time.Minute); err == nil {
		t.Fatal("ACK failure returned successful claim")
	}
	task, err := s.GetTask("lane", "A")
	if err != nil || task.Status != "pending" {
		t.Fatalf("claim did not roll back: %+v %v", task, err)
	}
	var attempts int
	var token string
	if err = s.db.QueryRow(`SELECT attempts,lease_token FROM tasks WHERE agent='lane' AND task_id='A'`).Scan(&attempts, &token); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || token != "" {
		t.Fatal("failed ACK leaked claim/attempt")
	}
	events, err := s.ListWakeEvents("lane")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Status != "pending" || e.AckRef != "" {
			t.Fatal("failed ACK changed source wake")
		}
	}
}

func TestDirectReclaimPreservesEarlierAckEvidence(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "lane", TaskID: "A", Subject: "work"}); err != nil {
		t.Fatal(err)
	}
	first, err := s.ClaimTask("lane", "A", "w1", "thr1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ReleaseTaskLease("lane", "A", first.Token, "partial; original remains unfinished"); err != nil {
		t.Fatal(err)
	}
	second, err := s.ClaimTask("lane", "A", "w2", "thr2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token == first.Token {
		t.Fatal("new claim reused fence")
	}
	events, err := s.ListWakeEvents("lane")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.SourceKind == "ledger_task" && e.SourceID == "A" && e.AckRef != "task-lease:lane:A:"+first.Token {
			t.Fatal("earlier ACK evidence overwritten")
		}
	}
}
