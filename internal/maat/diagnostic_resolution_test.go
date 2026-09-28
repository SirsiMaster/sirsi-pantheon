package maat

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordDiagnosticOwnerReviewIsEvidenceBoundAndDurable(t *testing.T) {
	j := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "maat", "decisions.jsonl")}
	decision, err := RecordDiagnosticOwnerReview(j, "sirsi maat record-resolution", DiagnosticOwnerReview{
		Check: "Kernel Panics (7d)", Message: "Two kernel panics observed", Detail: "kernel panic reports retained",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Determination != "owner_review_required" || !strings.HasPrefix(decision.Evidence, "diagnostic-finding:sha256=") {
		t.Fatalf("decision = %+v", decision)
	}
	rows, err := j.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Evidence != decision.Evidence || rows[0].Resource != "Kernel Panics (7d)" {
		t.Fatalf("journal rows = %+v", rows)
	}
}

func TestRecordDiagnosticOwnerReviewRejectsIncompleteOrOversizeInput(t *testing.T) {
	j := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	if _, err := RecordDiagnosticOwnerReview(j, "", DiagnosticOwnerReview{Check: "x", Message: "y"}); err == nil {
		t.Fatal("accepted blank requester")
	}
	if _, err := RecordDiagnosticOwnerReview(j, "requester", DiagnosticOwnerReview{Check: "x", Message: strings.Repeat("m", 4097)}); err == nil {
		t.Fatal("accepted oversize message")
	}
}

func TestAcceptDiagnosticOwnerReviewLinksOnlyOneKnownCase(t *testing.T) {
	j := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "maat", "decisions.jsonl")}
	review, err := RecordDiagnosticOwnerReview(j, "sirsi maat record-resolution", DiagnosticOwnerReview{
		Check: "Kernel Panics (7d)", Message: "Two kernel panics observed", OriginEvidence: "receipt:sha256=origin",
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := AcceptDiagnosticOwnerReview(j, "sirsi maat accept-resolution", DiagnosticOwnerAcceptance{
		Evidence: review.Evidence, Conclusion: "Owner accepted the documented next step.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Determination != "accepted" || accepted.ResolutionFor != review.Evidence || !strings.HasPrefix(accepted.Evidence, "diagnostic-acceptance:sha256=") {
		t.Fatalf("acceptance = %+v", accepted)
	}
	if _, err := AcceptDiagnosticOwnerReview(j, "sirsi maat accept-resolution", DiagnosticOwnerAcceptance{Evidence: review.Evidence, Conclusion: "duplicate"}); err == nil {
		t.Fatal("accepted a duplicate owner acceptance")
	}
	if _, err := AcceptDiagnosticOwnerReview(j, "sirsi maat accept-resolution", DiagnosticOwnerAcceptance{Evidence: "diagnostic-finding:sha256=missing", Conclusion: "unknown"}); err == nil {
		t.Fatal("accepted an unknown owner review")
	}
}
