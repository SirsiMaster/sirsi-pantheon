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

func TestBuildGivesEscalatedSystemOneScreensAnEvidenceBoundReviewRoute(t *testing.T) {
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "SirsiMaster/sirsi-pantheon", Ref: "release/v0.24.4", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Boundary: "delivery"},
		FeatherWeight: 90, Confidence: 0.99,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "gofmt", Passed: true}}},
		Model: maat.ModelStamp{Provider: "local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	view := Build([]maat.Decision{{
		Time: "2026-09-27T09:00:00Z", Host: "m5", Kind: "system one screen", Requester: "sirsi maat screen",
		Resource: "SirsiMaster/sirsi-pantheon", Assessed: "commit release/v0.24.4", Affected: verdict.Subject.HeadSHA,
		Determination: string(verdict.Gate), Why: "sensitive boundary requires frontier review", Evidence: "maat-system-one:sha256=fixture", SystemOne: &verdict,
	}})
	if len(view.Cases) != 1 || view.Cases[0].Category != "governance" || view.Cases[0].NextAction == nil || view.Cases[0].NextAction.Kind != "system_one_review" {
		t.Fatalf("System One casebook projection = %+v", view)
	}
}

func TestBuildProjectsCalibrationAsCompletedEvidenceWithBothLinks(t *testing.T) {
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "SirsiMaster/sirsi-pantheon", Ref: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		FeatherWeight: 92, Confidence: 0.96,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "gofmt", Passed: true}}},
		Model: maat.ModelStamp{Provider: "local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	const screenEvidence = "maat-system-one:sha256=screen"
	const frontierEvidence = "review:sha256=independent"
	view := Build([]maat.Decision{
		{Time: "2026-09-27T09:00:00Z", Host: "m5", Kind: "system one screen", Requester: "sirsi maat screen", Assessed: "commit main", Determination: string(verdict.Gate), Why: "local pass", Evidence: screenEvidence, SystemOne: &verdict},
		{Time: "2026-09-27T09:01:00Z", Host: "m5", Kind: "system one calibration", Requester: "sirsi maat calibrate", Assessed: "System One auto-pass calibration", Determination: "block", Why: "independent review overturned a local System One auto-pass", Evidence: "maat-system-one-calibration:sha256=record", SystemOneCalibration: &maat.CalibrationRecord{SchemaVersion: maat.SystemOneSchemaVersion, ScreenEvidence: screenEvidence, FrontierEvidence: frontierEvidence, ScreenGate: maat.GatePass, FrontierGate: maat.GateBlock}},
	})
	if len(view.Cases) != 2 {
		t.Fatalf("cases = %+v", view.Cases)
	}
	calibration := view.Cases[0]
	if calibration.Kind != "system one calibration" || calibration.Status != StatusResolved || calibration.Priority != PriorityNormal || calibration.NextAction != nil || calibration.SystemOneCalibration == nil {
		t.Fatalf("calibration case = %+v", calibration)
	}
	if calibration.SystemOneCalibration.ScreenEvidence != screenEvidence || calibration.SystemOneCalibration.FrontierEvidence != frontierEvidence {
		t.Fatalf("calibration links = %+v", calibration.SystemOneCalibration)
	}
	if got := Search([]maat.Decision{{Time: "2026-09-27T09:01:00Z", Host: "m5", Kind: "system one calibration", Requester: "sirsi maat calibrate", Assessed: "System One auto-pass calibration", Determination: "block", Why: "independent review", Evidence: "maat-system-one-calibration:sha256=record", SystemOneCalibration: calibration.SystemOneCalibration}}, Query{Text: frontierEvidence}); len(got.Cases) != 1 {
		t.Fatalf("calibration evidence search = %+v", got)
	}
}
