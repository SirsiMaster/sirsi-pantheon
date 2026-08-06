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
	if ifErr1 := s.MarkRequirementAudit(agent, "audit://complete"); ifErr1 != nil {
		t.Fatal(ifErr1)
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

	if ifErr2 := s.AddTask(Task{Agent: agent, TaskID: "T", Subject: "work"}); ifErr2 != nil {
		t.Fatal(ifErr2)
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
	if ifErr3 := s.CompleteTaskLease(agent, "T", lease.Token, "proof://T"); ifErr3 != nil {
		t.Fatal(ifErr3)
	}
	assertLane(true, LaneComplete)
}

func TestClassifyLaneBlockedNotComplete(t *testing.T) {
	s := newTestStore(t)
	if ifErr4 := s.MarkRequirementAudit("codex-home", "audit://complete"); ifErr4 != nil {
		t.Fatal(ifErr4)
	}
	if ifErr5 := s.AddTask(Task{Agent: "codex-home", TaskID: "B", Subject: "blocked", BlockedBy: "security-review"}); ifErr5 != nil {
		t.Fatal(ifErr5)
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
	if ifErr6 := s.MarkRequirementAudit("codex-home", "audit://complete"); ifErr6 != nil {
		t.Fatal(ifErr6)
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
	if ifErr7 := s.MarkRequirementAudit("codex-home", "audit://complete"); ifErr7 != nil {
		t.Fatal(ifErr7)
	}
	now := s.clock().UTC().Format(time.RFC3339)
	if _, ifErr8 := s.db.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,status,created,updated) VALUES('e','e','codex-home','reconcile','codex-home','failed','terminal_failed',?,?)`, now, now); ifErr8 != nil {
		t.Fatal(ifErr8)
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
	if ifErr9 := s.MarkRequirementAudit("codex-home", "audit://complete"); ifErr9 != nil {
		t.Fatal(ifErr9)
	}
	if _, ifErr10 := s.Send("owner", "codex-home", "work", "review", "execute"); ifErr10 != nil {
		t.Fatal(ifErr10)
	}
	lease, err := s.ClaimNext("codex-home", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ifErr11 := s.StartWork(lease.ItemID, lease.Token); ifErr11 != nil {
		t.Fatal(ifErr11)
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
