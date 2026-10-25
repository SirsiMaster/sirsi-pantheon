package casebook

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

type casebookTestJournal struct{ decisions []maat.Decision }

func casebookCaseByEvidence(cases []Case, evidence string) (Case, bool) {
	for _, item := range cases {
		if item.Evidence == evidence {
			return item, true
		}
	}
	return Case{}, false
}

func (j *casebookTestJournal) Append(d maat.Decision) error {
	j.decisions = append(j.decisions, d)
	return nil
}
func (j *casebookTestJournal) Recent(limit int) ([]maat.Decision, error) {
	if limit > 0 && len(j.decisions) > limit {
		return append([]maat.Decision(nil), j.decisions[len(j.decisions)-limit:]...), nil
	}
	return append([]maat.Decision(nil), j.decisions...), nil
}

func TestSourceScreenV2ProjectsReviewWithoutHostRepairAuthority(t *testing.T) {
	for _, tc := range []struct {
		name  string
		score int
		major bool
		floor bool
		route string
	}{
		{"major", 92, true, true, "system_one_review"},
		{"low-score-no-findings", 84, false, true, "system_one_review"},
		{"declared-floor-red", 92, false, false, "system_one_floor_recovery"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := maat.SourceScreen{Contract: maat.SourceScreenContract, SchemaVersion: 2,
				Subject:       maat.VerdictSubject{Kind: "commit", Repo: "fixture/repo", Ref: "main", HeadSHA: strings.Repeat("a", 40)},
				FeatherWeight: tc.score, Confidence: 1, Floor: maat.FloorResult{Passed: tc.floor, Checks: []maat.FloorCheck{{Name: "gofmt", Passed: true}, {Name: "vet", Passed: true}, {Name: "golangci-lint", Passed: true}, {Name: "test-race", Passed: true}}},
				Model: maat.ModelStamp{Provider: "local:fixture", Version: "fixture", Local: true, Calibration: "unqualified", CalibrationID: strings.Repeat("c", 64), EvidenceSetDigest: strings.Repeat("d", 64)}}
			if tc.major {
				s.Findings = []maat.ScreenFinding{{ID: "major", Severity: "major", Category: "correctness", Claim: "inert claim", Evidence: "inert proof", EvidenceID: strings.Repeat("e", 64), Confidence: 1}}
			}
			v, err := maat.ScreenSource(s, s.Subject.HeadSHA)
			if err != nil {
				t.Fatal(err)
			}
			view := Build([]maat.Decision{{Time: "2026-10-24T00:00:00Z", Host: "fixture", Kind: "system one screen", Requester: "fixture", Assessed: "source", Determination: string(v.Gate), Why: "declared only", Evidence: "fixture:screen", SystemOne: &v}})
			if len(view.Cases) != 1 {
				t.Fatal("lost source case")
			}
			c := view.Cases[0]
			if c.Status != StatusOpen || c.NextAction == nil || c.NextAction.Kind != tc.route || c.NextAction.ActionID != "" {
				t.Fatalf("source route=%+v", c)
			}
			if c.SystemOne.Model.Calibration != "unqualified" || !strings.Contains(c.SystemOne.FloorEvidence, "declared-only") {
				t.Fatal("projection lost provenance")
			}
		})
	}
}

func TestSearchClassifiesPrioritizesAndLinksEvidence(t *testing.T) {
	decisions := []maat.Decision{
		{Time: "2026-09-27T09:00:00Z", Host: "m5", Kind: "reservation refusal", Requester: "apollo", Resource: "m5", Affected: "mercury", Assessed: "active reservation", Determination: "refuse", Why: "overlap", Evidence: "reservation:m5-1"},
		{Time: "2026-09-27T09:01:00Z", Host: "m5", Kind: "cede request", Requester: "apollo", Resource: "m5", Affected: "mercury", Assessed: "shared capacity", Determination: "pending", Why: "needs two cores", Evidence: "cede:m5-1"},
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

func TestBuildGivesFailureMemoryRejectAThreeLevelRecheckRoute(t *testing.T) {
	const evidence = "maat-failure-memory:receipt-sha256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa;action-sha256=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb;registry-sha256=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	view := Build([]maat.Decision{{
		Time: "2026-10-05T12:50:00Z", Host: "m5", Kind: "failure memory preflight", Requester: "maat",
		Resource: "router", Affected: "macos-local", Assessed: "service-repair", Determination: "reject",
		Why: "one measured record; one recovery action", Evidence: evidence,
	}})
	if len(view.Cases) != 1 || view.Cases[0].NextAction == nil {
		t.Fatalf("failure-memory casebook projection = %+v", view)
	}
	action := view.Cases[0].NextAction
	if action.Kind != "failure_memory_recheck" || !action.RequiresConfirmation || action.Evidence != evidence || len(action.Steps) != 3 {
		t.Fatalf("failure-memory recovery route = %+v", action)
	}
	for index, step := range action.Steps {
		if step.Level != index+1 || step.Evidence != evidence {
			t.Fatalf("recovery step %d = %+v", index, step)
		}
	}
	for _, identity := range []string{"receipt-sha256=", "action-sha256=", "registry-sha256="} {
		if !strings.Contains(action.Evidence, identity) {
			t.Fatalf("resolution route lost bound failure-memory provenance %q: %+v", identity, action)
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

func TestBuildGivesFailedSystemOneFloorAThreeLevelRecoveryRoute(t *testing.T) {
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "SirsiMaster/sirsi-pantheon", Ref: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		FeatherWeight: 91, Confidence: 0.98,
		Floor: maat.FloorResult{Passed: false, Checks: []maat.FloorCheck{{Name: "receipt schema", Passed: false, Detail: "the release receipt is malformed"}}},
		Findings: []maat.ScreenFinding{{
			ID: "receipt-schema", Severity: "block", Category: "provenance", Claim: "release receipt is malformed", Evidence: "receipt:invalid", Confidence: 0.99,
			FixHint: "Regenerate the exact release receipt from the committed object, then independently review it.",
		}},
		Model: maat.ModelStamp{Provider: "local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	view := Build([]maat.Decision{{
		Time: "2026-09-28T04:00:00Z", Host: "m5", Kind: "system one screen", Requester: "sirsi maat screen",
		Assessed: "commit main", Determination: string(verdict.Gate), Why: "receipt schema failed", Evidence: "maat-system-one:sha256=failed-floor", SystemOne: &verdict,
	}})
	if len(view.Cases) != 1 || view.Cases[0].NextAction == nil {
		t.Fatalf("System One failed-floor casebook projection = %+v", view)
	}
	action := view.Cases[0].NextAction
	if action.Kind != "system_one_floor_recovery" || !action.RequiresConfirmation || len(action.Steps) != 3 {
		t.Fatalf("failed-floor recovery route = %+v", action)
	}
	if action.Steps[0].Level != 1 || action.Steps[0].Detail != "receipt schema: the release receipt is malformed" || action.Steps[1].Detail != "Regenerate the exact release receipt from the committed object, then independently review it." || action.Steps[2].Level != 3 || !action.Steps[2].RequiresConfirmation {
		t.Fatalf("failed-floor recovery steps = %+v", action.Steps)
	}
}

func TestBuildRoutesKnownSystemOneRepairThroughClosedMaatAction(t *testing.T) {
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "host", Repo: "m5", Ref: "diagnostic", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		FeatherWeight: 72, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "diagnostic observation", Passed: true}}},
		Findings: []maat.ScreenFinding{{
			ID: "disabled-managed-labels", Severity: "block", Category: "host-health", Claim: "managed labels disabled", Evidence: "diagnostic:sha256=host:disabled", Confidence: 1,
			RepairID: maat.SystemOneRepairLaunchdDisabled,
		}},
		Model: maat.ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	const evidence = "maat-system-one:sha256=host-disabled"
	view := Build([]maat.Decision{{
		Time: "2026-09-28T04:00:00Z", Host: "m5", Kind: "system one screen", Requester: "sirsi maat triage",
		Assessed: "host diagnostic", Determination: string(verdict.Gate), Why: "managed labels disabled", Evidence: evidence, SystemOne: &verdict,
	}})
	if len(view.Cases) != 1 || view.Cases[0].NextAction == nil {
		t.Fatalf("System One repair projection = %+v", view)
	}
	action := view.Cases[0].NextAction
	if action.Kind != "maat_repair" || action.ActionID != maat.SystemOneRepairLaunchdDisabled || action.Evidence != evidence || !action.RequiresConfirmation || len(action.Steps) != 3 {
		t.Fatalf("closed repair action = %+v", action)
	}
}

func TestBuildRoutesLivenessWatchThroughClosedMaatAction(t *testing.T) {
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "host", Repo: "m5", Ref: "diagnostic", HeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		FeatherWeight: 72, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "diagnostic observation", Passed: true}}},
		Findings: []maat.ScreenFinding{{
			ID: "liveness-watch", Severity: "major", Category: "host-health", Claim: "liveness watch absent", Evidence: "diagnostic:sha256=host:liveness", Confidence: 1,
			RepairID: maat.SystemOneRepairLivenessWatch,
		}},
		Model: maat.ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	view := Build([]maat.Decision{{
		Time: "2026-10-05T04:00:00Z", Host: "m5", Kind: "system one screen", Requester: "sirsi maat triage",
		Assessed: "host diagnostic", Determination: string(verdict.Gate), Why: "liveness watch absent", Evidence: "maat-system-one:sha256=liveness", SystemOne: &verdict,
	}})
	if len(view.Cases) != 1 || view.Cases[0].NextAction == nil {
		t.Fatalf("System One liveness repair projection = %+v", view)
	}
	action := view.Cases[0].NextAction
	if action.Kind != "maat_repair" || action.ActionID != maat.SystemOneRepairLivenessWatch || !action.RequiresConfirmation || len(action.Steps) != 4 {
		t.Fatalf("liveness repair action = %+v", action)
	}
	if !strings.Contains(action.Steps[1].Detail, "no prior incidents are imported") || !strings.Contains(action.Steps[2].Detail, "durable attempt record") {
		t.Fatalf("liveness repair action omits explicit local genesis or durable attempt: %+v", action.Steps)
	}
}

func TestBuildRoutesBinaryDriftThroughConfirmedSupportedUpdater(t *testing.T) {
	const headSHA = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "host", Repo: "m5", Ref: "diagnostic", HeadSHA: headSHA},
		FeatherWeight: 72, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "diagnostic observation", Passed: true}}},
		Findings: []maat.ScreenFinding{{
			ID: maat.CanonicalHostFindingID("binary-drift"), Severity: "major", Category: "host-health", Claim: "Sirsi binary drift detected",
			Evidence: maat.CanonicalHostFindingEvidence(headSHA, "binary-drift"), Confidence: 1, FixHint: "untrusted producer command",
			RepairID: maat.SystemOneRepairBinaryDrift,
		}},
		Model: maat.ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	journal := &casebookTestJournal{}
	screen, err := maat.RecordSystemOne(journal, "untrusted label is not authority", verdict)
	if err != nil {
		t.Fatal(err)
	}
	evidence := screen.Evidence
	view := Build(journal.decisions)
	if len(view.Cases) != 1 {
		t.Fatalf("binary-drift case projection = %+v", view)
	}
	action := view.Cases[0].NextAction
	if action == nil || action.Kind != "maat_repair" || action.ActionID != maat.SystemOneRepairBinaryDrift || action.Evidence != evidence || !action.RequiresConfirmation || len(action.Steps) != 3 {
		t.Fatalf("binary-drift resolution = %+v", action)
	}
	for index, step := range action.Steps {
		if step.Level != index+1 || step.Evidence != evidence {
			t.Fatalf("binary-drift step %d lost source evidence: %+v", index+1, step)
		}
	}
	if strings.Contains(action.Detail, "untrusted producer command") || !strings.Contains(action.Detail, "Homebrew") {
		t.Fatalf("repair route trusted producer text or omitted updater boundary: %+v", action)
	}
}

func TestResolvedBinaryRepairDoesNotDischargeOtherFindingsOrFailedFloor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		floor bool
		other bool
	}{
		{name: "independent finding", floor: true, other: true},
		{name: "failed floor", floor: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head := strings.Repeat("c", 64)
			floorDetail := ""
			if !tc.floor {
				floorDetail = "fixture: local diagnostic failed"
			}
			findings := []maat.ScreenFinding{{ID: maat.CanonicalHostFindingID("binary-drift"), Severity: "major", Category: "host-health", Claim: "binary drift", Evidence: maat.CanonicalHostFindingEvidence(head, "binary-drift"), Confidence: 1, RepairID: maat.SystemOneRepairBinaryDrift}}
			if tc.other {
				findings = append(findings, maat.ScreenFinding{ID: "independent-health", Severity: "major", Category: "host-health", Claim: "independent issue", Evidence: "fixture:independent", Confidence: 1})
			}
			verdict, err := maat.Screen(maat.SystemOneScreen{
				Subject:       maat.VerdictSubject{Kind: "host", Repo: "fixture-host", Ref: "diagnostic", HeadSHA: head},
				FeatherWeight: 72, Confidence: 1, Floor: maat.FloorResult{Passed: tc.floor, Checks: []maat.FloorCheck{{Name: "local diagnostic", Passed: tc.floor, Detail: floorDetail}}}, Findings: findings,
				Model: maat.ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			journal := &casebookTestJournal{}
			screen, err := maat.RecordSystemOne(journal, "fixture", verdict)
			if err != nil {
				t.Fatal(err)
			}
			journal.decisions = append(journal.decisions, maat.Decision{Time: "2026-10-24T05:00:00Z", Kind: "diagnostic verified repair", Determination: "resolved", OriginEvidence: screen.Evidence, Evidence: "fixture:repair-receipt", Resource: "binary-drift"})
			view := Build(journal.decisions)
			item, found := casebookCaseByEvidence(view.Cases, screen.Evidence)
			if !found || item.Status != StatusOpen || item.VerifiedRepairEvidence == "" || item.RepairOutcomeEvidence == "" || item.NextAction == nil || item.NextAction.Kind != "system_one_review" || item.NextAction.ActionID != "rescreen_system_one" {
				t.Fatalf("remaining source debt was not kept open with a re-screen route: %+v", view)
			}
		})
	}
}

func TestResolvedBinaryOnlyRepairClosesFullyDischargedScreen(t *testing.T) {
	head := strings.Repeat("d", 64)
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "host", Repo: "fixture-host", Ref: "diagnostic", HeadSHA: head},
		FeatherWeight: 72, Confidence: 1, Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "local diagnostic", Passed: true}}},
		Findings: []maat.ScreenFinding{{ID: maat.CanonicalHostFindingID("binary-drift"), Severity: "major", Category: "host-health", Claim: "binary drift", Evidence: maat.CanonicalHostFindingEvidence(head, "binary-drift"), Confidence: 1, RepairID: maat.SystemOneRepairBinaryDrift}},
		Model:    maat.ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	journal := &casebookTestJournal{}
	screen, err := maat.RecordSystemOne(journal, "fixture", verdict)
	if err != nil {
		t.Fatal(err)
	}
	journal.decisions = append(journal.decisions, maat.Decision{Time: "2026-10-24T06:00:00Z", Kind: "diagnostic verified repair", Resource: "binary-drift", Determination: "resolved", OriginEvidence: screen.Evidence, Evidence: "fixture:repair-resolved"})
	view := Build(journal.decisions)
	item, found := casebookCaseByEvidence(view.Cases, screen.Evidence)
	if !found || item.Status != StatusResolved || item.VerifiedRepairEvidence == "" || item.NextAction != nil {
		t.Fatalf("fully discharged binary-only screen did not resolve: %+v", view)
	}
}

func TestUnresolvedBinaryAttemptProjectsReadOnlyInspectionRoute(t *testing.T) {
	head := strings.Repeat("e", 64)
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject: maat.VerdictSubject{Kind: "host", Repo: "fixture-host", Ref: "diagnostic", HeadSHA: head}, FeatherWeight: 72, Confidence: 1,
		Floor:    maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "local diagnostic", Passed: true}}},
		Findings: []maat.ScreenFinding{{ID: maat.CanonicalHostFindingID("binary-drift"), Severity: "major", Category: "host-health", Claim: "binary drift", Evidence: maat.CanonicalHostFindingEvidence(head, "binary-drift"), Confidence: 1, RepairID: maat.SystemOneRepairBinaryDrift}},
		Model:    maat.ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	journal := &casebookTestJournal{}
	screen, err := maat.RecordSystemOne(journal, "fixture", verdict)
	if err != nil {
		t.Fatal(err)
	}
	journal.decisions = append(journal.decisions, maat.Decision{Time: "2026-10-24T07:00:00Z", Kind: "diagnostic repair attempt", Resource: "binary-drift", Determination: "in_progress", OriginEvidence: screen.Evidence, Evidence: "diagnostic-repair-attempt:sha256=" + strings.Repeat("f", 64)})
	view := Build(journal.decisions)
	item, found := casebookCaseByEvidence(view.Cases, screen.Evidence)
	if !found || item.Status != StatusOpen || item.PendingRepairAttemptEvidence == "" || item.NextAction == nil || item.NextAction.ActionID != "inspect_binary_drift_attempt" || !strings.Contains(item.NextAction.Detail, "--inspect-attempt --case-evidence "+screen.Evidence) {
		t.Fatalf("unresolved attempt lacks read-only recovery route: %+v", view)
	}
}

func TestBuildRoutesLegacyBinaryDriftAndLinksOnlyVerifiedRepair(t *testing.T) {
	const (
		headSHA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		evidence = "maat-system-one:sha256=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "host", Repo: "fixture-host", Ref: "diagnostic", HeadSHA: headSHA},
		FeatherWeight: 72, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "diagnostic observation", Passed: true}}},
		Findings: []maat.ScreenFinding{{
			ID: maat.CanonicalHostFindingID("binary-drift"), Severity: "major", Category: "host-health",
			Claim: "Sirsi binary drift detected", Evidence: maat.CanonicalHostFindingEvidence(headSHA, "binary-drift"),
			Confidence: 1, FixHint: "Level 1: sirsi self-update",
		}},
		Model: maat.ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	original := maat.Decision{
		Time: "2026-10-24T04:00:00Z", Host: "fixture-host", Kind: "system one screen", Requester: "old producer title",
		Assessed: "host diagnostic", Determination: string(verdict.Gate), Why: "binary drift", Evidence: evidence, SystemOne: &verdict,
	}
	open := Build([]maat.Decision{original})
	if len(open.Cases) != 1 || open.Cases[0].Status != StatusOpen || open.Cases[0].NextAction == nil || open.Cases[0].NextAction.ActionID != maat.SystemOneRepairBinaryDrift {
		t.Fatalf("legacy v1 case did not receive the closed typed route: %+v", open.Cases)
	}
	if strings.Contains(open.Cases[0].NextAction.Detail, "sirsi self-update") {
		t.Fatal("legacy producer hint became part of the recovery authority")
	}

	failed := maat.Decision{Time: "2026-10-24T04:01:00Z", Kind: "diagnostic verified repair", Resource: "binary-drift", Determination: "failed", Evidence: "diagnostic-repair:sha256=failed", OriginEvidence: evidence}
	stillOpen := Build([]maat.Decision{original, failed})
	failedSource, found := casebookCaseByEvidence(stillOpen.Cases, evidence)
	if !found || failedSource.Status != StatusOpen || failedSource.VerifiedRepairEvidence != "" || failedSource.RepairOutcomeEvidence != failed.Evidence || failedSource.NextAction == nil || failedSource.NextAction.Kind != "system_one_review" || failedSource.NextAction.ActionID != "rescreen_system_one" {
		t.Fatalf("failed repair receipt was not preserved with a fresh-review route: %+v", stillOpen.Cases)
	}

	verified := maat.Decision{Time: "2026-10-24T04:02:00Z", Kind: "diagnostic verified repair", Resource: "binary-drift", Determination: "resolved", Evidence: "diagnostic-repair:sha256=verified", OriginEvidence: evidence}
	resolved := Build([]maat.Decision{original, verified})
	if len(resolved.Cases) != 2 {
		t.Fatalf("repair receipt or original case was not preserved: %+v", resolved.Cases)
	}
	var source Case
	for _, c := range resolved.Cases {
		if c.Evidence == evidence {
			source = c
		}
	}
	if source.Status != StatusResolved || source.VerifiedRepairEvidence != verified.Evidence || source.RepairOutcomeEvidence != verified.Evidence || source.Resolution != "" || source.NextAction != nil {
		t.Fatalf("verified repair was confused with owner acceptance or did not resolve its source case: %+v", source)
	}
}

func TestBuildDoesNotTreatBinaryDriftLabelAsAuthorityWithoutCanonicalFinding(t *testing.T) {
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "fixture/repo", Ref: "main", HeadSHA: strings.Repeat("a", 40)},
		FeatherWeight: 72, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "source observation", Passed: true}}},
		Findings: []maat.ScreenFinding{{
			ID: "invented-binary-drift", Severity: "major", Category: "host-health", Claim: "binary drift",
			Evidence: "producer text says binary drift", Confidence: 1, FixHint: "sirsi self-update",
			RepairID: maat.SystemOneRepairBinaryDrift,
		}},
		Model: maat.ModelStamp{Provider: "local:fixture", Version: "v1", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	path, ok := systemOneRepair(&verdict, "untrusted-screen-label")
	if ok || path != nil {
		t.Fatalf("producer-supplied repair label bypassed canonical host finding checks: %+v", path)
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

func TestLocalPreventionDispositionCompletesOnlyItsReceiptAndKeepsIncidentSearch(t *testing.T) {
	keys := []string{strings.Repeat("b", 64)}
	rows := []maat.Decision{
		{Time: "2026-10-25T00:00:00Z", Kind: "failure memory preflight", Determination: "reject", Evidence: "fixture:unhealthy", FailureMemoryIncidentKeys: &keys},
		{Time: "2026-10-25T00:00:01Z", Kind: "local prevention owner disposition", Determination: "accepted_not_repaired", Evidence: "fixture:receipt", OriginEvidence: "fixture:unhealthy"},
		{Time: "2026-10-25T00:00:02Z", Kind: "arbitrary caller label", Determination: "accepted_not_repaired", Evidence: "fixture:foreign"},
	}
	view := Build(rows)
	for _, row := range view.Cases {
		want := StatusOpen
		if row.Evidence == "fixture:receipt" {
			want = StatusResolved
		}
		if row.Status != want {
			t.Fatalf("wrong scope of completion: %+v", row)
		}
	}
	found := Search(rows, Query{Text: keys[0]})
	if len(found.Cases) != 1 || found.Cases[0].Evidence != "fixture:unhealthy" {
		t.Fatalf("incident identity lost in shared search: %+v", found.Cases)
	}
}
