package casebook

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestSearchHistoryReturnsOldScopedScreen(t *testing.T) {
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "fixture/repo", Ref: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		FeatherWeight: 80, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "fixture", Passed: true}}},
		Model: maat.ModelStamp{Provider: "local:fixture", Version: "fixture", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	const screenEvidence = "maat-system-one:fixture-screen"
	rows := []maat.Decision{{
		Time: "2026-09-01T00:00:00Z", Host: "fixture", Kind: "system one screen", Requester: "fixture",
		Assessed: "fixture screen", Determination: string(verdict.Gate), Why: "retained screen",
		Evidence: screenEvidence, SystemOne: &verdict,
	}}
	for index := 0; index < 60; index++ {
		rows = append(rows, maat.Decision{
			Time: time.Date(2026, 10, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "reservation",
			Determination: "grant", Why: "unrelated", Evidence: "reservation:fixture",
		})
	}
	for _, row := range rows {
		if err := journal.Append(row); err != nil {
			t.Fatal(err)
		}
	}
	result, err := SearchHistory(journal, HistoryQuery{Query: Query{Limit: 5}, Scope: maat.HistoryScopeSystemOne})
	if err != nil {
		t.Fatal(err)
	}
	if result.Window.Coverage != maat.HistoryCoverageComplete || result.Window.HasMore || result.Window.Returned != 1 {
		t.Fatalf("history window = %+v", result.Window)
	}
	if len(result.View.Cases) != 1 {
		t.Fatalf("scoped cases = %+v", result.View.Cases)
	}
	got := result.View.Cases[0]
	if got.Evidence != screenEvidence || got.Status != StatusOpen || got.Resolution != "" {
		t.Fatalf("scoped linked case = %+v", got)
	}
	if result.View.Summary.Total != 1 || result.Window.Total == nil || *result.Window.Total != 1 {
		t.Fatalf("complete scoped summary/window = %+v / %+v", result.View.Summary, result.Window)
	}
}

func TestSearchHistoryAppliesKindBeforeThePageWindow(t *testing.T) {
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	for index := 0; index < 55; index++ {
		if err := journal.Append(maat.Decision{
			Time: time.Date(2026, 10, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "reservation",
			Determination: "grant", Why: "unrelated", Evidence: "reservation:fixture",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := journal.Append(maat.Decision{
		Time: "2026-09-01T00:00:00Z", Host: "fixture", Kind: "assessment report", Requester: "fixture",
		Assessed: "assessment", Determination: "pass", Why: "older matching record", Evidence: "assessment:older",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := SearchHistory(journal, HistoryQuery{Query: Query{Kind: "assessment report", Limit: 1}, Scope: maat.HistoryScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.View.Cases) != 1 || result.View.Cases[0].Evidence != "assessment:older" || result.Window.HasMore {
		t.Fatalf("kind-filtered history = %+v / %+v", result.View.Cases, result.Window)
	}
}

func TestSearchHistoryInvalidOnlyIsNotACompleteEmptyResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	const privateInvalidValue = "private-invalid-time-value"
	if err := os.WriteFile(path, []byte(`{"time":"`+privateInvalidValue+`","host":"fixture","kind":"assessment","requester":"fixture","assessed":"fixture","determination":"block","why":"fixture"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := SearchHistory(&maat.FileDecisionJournal{Path: path}, HistoryQuery{Query: Query{Limit: 5}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Window.Coverage != maat.HistoryCoverageComplete || result.Integrity.InvalidCount != 1 || len(result.Integrity.Issues) != 1 || result.Window.TotalComplete || result.Window.Total != nil || len(result.View.Cases) != 0 {
		t.Fatalf("invalid-only history was presented as empty/complete: %+v", result)
	}
	if strings.Contains(result.Integrity.Issues[0].Reason, privateInvalidValue) || strings.Contains(result.Integrity.Issues[0].Digest, privateInvalidValue) {
		t.Fatal("history integrity exposed an invalid raw record value")
	}
}

func TestSearchHistoryEmptyWireAlwaysUsesArrays(t *testing.T) {
	emptyPath := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(t.TempDir(), "invalid.jsonl")
	if err := os.WriteFile(invalidPath, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hidden := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "hidden.jsonl")}
	review, err := maat.RecordDiagnosticOwnerReview(hidden, "fixture", maat.DiagnosticOwnerReview{Check: "empty relationship page", Message: "canonical owner review"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maat.AcceptDiagnosticOwnerReview(hidden, "fixture", maat.DiagnosticOwnerAcceptance{Evidence: review.Evidence, Conclusion: "retained conclusion"}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		journal *maat.FileDecisionJournal
		query   HistoryQuery
	}{
		{name: "missing journal", journal: &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "missing.jsonl")}},
		{name: "existing empty journal", journal: &maat.FileDecisionJournal{Path: emptyPath}},
		{name: "invalid-only journal", journal: &maat.FileDecisionJournal{Path: invalidPath}},
		{name: "hidden relationship only", journal: hidden, query: HistoryQuery{Query: Query{Kind: "diagnostic owner acceptance"}}},
		{name: "filtered empty", journal: func() *maat.FileDecisionJournal {
			j := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "filtered.jsonl")}
			if err := j.Append(maat.Decision{Time: "2026-10-01T00:00:00Z", Host: "fixture", Kind: "assessment", Requester: "fixture", Assessed: "fixture", Determination: "pass", Why: "not the requested phrase", Evidence: "filtered:one"}); err != nil {
				t.Fatal(err)
			}
			return j
		}(), query: HistoryQuery{Query: Query{Text: "absent phrase"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := SearchHistory(test.journal, test.query)
			if err != nil {
				t.Fatal(err)
			}
			if result.View.Cases == nil || result.View.Graph.Nodes == nil || result.View.Graph.Edges == nil {
				t.Fatalf("empty projection contains a nil wire array: %+v", result.View)
			}
			encoded, err := json.Marshal(result.View)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"cases", "graph"} {
				if string(wire[field]) == "null" {
					t.Fatalf("%s encoded null instead of an empty collection: %s", field, encoded)
				}
			}
			var cases []json.RawMessage
			if err := json.Unmarshal(wire["cases"], &cases); err != nil || cases == nil || len(cases) != 0 {
				t.Fatalf("cases wire value = %s, err=%v", wire["cases"], err)
			}
		})
	}
}

func TestSearchHistoryPagesMoreThanFiftyScreensAndCalibrationsHonestly(t *testing.T) {
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "fixture/repo", Ref: "main", HeadSHA: strings.Repeat("a", 40)},
		FeatherWeight: 92, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "fixture", Passed: true}}},
		Model: maat.ModelStamp{Provider: "local:fixture", Version: "fixture", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 55; index++ {
		evidence := "screen:page:" + strings.Repeat("0", 2-len(strconv.Itoa(index))) + strconv.Itoa(index)
		decision := maat.Decision{
			Time: time.Date(2026, 9, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "system one screen", Requester: "fixture", Assessed: "screen",
			Determination: string(verdict.Gate), Why: "page fixture", Evidence: evidence, SystemOne: &verdict,
		}
		if err := journal.Append(decision); err != nil {
			t.Fatal(err)
		}
		if index%10 == 0 {
			if _, _, err := maat.RecordSystemOneCalibration(journal, "fixture", evidence, "frontier:"+strconv.Itoa(index), maat.GateBlock); err != nil {
				t.Fatal(err)
			}
		}
	}
	first, err := SearchHistory(journal, HistoryQuery{Query: Query{Limit: 50}, Scope: maat.HistoryScopeSystemOne})
	if err != nil {
		t.Fatal(err)
	}
	if first.Window.Returned != 50 || !first.Window.HasMore || !first.Window.TotalComplete || first.Window.Total == nil || *first.Window.Total != 61 || first.Window.CountScope != "query" {
		t.Fatalf("initial complete query counts = %+v", first.Window)
	}
	second, err := SearchHistory(journal, HistoryQuery{Query: Query{Limit: 50}, Scope: maat.HistoryScopeSystemOne, Cursor: first.Window.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if second.Window.Returned != 11 || second.Window.HasMore || second.Window.TotalComplete || second.Window.Total != nil || second.Window.CandidateMatches != 11 || second.Window.CandidateMatchesComplete || second.Window.CountScope != "after-cursor" {
		t.Fatalf("terminal page mislabeled its tail as whole-query totals: %+v", second.Window)
	}
	if _, err := SearchHistory(journal, HistoryQuery{Query: Query{Text: "no such case", Limit: 50}, Scope: maat.HistoryScopeSystemOne, Cursor: first.Window.NextCursor}); !errors.Is(err, maat.ErrHistorySnapshotChanged) {
		t.Fatalf("changing the filter reused an earlier query's tail: %v", err)
	}
}

func TestSearchHistoryExactEvidenceCountsHiddenDuplicateIdentity(t *testing.T) {
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	review, err := maat.RecordDiagnosticOwnerReview(journal, "fixture", maat.DiagnosticOwnerReview{Check: "fixture", Message: "exact identity"})
	if err != nil {
		t.Fatal(err)
	}
	// A corrupt producer can give a redundant relationship row the review's
	// identity. Projection suppression must not make that identity unique.
	if err := journal.Append(maat.Decision{Time: "2026-10-24T23:00:00Z", Host: "fixture", Kind: "diagnostic owner acceptance", Requester: "fixture", Assessed: "owner review", Determination: "accepted", Why: "retained duplicate identity", Evidence: review.Evidence, ResolutionFor: review.Evidence}); err != nil {
		t.Fatal(err)
	}
	result, err := SearchHistory(journal, HistoryQuery{Query: Query{Limit: 2}, Evidence: review.Evidence})
	if err != nil {
		t.Fatal(err)
	}
	if result.Window.EvidenceMatches == nil || *result.Window.EvidenceMatches != 2 || !result.Window.EvidenceMatchesComplete || len(result.View.Cases) != 1 {
		t.Fatalf("hidden duplicate lost raw exact-evidence multiplicity: %+v", result)
	}
}

func TestSearchHistoryExactCaseIDFiltersBeforeLinkedQualification(t *testing.T) {
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	review, err := maat.RecordDiagnosticOwnerReview(journal, "fixture", maat.DiagnosticOwnerReview{
		Check: "retained finding", Message: "owner review remains linked",
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := Build([]maat.Decision{review}).Cases
	if len(cases) != 1 {
		t.Fatalf("review projection = %+v", cases)
	}
	for index := 0; index < 150; index++ {
		if err := journal.Append(maat.Decision{
			Time: time.Date(2027, 1, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "unrelated reservation",
			Determination: "grant", Why: "unrelated linked-state candidate", Evidence: "reservation:" + strconv.Itoa(index),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := maat.AcceptDiagnosticOwnerReview(journal, "fixture", maat.DiagnosticOwnerAcceptance{
		Evidence: review.Evidence, Conclusion: "recorded owner conclusion",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := SearchHistory(journal, HistoryQuery{
		Query: Query{Text: cases[0].ID, Status: StatusResolved, Limit: 2}, Scope: maat.HistoryScopeAll,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Window.Coverage != maat.HistoryCoverageComplete || result.Window.Returned != 1 ||
		len(result.View.Cases) != 1 || result.View.Cases[0].ID != cases[0].ID ||
		result.View.Cases[0].Evidence != review.Evidence || result.View.Cases[0].Status != StatusResolved {
		t.Fatalf("exact ID did not return the linked, currently resolved case: %+v", result)
	}
	rows, _, err := journal.RecentTolerant(1000)
	if err != nil {
		t.Fatal(err)
	}
	if result.Window.ScannedRecords > 4*len(rows) {
		t.Fatalf("exact ID qualified unrelated rows in repeated whole-journal passes: scanned=%d rows=%d", result.Window.ScannedRecords, len(rows))
	}
}

func TestSearchHistoryExactCaseIDRetainsDuplicateRowsForAmbiguity(t *testing.T) {
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	duplicate := maat.Decision{
		Time: "2026-10-30T00:00:00Z", Host: "fixture", Kind: "diagnostic owner review",
		Requester: "fixture", Resource: "same finding", Assessed: "diagnostic finding",
		Determination: "owner_review_required", Why: "duplicate identity fixture",
		Evidence: "diagnostic-finding:sha256=" + strings.Repeat("a", 64),
	}
	for index := 0; index < 2; index++ {
		if err := journal.Append(duplicate); err != nil {
			t.Fatal(err)
		}
	}
	identity := Build([]maat.Decision{duplicate}).Cases[0].ID
	result, err := SearchHistory(journal, HistoryQuery{Query: Query{Text: identity, Limit: 2}, Scope: maat.HistoryScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	if result.Window.CandidateMatches != 2 || result.Window.Returned != 2 || len(result.View.Cases) != 2 ||
		result.View.Cases[0].ID != identity || result.View.Cases[1].ID != identity {
		t.Fatalf("duplicate case identity was collapsed or reported unique: %+v", result)
	}
}

func TestSearchHistoryUsesCanonicalReviewAcceptanceBeforeTextStatusLimit(t *testing.T) {
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	input := maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "fixture/repo", Ref: "main", HeadSHA: strings.Repeat("b", 40)},
		FeatherWeight: 80, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "fixture", Passed: true}}},
		Model: maat.ModelStamp{Provider: "local:fixture", Version: "fixture", Local: true},
		Findings: []maat.ScreenFinding{
			{ID: "finding-a", Severity: "block", Category: "governance", File: "a", Line: 1, Claim: "first retained issue", Evidence: "receipt:first", Confidence: 1},
			{ID: "finding-b", Severity: "block", Category: "governance", File: "b", Line: 2, Claim: "second retained issue", Evidence: "receipt:second", Confidence: 1},
		},
	}
	verdict, err := maat.Screen(input)
	if err != nil {
		t.Fatal(err)
	}
	screen := maat.Decision{Time: "2026-09-01T00:00:00Z", Host: "fixture", Kind: "system one screen", Requester: "fixture", Assessed: "multi-finding screen", Determination: string(verdict.Gate), Why: "two active findings", Evidence: "screen:multi-finding", SystemOne: &verdict}
	if err := journal.Append(screen); err != nil {
		t.Fatal(err)
	}
	review, err := maat.RecordDiagnosticOwnerReview(journal, "fixture owner", maat.DiagnosticOwnerReview{
		Check: "multi-finding screen", Message: "Review both retained findings", OriginEvidence: screen.Evidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maat.AcceptDiagnosticOwnerReview(journal, "fixture owner", maat.DiagnosticOwnerAcceptance{
		Evidence: review.Evidence, Conclusion: "accepted canonical owner conclusion",
	}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 20; index++ {
		if err := journal.Append(maat.Decision{
			Time: time.Date(2027, 1, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "noise",
			Determination: "grant", Why: "unrelated newer row", Evidence: "noise:" + strconv.Itoa(index),
		}); err != nil {
			t.Fatal(err)
		}
	}
	filtered, err := SearchHistory(journal, HistoryQuery{Query: Query{Kind: "diagnostic owner review", Status: StatusResolved, Text: "accepted canonical owner conclusion", Limit: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Window.Returned != 1 || filtered.View.Cases[0].Evidence != review.Evidence || filtered.View.Cases[0].Status != StatusResolved || filtered.View.Cases[0].Resolution != "accepted canonical owner conclusion" {
		t.Fatalf("canonical linked outcome was filtered after page limiting: %+v", filtered)
	}
	all, err := SearchHistory(journal, HistoryQuery{Query: Query{Limit: 50}})
	if err != nil {
		t.Fatal(err)
	}
	var original *Case
	var acceptedReview *Case
	acceptanceRows := 0
	for index := range all.View.Cases {
		entry := &all.View.Cases[index]
		switch entry.Evidence {
		case screen.Evidence:
			original = entry
		case review.Evidence:
			acceptedReview = entry
		}
		if entry.Kind == "diagnostic owner acceptance" {
			acceptanceRows++
		}
	}
	if original == nil || original.Status != StatusOpen || original.SystemOne == nil || len(original.SystemOne.Findings) != 2 {
		t.Fatalf("accepting the owner review incorrectly resolved the original multi-finding screen: %+v", original)
	}
	if acceptedReview == nil || acceptedReview.Status != StatusResolved || acceptanceRows != 0 {
		t.Fatalf("canonical acceptance row/review projection = review:%+v acceptanceRows:%d", acceptedReview, acceptanceRows)
	}
}
