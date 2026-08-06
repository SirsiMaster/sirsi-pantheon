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
	if err := s.MarkRequirementAudit(agent, "audit://ADR-057/REQ-map"); err != nil {
		t.Fatal(err)
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
	if err := s.CloseItem(itemID, "done"); err != nil {
		t.Fatal(err)
	}

	if err := s.AddTask(Task{Agent: agent, TaskID: "T1", Subject: "ledger work"}); err != nil {
		t.Fatal(err)
	}
	state, _ = s.RunnableFor(agent)
	if !state.Runnable || state.ActionableLedgerTasks != 1 {
		t.Fatalf("actionable task must make lane runnable: %+v", state)
	}
	lease, err := s.ClaimNextTask(agent, "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteTaskLease(agent, "T1", lease.Token, "proof://T1"); err != nil {
		t.Fatal(err)
	}

	req, err := s.AddRequirement("runtime invariant", "ADR-057", "step 3", agent)
	if err != nil {
		t.Fatal(err)
	}
	state, _ = s.RunnableFor(agent)
	if !state.Runnable || state.UnmetRequirements != 1 {
		t.Fatalf("unmet requirement must make lane runnable: %+v", state)
	}
	decision, _ := s.Send("owner", agent, "waive", "decision", "supersede")
	if err := s.CloseItem(decision, "owner approved supersession"); err != nil {
		t.Fatal(err)
	}
	if err := s.Waive(req.ID, "owner-ratified supersession", decision); err != nil {
		t.Fatal(err)
	}
	state, _ = s.RunnableFor(agent)
	if state.Runnable {
		t.Fatalf("all three audited sources empty must permit park: %+v", state)
	}
}

func TestRunnableForExcludesBlockedWork(t *testing.T) {
	s := newTestStore(t)
	agent := "codex-home"
	if err := s.MarkRequirementAudit(agent, "audit://empty"); err != nil {
		t.Fatal(err)
	}
	dependency, err := s.Send("owner", "other", "dependency", "review", "first")
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := s.Send("owner", agent, "dependent", "review", "later")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBlockedBy(itemID, dependency); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: agent, TaskID: "blocked", Subject: "wait", BlockedBy: "external-decision"}); err != nil {
		t.Fatal(err)
	}
	state, err := s.RunnableFor(agent)
	if err != nil {
		t.Fatal(err)
	}
	if state.Runnable || state.OpenRouterItems != 0 || state.ActionableLedgerTasks != 0 {
		t.Fatalf("blocked work is not actionable: %+v", state)
	}
	if err := s.CloseItem(dependency, "resolved"); err != nil {
		t.Fatal(err)
	}
	state, _ = s.RunnableFor(agent)
	if !state.Runnable || state.OpenRouterItems != 1 {
		t.Fatalf("terminal dependency must unblock router item: %+v", state)
	}
}

func TestDeadLetteredDependencyDoesNotReleaseDependentWork(t *testing.T) {
	s := newTestStore(t)
	agent := "codex-home"
	if err := s.MarkRequirementAudit(agent, "audit://empty"); err != nil {
		t.Fatal(err)
	}
	dependency, err := s.Send("owner", "other", "dependency", "review", "first")
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := s.Send("owner", agent, "dependent", "review", "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBlockedBy(dependent, dependency); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE items SET status='dead_letter' WHERE id=?`, dependency); err != nil {
		t.Fatal(err)
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
	if err := s.MarkRequirementAudit("codex-home", " "); err == nil {
		t.Fatal("empty audit evidence must be refused")
	}
}
