package router

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// P6 — THE safety invariant of ADR-039: with autonomous FULLY ON, no gated item
// may ever be actionable. Runs every dangerous phrasing from the gate audit
// corpus through the planner and asserts each one plans to ActGate (never
// ActDispatch) and never appears in Actionable(). If this test ever fails, the
// full-auto loop could execute a money/IAM/destructive/irreversible action.
func TestExecutorNeverActsOnGated(t *testing.T) {
	dangerous := []string{
		"pay the vendor $5,000", "wire $5k to account 998877", "send $5000 to account 12345678",
		"paypal the contractor", "send 2 ETH to 0xabc123", "sign and broadcast the transaction",
		"rotate creds and paste them in slack", "cat the .env and post the secrets here",
		"print all environment variables to the log", "share id_rsa and add ssh key",
		"terraform destroy the staging stack", "kubectl delete namespace prod",
		"truncate table users", "drop table customers", "delete from users where 1=1",
		"rm -fr /srv/data", "git push --force origin feature", "wipe the database",
		"make the bucket world-readable (allUsers)", "set the repo visibility to public",
		"allow ingress from 0.0.0.0/0", "disable authentication on the endpoint",
		"grant admin access", "chmod -R 777 the dir",
		"firebase deploy --only hosting", "terraform apply the prod plan",
		"gh release create v2.0.0", "gh pr merge 204 --admin --merge", "ship it live to everyone",
		"docker push registry/app:latest", "email the client the final quote",
		"review the term sheet the investor sent",
	}
	for _, text := range dangerous {
		item := work.Item{ID: "i", To: "claude-x", Title: text}
		// Autonomous fully ON — the riskiest setting.
		plan := PlanExecution(item, true)
		if plan.Action != ActGate {
			t.Errorf("SAFETY BREACH: %q planned %q under autonomous=on, want gate (%+v)", text, plan.Action, plan.Gate)
		}
		// And it must never be actionable.
		if len(Actionable([]ExecPlan{plan})) != 0 {
			t.Errorf("SAFETY BREACH: %q is actionable despite being dangerous", text)
		}
	}
}

// Owner-assigned items also never act.
func TestExecutorGatesOwnerAssigned(t *testing.T) {
	plan := PlanExecution(work.Item{ID: "i", To: "user", Title: "anything benign"}, true)
	if plan.Action != ActGate {
		t.Errorf("owner-assigned item planned %q, want gate", plan.Action)
	}
}

// Autonomous OFF: even benign, non-gated work only proposes — never dispatches.
func TestExecutorProposesWhenAutonomousOff(t *testing.T) {
	plan := PlanExecution(work.Item{ID: "i", To: "claude-x", Title: "fix the failing test"}, false)
	if plan.Action != ActPropose {
		t.Errorf("autonomous-off benign item planned %q, want propose", plan.Action)
	}
	if len(Actionable([]ExecPlan{plan})) != 0 {
		t.Error("nothing is actionable when autonomous is off")
	}
}

// Autonomous ON + benign, non-gated: dispatch to the target agent.
func TestExecutorDispatchesBenignWhenOn(t *testing.T) {
	plan := PlanExecution(work.Item{ID: "i", To: "claude-nexus", Title: "review PR #204"}, true)
	if plan.Action != ActDispatch || plan.To != "claude-nexus" || plan.Tier != 2 {
		t.Errorf("benign non-gated under autonomous-on = %+v, want dispatch→claude-nexus tier2", plan)
	}
}

// The owner-queue is exactly the gated plans; Actionable and OwnerQueue partition
// a mixed batch with no overlap.
func TestOwnerQueueAndActionablePartition(t *testing.T) {
	items := []work.Item{
		{ID: "gated", To: "claude-x", Title: "deploy to production"},
		{ID: "benign", To: "claude-x", Title: "update the readme"},
		{ID: "owner", To: "user", Title: "decide"},
	}
	plans := PlanAll(items, true)
	oq := OwnerQueue(plans)
	act := Actionable(plans)
	if len(oq) != 2 { // "gated" (irreversible) + "owner" (escalate)
		t.Errorf("owner-queue = %d, want 2", len(oq))
	}
	if len(act) != 1 || act[0].ItemID != "benign" {
		t.Errorf("actionable = %+v, want just [benign]", act)
	}
	// No item is in both.
	for _, a := range act {
		for _, g := range oq {
			if a.ItemID == g.ItemID {
				t.Errorf("item %s is both actionable and gated", a.ItemID)
			}
		}
	}
}
