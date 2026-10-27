package routerstore

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func resumeBarrierPrincipal(t *testing.T, s *SQLiteStore, agent string) context.Context {
	t.Helper()
	threadID := "resume-thread-" + agent
	host, runtime := "resume-host-"+agent, "resume-runtime"
	if err := s.UpsertThreads([]ThreadRecord{{
		ThreadID:   threadID,
		Agent:      agent,
		Host:       host,
		Status:     "active",
		LastSeenAt: s.clock().UTC().Format(time.RFC3339Nano),
		Payload:    []byte(`{"surface":"worker","answer_role":false}`),
	}}); err != nil {
		t.Fatal(err)
	}
	session, err := s.MintSessionForThread(host, agent, runtime, threadID)
	if err != nil {
		t.Fatal(err)
	}
	actor := taskEvidenceActor{Agent: agent, Host: host, ThreadID: threadID, Runtime: runtime, SessionID: session.ID}
	return context.WithValue(context.Background(), taskEvidenceActorKey{}, actor)
}

func addResumeBarrierTask(t *testing.T, s *SQLiteStore, agent, taskID string) {
	t.Helper()
	if err := s.AddTask(Task{Agent: agent, TaskID: taskID, Subject: "resume fixture", Status: "blocked"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.exec(`UPDATE tasks SET attempts=? WHERE agent=? AND task_id=?`, MaxRetriesPerItem, agent, taskID); err != nil {
		t.Fatal(err)
	}
	wakeID := "resume-wake-" + agent + "-" + taskID
	now := s.clock().UTC().Format(time.RFC3339)
	if _, err := s.exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,status,created,updated) VALUES(?,?,?,?,?,?,?,?,?)`, wakeID, wakeID, agent, "ledger_task", taskID, "fixture", "pending", now, now); err != nil {
		t.Fatal(err)
	}
}

func resumeBarrierApplyRequest(t *testing.T, s *SQLiteStore, ctx context.Context, agent, taskID string) TaskResumeRequest {
	t.Helper()
	inspect := TaskResumeRequest{Version: 1, Action: "inspect", Agent: agent, TaskID: taskID}
	out, err := s.ResumeTaskClaim(ctx, inspect)
	if err != nil || out.Claimed || out.Lease != nil || !taskEvidenceDigest(out.RowVersion) {
		t.Fatalf("inspect %s/%s: %+v %v", agent, taskID, out, err)
	}
	return TaskResumeRequest{
		Version: 1, Action: "claim", Agent: agent, TaskID: taskID,
		ExpectedVersion: out.RowVersion, EvidenceRef: "evidence://cause-fixed",
		EvidenceSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Reason:         "fixture cause and evidence reviewed", TTLSeconds: 60,
	}
}

func resumeBarrierWakeSnapshot(t *testing.T, s *SQLiteStore, agent, taskID string) (status, ack string, attempts int) {
	t.Helper()
	wakeID := "resume-wake-" + agent + "-" + taskID
	if err := s.db.QueryRow(`SELECT status,ack_ref,attempts FROM wake_events WHERE event_key=?`, wakeID).Scan(&status, &ack, &attempts); err != nil {
		t.Fatal(err)
	}
	return status, ack, attempts
}

func resumeBarrierLeaseLogCount(t *testing.T, s *SQLiteStore, agent, taskID string) int {
	t.Helper()
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM task_lease_log WHERE agent=? AND task_id=? AND op='resume-claim'`, agent, taskID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestResumeTaskClaimHonorsGemmaPauseWithoutBlockingInspectOrReplay(t *testing.T) {
	s := newTestStore(t)
	gemmaCtx := resumeBarrierPrincipal(t, s, "gemma-pantheon")
	otherCtx := resumeBarrierPrincipal(t, s, "fixture-other-lane")
	addResumeBarrierTask(t, s, "gemma-pantheon", "fresh-task")
	addResumeBarrierTask(t, s, "gemma-pantheon", "replay-task")
	addResumeBarrierTask(t, s, "fixture-other-lane", "other-task")

	// A previously issued exact lease must remain replayable during the pause.
	replayReq := resumeBarrierApplyRequest(t, s, gemmaCtx, "gemma-pantheon", "replay-task")
	prior, err := s.ResumeTaskClaim(gemmaCtx, replayReq)
	if err != nil || !prior.Claimed || prior.Lease == nil {
		t.Fatalf("initial replay-fixture claim: %+v %v", prior, err)
	}
	freshReq := resumeBarrierApplyRequest(t, s, gemmaCtx, "gemma-pantheon", "fresh-task")
	otherReq := resumeBarrierApplyRequest(t, s, otherCtx, "fixture-other-lane", "other-task")

	paused, err := s.LaneClaimBarrier(barrierCtx(), LaneClaimBarrierRequest{
		Action: "acquire", Lane: "gemma-pantheon", Token: "0123456789abcdef0123456789abcdef", // gitleaks:allow -- fixed synthetic capability in isolated store fixture only
	})
	if err != nil || !paused.Paused {
		t.Fatalf("acquire Gemma pause: %+v %v", paused, err)
	}
	before := taskEvidenceRaw(t, s, freshReq.Agent, freshReq.TaskID)
	wakeStatus, wakeAck, wakeAttempts := resumeBarrierWakeSnapshot(t, s, freshReq.Agent, freshReq.TaskID)
	auditsBefore := taskResumeAuditCount(t, s)
	leaseLogsBefore := resumeBarrierLeaseLogCount(t, s, freshReq.Agent, freshReq.TaskID)
	if auditsBefore != 1 {
		t.Fatalf("resume audits before paused apply = %d, want existing replay audit only", auditsBefore)
	}
	if leaseLogsBefore != 0 {
		t.Fatalf("fresh task has %d resume lease logs before paused apply", leaseLogsBefore)
	}

	inspect, err := s.ResumeTaskClaim(gemmaCtx, TaskResumeRequest{Version: 1, Action: "inspect", Agent: freshReq.Agent, TaskID: freshReq.TaskID})
	if err != nil || inspect.Claimed || inspect.Lease != nil || inspect.Attempts != MaxRetriesPerItem || inspect.RowVersion != freshReq.ExpectedVersion {
		t.Fatalf("paused inspection changed custody: %+v %v", inspect, err)
	}
	if _, err := s.ResumeTaskClaim(gemmaCtx, freshReq); !errors.Is(err, ErrLanePaused) {
		t.Fatalf("paused fresh resume claim error = %v, want ErrLanePaused", err)
	}
	if got := taskEvidenceRaw(t, s, freshReq.Agent, freshReq.TaskID); !reflect.DeepEqual(before, got) {
		t.Fatal("paused resume changed task custody or attempts")
	}
	if taskResumeAuditCount(t, s) != auditsBefore {
		t.Fatal("paused resume recorded a new audit")
	}
	if leaseLogs := resumeBarrierLeaseLogCount(t, s, freshReq.Agent, freshReq.TaskID); leaseLogs != leaseLogsBefore {
		t.Fatalf("paused resume lease log count = %d, want %d", leaseLogs, leaseLogsBefore)
	}
	if status, ack, attempts := resumeBarrierWakeSnapshot(t, s, freshReq.Agent, freshReq.TaskID); status != wakeStatus || ack != wakeAck || attempts != wakeAttempts {
		t.Fatalf("paused resume changed wake event: %q %q %d", status, ack, attempts)
	}
	if session, err := s.TaskSession(freshReq.Agent, freshReq.TaskID); err != nil || session != "" {
		t.Fatalf("paused resume bound a session: %q %v", session, err)
	}

	replay, err := s.ResumeTaskClaim(gemmaCtx, replayReq)
	if err != nil || !replay.Repeated || replay.Lease == nil || replay.Lease.Token != prior.Lease.Token {
		t.Fatalf("existing exact replay failed while paused: %+v %v", replay, err)
	}
	other, err := s.ResumeTaskClaim(otherCtx, otherReq)
	if err != nil || !other.Claimed || other.Lease == nil {
		t.Fatalf("unrelated lane blocked by Gemma pause: %+v %v", other, err)
	}
	if _, err := s.LaneClaimBarrier(barrierCtx(), LaneClaimBarrierRequest{
		Action: "release", Lane: "gemma-pantheon", Token: paused.Token,
	}); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.ResumeTaskClaim(gemmaCtx, freshReq)
	if err != nil || !resumed.Claimed || resumed.Lease == nil || resumed.Attempts != MaxRetriesPerItem+1 {
		t.Fatalf("unpaused fresh resume claim failed: %+v %v", resumed, err)
	}
}
