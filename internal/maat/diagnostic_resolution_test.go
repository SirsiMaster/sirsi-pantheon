package maat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestAcceptDiagnosticOwnerReviewFindsExactOldReviewBeyondRecentWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	j := &FileDecisionJournal{Path: path}
	review, err := RecordDiagnosticOwnerReview(j, "fixture owner", DiagnosticOwnerReview{
		Check: "historical review", Message: "review remains actionable", OriginEvidence: "screen:historical",
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for index := 0; index < 1001; index++ {
		row := Decision{
			Time: time.Date(2026, 10, 25, 0, index/3600, index/60%60, index%60, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "unrelated reservation",
			Determination: "grant", Why: "newer unrelated evidence", Evidence: "reservation:" + strconv.Itoa(index),
		}
		if err := encoder.Encode(row); err != nil {
			file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	recent, err := j.Recent(1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range recent {
		if row.Evidence == review.Evidence {
			t.Fatal("fixture review is still within the legacy Recent(1000) window")
		}
	}
	accepted, err := AcceptDiagnosticOwnerReview(j, "fixture owner", DiagnosticOwnerAcceptance{
		Evidence: review.Evidence, Conclusion: "accepted after the bounded recent window",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.ResolutionFor != review.Evidence {
		t.Fatalf("historical acceptance lost its exact review link: %+v", accepted)
	}
	rows, err := j.Recent(1100)
	if err != nil {
		t.Fatal(err)
	}
	acceptances := 0
	for _, row := range rows {
		if row.Evidence == accepted.Evidence && row.ResolutionFor == review.Evidence {
			acceptances++
		}
	}
	if acceptances != 1 {
		t.Fatalf("strict exact mutation appended %d acceptances, want one", acceptances)
	}
	if _, err := AcceptDiagnosticOwnerReview(j, "fixture owner", DiagnosticOwnerAcceptance{Evidence: review.Evidence, Conclusion: "replay"}); err == nil {
		t.Fatal("strict exact lookup accepted an already-accepted old review")
	}
}

func TestAcceptDiagnosticOwnerReviewRefusesMalformedStrictHistoryWithoutAppending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	j := &FileDecisionJournal{Path: path}
	review, err := RecordDiagnosticOwnerReview(j, "fixture owner", DiagnosticOwnerReview{
		Check: "strict history", Message: "requires complete strict history",
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{malformed strict row}\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcceptDiagnosticOwnerReview(j, "fixture owner", DiagnosticOwnerAcceptance{
		Evidence: review.Evidence, Conclusion: "must not append",
	}); err == nil {
		t.Fatal("accepted from a malformed strict journal")
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() != after.Size() {
		t.Fatalf("strict refusal appended bytes: before=%d after=%d", before.Size(), after.Size())
	}
}
