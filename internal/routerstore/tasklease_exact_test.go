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
