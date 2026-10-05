package routerstore

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestClaimTaskSelectsExactIDAndRefusesIneligibleStates(t *testing.T) {
	s := newTestStore(t)
	for _, task := range []Task{
		{Agent: "codex-home", TaskID: "older", Subject: "older"},
		{Agent: "codex-home", TaskID: "exact", Subject: "exact"},
		{Agent: "codex-home", TaskID: "blocked", Subject: "blocked", Status: "blocked"},
		{Agent: "codex-home", TaskID: "done", Subject: "done", Status: "done"},
	} {
		if err := s.AddTask(task); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := s.ClaimTask("codex-home", "exact", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if lease.TaskID != "exact" || lease.Attempt != 1 {
		t.Fatalf("exact claim selected wrong task: %+v", lease)
	}
	for _, taskID := range []string{"blocked", "done", "missing"} {
		if _, err := s.ClaimTask("codex-home", taskID, "worker", "thread", time.Minute); !errors.Is(err, ErrNoClaimableTask) {
			t.Fatalf("exact claim of %q = %v, want ErrNoClaimableTask", taskID, err)
		}
	}
}

func TestClaimRetriesReturnTheExistingLiveLease(t *testing.T) {
	s := newTestStore(t)
	for _, taskID := range []string{"a-first", "b-next"} {
		if err := s.AddTask(Task{Agent: "codex-home", TaskID: taskID, Subject: taskID}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatalf("initial ClaimNextTask: %v", err)
	}
	if first.TaskID != "a-first" || first.Attempt != 1 {
		t.Fatalf("initial lease = %+v, want first task attempt 1", first)
	}

	for _, retry := range []struct {
		name  string
		claim func() (*TaskLease, error)
	}{
		{name: "next-task retry", claim: func() (*TaskLease, error) {
			return s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
		}},
		{name: "exact-task retry", claim: func() (*TaskLease, error) {
			return s.ClaimTask("codex-home", "a-first", "worker", "thread", time.Minute)
		}},
	} {
		t.Run(retry.name, func(t *testing.T) {
			got, err := retry.claim()
			if err != nil {
				t.Fatalf("retry claim: %v", err)
			}
			if got.TaskID != first.TaskID || got.Token != first.Token || got.Attempt != first.Attempt || !got.Expires.Equal(first.Expires) {
				t.Fatalf("retry lease = %+v, want original lease %+v", got, first)
			}
		})
	}

	next, err := s.ClaimNextTask("codex-home", "other-worker", "other-thread", time.Minute)
	if err != nil {
		t.Fatalf("claim remaining task: %v", err)
	}
	if next.TaskID != "b-next" || next.Attempt != 1 {
		t.Fatalf("remaining lease = %+v, want b-next attempt 1", next)
	}
}

func TestClaimRetryAfterExpiryCreatesANewLeaseAttempt(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "expiring", Subject: "expiring"}); err != nil {
		t.Fatal(err)
	}

	first, err := s.ClaimTask("codex-home", "expiring", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatalf("initial claim: %v", err)
	}
	now = first.Expires.Add(time.Second)

	reclaimed, err := s.ClaimTask("codex-home", "expiring", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatalf("claim after expiry: %v", err)
	}
	if reclaimed.Token == first.Token || reclaimed.Attempt != first.Attempt+1 || !reclaimed.Expires.After(now) {
		t.Fatalf("expired lease was reused instead of reclaimed: first=%+v reclaimed=%+v", first, reclaimed)
	}
	if err := s.ReleaseTaskLease(first.Agent, first.TaskID, first.Token, "stale owner"); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("expired owner release = %v, want ErrLeaseInvalid", err)
	}
}

func TestClaimTaskContentionHasOneWinner(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "exact", Subject: "exact"}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.ClaimTask("codex-home", "exact", "worker", "thread", time.Minute)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	wins, losses := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ErrNoClaimableTask):
			losses++
		default:
			t.Fatalf("unexpected contention error: %v", err)
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatalf("contention wins=%d losses=%d, want one each", wins, losses)
	}
}

func TestClaimTaskUsesTTLAndRetryCeiling(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "exact", Subject: "exact"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimTask("codex-home", "exact", "worker", "thread", MaxLeaseTTL+time.Second); err == nil {
		t.Fatal("exact claim accepted TTL above the shared ceiling")
	}
	for attempt := 1; attempt <= MaxRetriesPerItem; attempt++ {
		lease, err := s.ClaimTask("codex-home", "exact", "worker", "thread", time.Minute)
		if err != nil || lease.Attempt != attempt {
			t.Fatalf("attempt %d: lease=%+v err=%v", attempt, lease, err)
		}
		now = now.Add(2 * time.Minute)
	}
	if _, err := s.ClaimTask("codex-home", "exact", "worker", "thread", time.Minute); !errors.Is(err, ErrNoClaimableTask) {
		t.Fatalf("exact claim bypassed retry ceiling: %v", err)
	}
	got, err := s.GetTask("codex-home", "exact")
	if err != nil || got.Status != "blocked" {
		t.Fatalf("exhausted exact task not blocked: %+v err=%v", got, err)
	}
}
