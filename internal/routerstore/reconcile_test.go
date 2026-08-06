package routerstore

import (
	"testing"
	"time"
)

func TestReconcileCreatesTaskForRequirementAndRejectsFalseDone(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	s.now = func() time.Time { return now }
	req, err := s.AddRequirement("event supervisor", "ADR-057", "R4", "codex-home")
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.ReconcileOperationalState("codex-home", true)
	if err != nil {
		t.Fatal(err)
	}
	if report.RequirementTasksCreated != 1 {
		t.Fatalf("missing requirement task repair: %+v", report)
	}
	if _, err := s.db.Exec(`UPDATE tasks SET status='done',result_ref='' WHERE agent=? AND task_id=?`, "codex-home", "requirement/"+req.ID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	report, err = s.ReconcileOperationalState("codex-home", true)
	if err != nil {
		t.Fatal(err)
	}
	if report.FalseDoneRejected != 1 {
		t.Fatalf("false done not rejected: %+v", report)
	}
	task, _ := s.GetTask("codex-home", "requirement/"+req.ID)
	if task.Status != "in-progress" {
		t.Fatalf("false done survived: %+v", task)
	}
}

func TestReconcileExpiresOrphanTaskAndWakeLeases(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	s.now = func() time.Time { return now }
	if err := s.MarkRequirementAudit("codex-home", "audit://empty"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "T", Subject: "work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimWakeEvent(time.Minute); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	report, err := s.ReconcileOperationalState("codex-home", true)
	if err != nil {
		t.Fatal(err)
	}
	if report.ExpiredTaskLeases != 1 || report.ExpiredWakeLeases != 1 {
		t.Fatalf("orphan leases not expired: %+v", report)
	}
}
