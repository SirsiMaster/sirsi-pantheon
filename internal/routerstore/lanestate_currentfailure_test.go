package routerstore

import (
	"testing"
	"time"
)

func insertLaneFailure(t *testing.T, s *SQLiteStore, id, stamp string) {
	t.Helper()
	_, err := s.db.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,status,created,updated) VALUES(?,?,'codex-home','reconcile','codex-home','failed','terminal_failed',?,?)`, id, id, stamp, stamp)
	if err != nil {
		t.Fatal(err)
	}
}
func TestClassifyLaneNewFencedCompletionResolvesOnlyOlderFailure(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	s.now = func() time.Time { return now }
	if err := s.MarkRequirementAudit("codex-home", "audit://current"); err != nil {
		t.Fatal(err)
	}
	insertLaneFailure(t, s, "old", now.Add(-time.Minute).Format(time.RFC3339))
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "proof", Subject: "real work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimWakeEventFor("codex-home", time.Minute); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if err := s.CompleteTaskLease("codex-home", "proof", lease.Token, "canonical://actual-result"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "remaining", Subject: "remaining work"}); err != nil {
		t.Fatal(err)
	}
	state, err := s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil || state.Classification != LaneIdleWithWork || state.TerminalWakeFailures != 1 || state.UnresolvedWakeFailures != 0 {
		t.Fatalf("new completion did not resolve current failure/history lost: %+v %v", state, err)
	}
	// A metadata touch is not a later completion. The durable claim lower bound
	// prevents it clearing this new failure even though updated moves forward.
	insertLaneFailure(t, s, "new", now.Add(time.Second).Format(time.RFC3339))
	now = now.Add(time.Hour)
	if _, err := s.UpdateTask("codex-home", "proof", TaskUpdate{Subject: "metadata only"}); err != nil {
		t.Fatal(err)
	}
	state, err = s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil || state.Classification != LaneUnroutable || state.TerminalWakeFailures != 2 || state.UnresolvedWakeFailures != 1 {
		t.Fatalf("newer failure/metadata falsely cleared: %+v %v", state, err)
	}
}
func TestClassifyLaneEqualUnknownFailuresConservative(t *testing.T) {
	for _, stamp := range []string{"equal", "malformed", ""} {
		t.Run(stamp, func(t *testing.T) {
			s := newTestStore(t)
			now := time.Now().UTC().Truncate(time.Second)
			s.now = func() time.Time { return now }
			if err := s.MarkRequirementAudit("codex-home", "audit://current"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Send("owner", "codex-home", "work", "review", "do work"); err != nil {
				t.Fatal(err)
			}
			lease, err := s.ClaimNext("codex-home", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Complete(lease.ItemID, lease.Token, "actual durable result"); err != nil {
				t.Fatal(err)
			}
			failed := stamp
			if stamp == "equal" {
				failed = now.Format(time.RFC3339)
			}
			insertLaneFailure(t, s, "failed", failed)
			state, err := s.ClassifyLane("codex-home", true, time.Minute)
			if err != nil || state.Classification != LaneBlocked || state.UnresolvedWakeFailures != 1 || state.TerminalWakeFailures != 1 {
				t.Fatalf("equal/unknown failure falsely COMPLETE: %+v %v", state, err)
			}
		})
	}
}
func TestClassifyLaneHeartbeatAndMalformedSuccessDoNotResolve(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	s.now = func() time.Time { return now }
	if err := s.MarkRequirementAudit("codex-home", "audit://current"); err != nil {
		t.Fatal(err)
	}
	insertLaneFailure(t, s, "failure", now.Add(-time.Minute).Format(time.RFC3339))
	// Fresh store-owned thread/heartbeat payload is deliberately not success.
	if err := s.UpsertThreads([]ThreadRecord{{ThreadID: "heartbeat-only", Agent: "codex-home", Status: "active", LastSeenAt: now.Add(time.Hour).Format(time.RFC3339), Payload: []byte(`{"heartbeat":true}`), Host: "test"}}); err != nil {
		t.Fatal(err)
	}
	// No completion: time/fresh metadata alone do not resolve routing failure.
	now = now.Add(time.Hour)
	state, err := s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil || state.Classification != LaneBlocked || state.UnresolvedWakeFailures != 1 {
		t.Fatalf("clock/heartbeat availability falsely cleared: %+v %v", state, err)
	}
	if _, err = s.Send("owner", "codex-home", "work", "review", "do work"); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Complete(lease.ItemID, lease.Token, "real result"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE items SET closed='bad' WHERE id=?`, lease.ItemID); err != nil {
		t.Fatal(err)
	}
	state, err = s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil || state.UnresolvedWakeFailures != 1 {
		t.Fatalf("malformed completion cleared failure: %+v %v", state, err)
	}
}

func TestClassifyLaneIntegrityFailureStillBlocksRunnableLane(t *testing.T) {
	s := newTestStore(t)
	if err := s.MarkRequirementAudit("codex-home", "audit://current"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "work", Subject: "work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO requirements(req_id,owner,source,source_ref,title,status,created,updated) VALUES('invalid','codex-home','ADR-061','R1','missing proof','satisfied',?,?)`, s.clock().UTC().Format(time.RFC3339), s.clock().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	state, err := s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil || state.CompletionIntegrityFailures == 0 || state.Classification != LaneBlocked {
		t.Fatalf("integrity failure hidden: %+v %v", state, err)
	}
	// Historical debt must not hide a real current fenced worker. The debt
	// counter remains visible while current work has precedence.
	lease, err := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	state, err = s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil || state.Classification != LaneWorking || state.CompletionIntegrityFailures == 0 {
		t.Fatalf("historical integrity debt hid live fenced work: %+v %v", state, err)
	}
	if err := s.CompleteTaskLease("codex-home", "work", lease.Token, "canonical://live-work"); err != nil {
		t.Fatal(err)
	}
	state, err = s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil || state.Classification != LaneBlocked || state.CompletionIntegrityFailures == 0 {
		t.Fatalf("no-work integrity debt falsely became COMPLETE: %+v %v", state, err)
	}
}
