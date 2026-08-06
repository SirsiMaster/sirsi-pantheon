package routerstore

import (
	"testing"
	"time"
)

func TestClassifyLaneHonestStates(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	agent := "codex-home"
	if err := s.MarkRequirementAudit(agent, "audit://complete"); err != nil {
		t.Fatal(err)
	}
	assertLane := func(wake bool, want string) {
		t.Helper()
		got, err := s.ClassifyLane(agent, wake, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if got.Classification != want {
			t.Fatalf("classification=%s want=%s state=%+v", got.Classification, want, got)
		}
	}
	assertLane(true, LaneComplete)

	if err := s.AddTask(Task{Agent: agent, TaskID: "T", Subject: "work"}); err != nil {
		t.Fatal(err)
	}
	assertLane(false, LaneUnroutable)
	assertLane(true, LaneIdleWithWork)

	lease, err := s.ClaimNextTask(agent, "worker", "thread", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	assertLane(true, LaneWorking)

	now = now.Add(2 * time.Minute) // lease valid, mutation no longer recent
	assertLane(true, LaneAssigned)
	if err := s.CompleteTaskLease(agent, "T", lease.Token, "proof://T"); err != nil {
		t.Fatal(err)
	}
	assertLane(true, LaneComplete)
}

func TestClassifyLaneBlockedNotComplete(t *testing.T) {
	s := newTestStore(t)
	if err := s.MarkRequirementAudit("codex-home", "audit://complete"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "B", Subject: "blocked", BlockedBy: "security-review"}); err != nil {
		t.Fatal(err)
	}
	state, err := s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if state.Classification != LaneBlocked {
		t.Fatalf("blocked work misclassified: %+v", state)
	}
}

func TestClassifyLaneIgnoresSessionLiveness(t *testing.T) {
	s := newTestStore(t)
	if err := s.MarkRequirementAudit("codex-home", "audit://complete"); err != nil {
		t.Fatal(err)
	}
	state, err := s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if state.Classification != LaneComplete || state.LiveLeases != 0 {
		t.Fatalf("no work/leases must be COMPLETE regardless of any external heartbeat: %+v", state)
	}
}

func TestClassifyLaneNeverReportsCompleteWithTerminalWakeFailure(t *testing.T) {
	s := newTestStore(t)
	if err := s.MarkRequirementAudit("codex-home", "audit://complete"); err != nil {
		t.Fatal(err)
	}
	now := s.clock().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,status,created,updated) VALUES('e','e','codex-home','reconcile','codex-home','failed','terminal_failed',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	state, err := s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if state.Classification == LaneComplete {
		t.Fatalf("terminal wake failure falsely rendered COMPLETE: %+v", state)
	}
}

func TestOldWorkingItemLeaseIsAssignedNotWorking(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.MarkRequirementAudit("codex-home", "audit://complete"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send("owner", "codex-home", "work", "review", "execute"); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNext("codex-home", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StartWork(lease.ItemID, lease.Token); err != nil {
		t.Fatal(err)
	}
	state, err := s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil || state.Classification != LaneWorking {
		t.Fatalf("fresh item mutation=%+v err=%v", state, err)
	}
	now = now.Add(2 * time.Minute)
	state, err = s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if state.Classification != LaneAssigned {
		t.Fatalf("old working item falsely remained WORKING: %+v", state)
	}
}
