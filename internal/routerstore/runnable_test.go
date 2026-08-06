package routerstore

import (
	"testing"
	"time"
)

func TestRunnableForThreeSourcesAndAuditedEmpty(t *testing.T) {
	s := newTestStore(t)
	agent := "codex-home"

	// No rows is not proof of completion: canon may simply never have been
	// enumerated. The audit itself is runnable work until evidence is recorded.
	state, err := s.RunnableFor(agent)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Runnable || !state.RequirementAuditNeeded {
		t.Fatalf("unaudited empty registry must remain runnable: %+v", state)
	}
	if ifErr1 := s.MarkRequirementAudit(agent, "audit://ADR-061/REQ-map"); ifErr1 != nil {
		t.Fatal(ifErr1)
	}
	state, _ = s.RunnableFor(agent)
	if state.Runnable {
		t.Fatalf("audited empty three-source state must be parkable: %+v", state)
	}

	itemID, err := s.Send("owner", agent, "work", "review", "do it")
	if err != nil {
		t.Fatal(err)
	}
	state, _ = s.RunnableFor(agent)
	if !state.Runnable || state.OpenRouterItems != 1 {
		t.Fatalf("open router item must make lane runnable: %+v", state)
	}
	if ifErr2 := s.CloseItem(itemID, "done"); ifErr2 != nil {
		t.Fatal(ifErr2)
	}

	if ifErr3 := s.AddTask(Task{Agent: agent, TaskID: "T1", Subject: "ledger work"}); ifErr3 != nil {
		t.Fatal(ifErr3)
	}
	state, _ = s.RunnableFor(agent)
	if !state.Runnable || state.ActionableLedgerTasks != 1 {
		t.Fatalf("actionable task must make lane runnable: %+v", state)
	}
	lease, err := s.ClaimNextTask(agent, "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	state, _ = s.RunnableFor(agent)
	if !state.Runnable || state.ActionableLedgerTasks != 1 || state.ClaimableLedgerTasks != 0 || state.LeasedLedgerTasks != 1 {
		t.Fatalf("leased work must remain runnable but not claimable: %+v", state)
	}
	reconcile, err := s.ReconcileOperationalState(agent, true)
	if err != nil {
		t.Fatal(err)
	}
	if reconcile.WakeEventsCreated != 0 {
		t.Fatalf("leased task caused duplicate wake backfill: %+v", reconcile)
	}
	if ifErr4 := s.CompleteTaskLease(agent, "T1", lease.Token, "proof://T1"); ifErr4 != nil {
		t.Fatal(ifErr4)
	}

	req, err := s.AddRequirement("runtime invariant", "ADR-061", "step 3", agent)
	if err != nil {
		t.Fatal(err)
	}
	state, _ = s.RunnableFor(agent)
	if !state.Runnable || state.UnmetRequirements != 1 {
		t.Fatalf("unmet requirement must make lane runnable: %+v", state)
	}
	decision, _ := s.Send("owner", agent, "waive", "decision", "supersede")
	if ifErr5 := s.CloseItem(decision, "owner approved supersession"); ifErr5 != nil {
		t.Fatal(ifErr5)
	}
	if ifErr6 := s.Waive(req.ID, "owner-ratified supersession", decision); ifErr6 != nil {
		t.Fatal(ifErr6)
	}
	state, _ = s.RunnableFor(agent)
	if state.Runnable {
		t.Fatalf("all three audited sources empty must permit park: %+v", state)
	}
}

func TestRunnableForExcludesBlockedWork(t *testing.T) {
	s := newTestStore(t)
	agent := "codex-home"
	if ifErr7 := s.MarkRequirementAudit(agent, "audit://empty"); ifErr7 != nil {
		t.Fatal(ifErr7)
	}
	dependency, err := s.Send("owner", "other", "dependency", "review", "first")
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := s.Send("owner", agent, "dependent", "review", "later")
	if err != nil {
		t.Fatal(err)
	}
	if ifErr8 := s.SetBlockedBy(itemID, dependency); ifErr8 != nil {
		t.Fatal(ifErr8)
	}
	if ifErr9 := s.AddTask(Task{Agent: agent, TaskID: "blocked", Subject: "wait", BlockedBy: "external-decision"}); ifErr9 != nil {
		t.Fatal(ifErr9)
	}
	state, err := s.RunnableFor(agent)
	if err != nil {
		t.Fatal(err)
	}
	if state.Runnable || state.OpenRouterItems != 0 || state.ActionableLedgerTasks != 0 {
		t.Fatalf("blocked work is not actionable: %+v", state)
	}
	if ifErr10 := s.CloseItem(dependency, "resolved"); ifErr10 != nil {
		t.Fatal(ifErr10)
	}
	state, _ = s.RunnableFor(agent)
	if !state.Runnable || state.OpenRouterItems != 1 {
		t.Fatalf("terminal dependency must unblock router item: %+v", state)
	}
}

func TestDeadLetteredDependencyDoesNotReleaseDependentWork(t *testing.T) {
	s := newTestStore(t)
	agent := "codex-home"
	if ifErr11 := s.MarkRequirementAudit(agent, "audit://empty"); ifErr11 != nil {
		t.Fatal(ifErr11)
	}
	dependency, err := s.Send("owner", "other", "dependency", "review", "first")
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := s.Send("owner", agent, "dependent", "review", "second")
	if err != nil {
		t.Fatal(err)
	}
	if ifErr12 := s.SetBlockedBy(dependent, dependency); ifErr12 != nil {
		t.Fatal(ifErr12)
	}
	if _, ifErr13 := s.db.Exec(`UPDATE items SET status='dead_letter' WHERE id=?`, dependency); ifErr13 != nil {
		t.Fatal(ifErr13)
	}
	state, err := s.RunnableFor(agent)
	if err != nil {
		t.Fatal(err)
	}
	if state.OpenRouterItems != 0 {
		t.Fatalf("failed prerequisite released dependent execution: %+v", state)
	}
}

func TestRequirementAuditRequiresEvidence(t *testing.T) {
	s := newTestStore(t)
	if ifErr14 := s.MarkRequirementAudit("codex-home", " "); ifErr14 == nil {
		t.Fatal("empty audit evidence must be refused")
	}
}
