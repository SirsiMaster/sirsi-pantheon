package routerstore

import (
	"errors"
	"testing"
	"time"
)

func TestTaskLeaseClaimRenewComplete(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if ifErr1 := s.AddTask(Task{Agent: "codex-home", TaskID: "R1", Subject: "runtime"}); ifErr1 != nil {
		t.Fatal(ifErr1)
	}
	lease, err := s.ClaimNextTask("codex-home", "worker-1", "thread-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Attempt != 1 || lease.ThreadID != "thread-1" || lease.Token == "" {
		t.Fatalf("bad lease: %+v", lease)
	}
	if _, ifErr2 := s.ClaimNextTask("codex-home", "worker-2", "thread-2", time.Minute); !errors.Is(ifErr2, ErrNoWork) {
		t.Fatalf("double claim must fail, got %v", ifErr2)
	}
	now = now.Add(30 * time.Second)
	if ifErr3 := s.RenewTaskLease(lease.Agent, lease.TaskID, lease.Token, 2*time.Minute); ifErr3 != nil {
		t.Fatal(ifErr3)
	}
	if ifErr4 := s.CompleteTaskLease(lease.Agent, lease.TaskID, lease.Token, "proof://R1"); ifErr4 != nil {
		t.Fatal(ifErr4)
	}
	got, err := s.GetTask("codex-home", "R1")
	if err != nil || got.Status != "done" {
		t.Fatalf("task not completed: %+v err=%v", got, err)
	}
}

func TestTaskLeaseExpiryReclaimsAndFencesOldWorker(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if ifErr5 := s.AddTask(Task{Agent: "codex-home", TaskID: "R2", Subject: "runtime"}); ifErr5 != nil {
		t.Fatal(ifErr5)
	}
	old, _ := s.ClaimNextTask("codex-home", "worker-1", "thread-1", time.Minute)
	now = now.Add(2 * time.Minute)
	fresh, err := s.ClaimNextTask("codex-home", "worker-2", "thread-2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Attempt != 2 || fresh.Token == old.Token {
		t.Fatalf("expiry must produce a new fenced attempt: old=%+v fresh=%+v", old, fresh)
	}
	if ifErr6 := s.CompleteTaskLease(old.Agent, old.TaskID, old.Token, "stale-proof"); !errors.Is(ifErr6, ErrLeaseInvalid) {
		t.Fatalf("expired worker completed newer lease: %v", ifErr6)
	}
	if ifErr7 := s.CompleteTaskLease(fresh.Agent, fresh.TaskID, fresh.Token, "proof://R2"); ifErr7 != nil {
		t.Fatal(ifErr7)
	}
}

func TestTaskLeaseCompletionRequiresEvidence(t *testing.T) {
	s := newTestStore(t)
	if ifErr8 := s.AddTask(Task{Agent: "codex-home", TaskID: "R3", Subject: "runtime"}); ifErr8 != nil {
		t.Fatal(ifErr8)
	}
	lease, _ := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
	if ifErr9 := s.CompleteTaskLease(lease.Agent, lease.TaskID, lease.Token, " "); ifErr9 == nil {
		t.Fatal("completion without evidence accepted")
	}
}

func TestTaskLeaseRetryExhaustionDoesNotForgeDependency(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if ifErr10 := s.AddTask(Task{Agent: "codex-home", TaskID: "R4", Subject: "runtime"}); ifErr10 != nil {
		t.Fatal(ifErr10)
	}
	for i := 0; i < MaxRetriesPerItem; i++ {
		lease, err := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if ifErr11 := s.ReleaseTaskLease(lease.Agent, lease.TaskID, lease.Token, "adapter failed"); ifErr11 != nil {
			t.Fatal(ifErr11)
		}
		now = now.Add(time.Second)
	}
	var status, blockedBy, failureReason string
	if ifErr12 := s.db.QueryRow(`SELECT status,blocked_by,failure_reason FROM tasks WHERE agent='codex-home' AND task_id='R4';`).Scan(&status, &blockedBy, &failureReason); ifErr12 != nil {
		t.Fatal(ifErr12)
	}
	if status != "blocked" || blockedBy != "" || failureReason == "" {
		t.Fatalf("retry exhaustion corrupted dependency semantics: status=%q blocked_by=%q failure=%q", status, blockedBy, failureReason)
	}
}

func TestTaskLeaseCrashExpiryStopsAtRetryCeiling(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if ifErr13 := s.AddTask(Task{Agent: "codex-home", TaskID: "R5", Subject: "runtime"}); ifErr13 != nil {
		t.Fatal(ifErr13)
	}
	for i := 0; i < MaxRetriesPerItem; i++ {
		if _, ifErr14 := s.ClaimNextTask("codex-home", "crashing-worker", "thread", time.Minute); ifErr14 != nil {
			t.Fatal(ifErr14)
		}
		now = now.Add(2 * time.Minute)
	}
	if _, ifErr15 := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute); !errors.Is(ifErr15, ErrNoWork) {
		t.Fatalf("crash loop remained claimable after retry ceiling: %v", ifErr15)
	}
	var status, reason string
	if ifErr16 := s.db.QueryRow(`SELECT status,failure_reason FROM tasks WHERE agent='codex-home' AND task_id='R5'`).Scan(&status, &reason); ifErr16 != nil {
		t.Fatal(ifErr16)
	}
	if status != "blocked" || reason == "" {
		t.Fatalf("expired crash loop was not escalated: status=%q reason=%q", status, reason)
	}
}
