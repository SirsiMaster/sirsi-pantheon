package casebook

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestSearchClassifiesPrioritizesAndLinksEvidence(t *testing.T) {
	decisions := []maat.Decision{
		{Time: "2026-09-27T09:00:00Z", Host: "m5", Kind: "reservation refusal", Requester: "apollo", Resource: "m5", Affected: "hermes", Assessed: "active reservation", Determination: "refuse", Why: "overlap", Evidence: "reservation:m5-1"},
		{Time: "2026-09-27T09:01:00Z", Host: "m5", Kind: "cede request", Requester: "apollo", Resource: "m5", Affected: "hermes", Assessed: "shared capacity", Determination: "pending", Why: "needs two cores", Evidence: "cede:m5-1"},
		{Time: "2026-09-27T09:02:00Z", Host: "m5", Kind: "guard", Requester: "pantheon", Resource: "release", Assessed: "all gates passed", Determination: "pass", Why: "verified", Evidence: "receipt:1"},
	}
	got := Search(decisions, Query{Text: "m5", Status: StatusOpen, Limit: 10})
	if got.Summary.Total != 2 || got.Summary.Urgent != 1 || got.Summary.High != 1 {
		t.Fatalf("summary = %+v, want two open (one urgent, one high)", got.Summary)
	}
	if got.Cases[0].Category != "allocation" || got.Cases[0].Priority != PriorityHigh {
		t.Fatalf("newest cede case = %+v", got.Cases[0])
	}
	if len(got.Graph.Nodes) == 0 || len(got.Graph.Edges) != 8 {
		t.Fatalf("graph = %+v", got.Graph)
	}
}

func TestSearchFiltersCategoryAndDoesNotInventMatches(t *testing.T) {
	decisions := []maat.Decision{
		{Time: "2026-09-27T09:00:00Z", Host: "m5", Kind: "guard", Requester: "pantheon", Assessed: "failed", Determination: "fail", Why: "timeout"},
	}
	got := Search(decisions, Query{Kind: "allocation", Limit: 5})
	if got.Summary.Total != 0 || len(got.Cases) != 0 || len(got.Graph.Nodes) != 0 {
		t.Fatalf("unmatched filter returned %+v", got)
	}
}

func TestBuildRoutesOpenCasesAndResolvesOnlyAcceptedOwnerReview(t *testing.T) {
	const reviewEvidence = "diagnostic-finding:sha256=review"
	decisions := []maat.Decision{
		{Time: "2026-09-27T09:00:00Z", Host: "m5", Kind: "guard", Requester: "pantheon", Resource: "release", Assessed: "release readiness", Determination: "block", Why: "missing signed evidence", Evidence: "receipt:sha256=blocked"},
		{Time: "2026-09-27T09:01:00Z", Host: "m5", Kind: "diagnostic owner review", Requester: "pantheon", Resource: "release", Assessed: "diagnostic finding", Determination: "owner_review_required", Why: "owner review required", Evidence: reviewEvidence, OriginEvidence: "receipt:sha256=blocked"},
		{Time: "2026-09-27T09:02:00Z", Host: "m5", Kind: "diagnostic owner acceptance", Requester: "pantheon", Resource: "release", Assessed: "owner-reviewed diagnostic finding", Determination: "accepted", Why: "accepted plan", Evidence: "diagnostic-acceptance:sha256=accepted", ResolutionFor: reviewEvidence},
	}
	view := Build(decisions)
	if len(view.Cases) != 2 {
		t.Fatalf("cases = %+v, want source case plus resolved review", view.Cases)
	}
	for _, c := range view.Cases {
		switch c.Evidence {
		case "receipt:sha256=blocked":
			if c.NextAction == nil || c.NextAction.Kind != "owner_review" {
				t.Fatalf("source case next action = %+v", c.NextAction)
			}
		case reviewEvidence:
			if c.Status != StatusResolved || c.Resolution != "accepted plan" || c.NextAction != nil {
				t.Fatalf("review case = %+v", c)
			}
		}
	}
}
