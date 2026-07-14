package router

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

func TestClassifyGate(t *testing.T) {
	cases := []struct {
		name  string
		item  work.Item
		gated bool
		class GateClass
	}{
		{"benign review", work.Item{To: "claude-pantheon", Title: "review PR #204", Instructions: "look at the diff"}, false, GateNone},
		{"benign build", work.Item{To: "claude-nexus", Title: "fix the failing test", Instructions: "the unit test is red"}, false, GateNone},

		{"money", work.Item{To: "claude-x", Title: "wire transfer to vendor", Instructions: "$5k"}, true, GateSafety},
		{"iam grant (the nexus case)", work.Item{To: "claude-nexus", Title: "deploy-contracts backend gap", Instructions: "run gcloud projects add-iam-policy-binding with roles/storage.admin"}, true, GateSafety},
		{"destructive", work.Item{To: "claude-x", Title: "clean up", Instructions: "rm -rf the stale worktrees"}, true, GateSafety},
		{"credential", work.Item{To: "claude-x", Title: "rotate the api key", Instructions: "new secret"}, true, GateSafety},

		{"founder", work.Item{To: "claude-deck", Title: "review the term sheet", Instructions: "investor sent terms"}, true, GateFounder},

		{"deploy prod", work.Item{To: "claude-fw", Title: "deploy to production", Instructions: "cut over"}, true, GateIrreversible},
		{"publish", work.Item{To: "claude-nexus", Title: "publish the article", Instructions: "goes live on sirsi.ai"}, true, GateIrreversible},
		{"send email", work.Item{To: "claude-x", Title: "send the email to the customer", Instructions: "the reply"}, true, GateIrreversible},

		{"addressed to owner", work.Item{To: "user", Title: "anything at all", Instructions: "even benign"}, true, GateEscalate},
		{"explicit escalate", work.Item{To: "claude-x", Title: "ESCALATE: ambiguous fork", Instructions: "cannot decide"}, true, GateEscalate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyGate(c.item)
			if got.Gated != c.gated {
				t.Fatalf("gated = %v, want %v (reason=%q)", got.Gated, c.gated, got.Reason)
			}
			if got.Gated && got.Class != c.class {
				t.Errorf("class = %s, want %s (reason=%q)", got.Class, c.class, got.Reason)
			}
			if got.Gated && got.Reason == "" {
				t.Error("a gated decision must carry a reason")
			}
		})
	}
}

// Safety must win over a weaker class when both terms are present — the strongest
// (most dangerous) gate is the one reported.
func TestClassifyGateSafetyWinsOrdering(t *testing.T) {
	item := work.Item{
		To:           "claude-x",
		Title:        "publish the release AND grant role storage.admin",
		Instructions: "both an irreversible publish and an IAM grant",
	}
	if got := ClassifyGate(item); got.Class != GateSafety {
		t.Errorf("class = %s, want safety (safety outranks irreversible)", got.Class)
	}
}

// A silently-empty gate table would make everything auto-executable — a critical
// safety failure. Lock the table non-empty.
func TestGateTableArmed(t *testing.T) {
	if GatePatternCount() == 0 {
		t.Fatal("hard-gate table is EMPTY — every action would be auto-executable")
	}
}
