package casebook

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func completeRepairHistorySearch(t *testing.T, journal *maat.FileDecisionJournal, query HistoryQuery) HistorySearchResult {
	t.Helper()
	for step := 0; step < 512; step++ {
		result, err := SearchHistory(journal, query)
		if err != nil {
			t.Fatal(err)
		}
		if result.Window.Coverage == maat.HistoryCoverageComplete {
			return result
		}
		if len(result.View.Cases) != 0 || result.Window.NextCursor == "" || result.Window.NextCursor == query.Cursor {
			t.Fatalf("provisional or stalled history: %+v", result)
		}
		query.Cursor = result.Window.NextCursor
	}
	t.Fatal("bounded fixture history did not complete")
	return HistorySearchResult{}
}

func TestSearchHistoryProjectsUpdaterLinksBeforeFilteringAndWindow(t *testing.T) {
	for _, tc := range []struct {
		name, outcome, action  string
		floor, other, verified bool
		status                 Status
	}{
		{name: "binary_only", floor: true, outcome: "resolved", verified: true, status: StatusResolved},
		{name: "other_finding", floor: true, other: true, outcome: "resolved", verified: true, status: StatusOpen, action: "rescreen_system_one"},
		{name: "failed_floor", outcome: "resolved", verified: true, status: StatusOpen, action: "rescreen_system_one"},
		{name: "failed_updater", floor: true, outcome: "failed", status: StatusOpen, action: "rescreen_system_one"},
		{name: "pending", floor: true, status: StatusOpen, action: "inspect_binary_drift_attempt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
			head := strings.Repeat("b", 64)
			findings := []maat.ScreenFinding{{ID: maat.CanonicalHostFindingID("binary-drift"), Severity: "major", Category: "host-health", Claim: "binary drift", Evidence: maat.CanonicalHostFindingEvidence(head, "binary-drift"), Confidence: 1, RepairID: maat.SystemOneRepairBinaryDrift}}
			if tc.other {
				findings = append(findings, maat.ScreenFinding{ID: "independent", Severity: "major", Category: "host-health", Claim: "independent finding", Evidence: "fixture:independent", Confidence: 1})
			}
			detail := ""
			if !tc.floor {
				detail = "fixture: local diagnostic failed"
			}
			verdict, err := maat.Screen(maat.SystemOneScreen{
				Subject:       maat.VerdictSubject{Kind: "host", Repo: "fixture-host", Ref: "diagnostic", HeadSHA: head},
				FeatherWeight: 72, Confidence: 1,
				Floor:    maat.FloorResult{Passed: tc.floor, Checks: []maat.FloorCheck{{Name: "local diagnostic", Passed: tc.floor, Detail: detail}}},
				Findings: findings, Model: maat.ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			collector := &casebookTestJournal{}
			screen, err := maat.RecordSystemOne(collector, "fixture", verdict)
			if err != nil {
				t.Fatal(err)
			}
			screen.Time = "2026-09-01T00:00:00Z"
			var rows []maat.Decision
			appendRow := func(row maat.Decision) {
				t.Helper()
				// Append normalizes a value copy; pin the same canonical host
				// before retaining the independent in-memory comparison rows.
				row.Host = "fixture"
				if err := journal.Append(row); err != nil {
					t.Fatal(err)
				}
				rows = append(rows, row)
			}
			appendRow(screen)
			for i := 0; i < 60; i++ {
				appendRow(maat.Decision{Time: fmt.Sprintf("2027-01-01T00:%02d:00Z", i), Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "unrelated", Determination: "grant", Why: "newer noise", Evidence: fmt.Sprintf("noise:%d", i)})
			}
			attempt := maat.Decision{Time: "2026-09-01T00:01:00Z", Host: "fixture", Kind: "diagnostic repair attempt", Requester: "fixture", Resource: "binary-drift", Assessed: "inert updater", Determination: "in_progress", Why: "fixture attempt", OriginEvidence: screen.Evidence, Evidence: fmt.Sprintf("diagnostic-repair-attempt:sha256=%x", sha256.Sum256([]byte(screen.Evidence)))}
			appendRow(attempt)
			linkedEvidence := attempt.Evidence
			if tc.outcome != "" {
				repair, err := maat.BuildDiagnosticRepairDecision("fixture", maat.DiagnosticRepair{Check: "binary-drift", Operation: "inert updater", Before: "drift", After: "fixture observation", Determination: tc.outcome, Detail: "inert history result", OriginEvidence: screen.Evidence})
				if err != nil {
					t.Fatal(err)
				}
				repair.Time = "2026-09-01T00:02:00Z"
				appendRow(repair)
				linkedEvidence = repair.Evidence
			}
			// Newer lookalike links must not affect the exact source screen.
			wrong := attempt
			wrong.Time, wrong.Kind, wrong.Determination, wrong.Evidence = "2028-01-01T00:00:00Z", "diagnostic verified repair", "resolved", "receipt:wrong-resource"
			wrong.Resource = "liveness-watch"
			appendRow(wrong)
			wrong.Resource, wrong.OriginEvidence, wrong.Evidence = "binary-drift", "screen:unrelated", "receipt:wrong-origin"
			appendRow(wrong)
			plain, err := maat.Screen(maat.SystemOneScreen{
				Subject:       maat.VerdictSubject{Kind: "commit", Repo: "fixture/repo", Ref: "main", HeadSHA: strings.Repeat("c", 40)},
				FeatherWeight: 80, Confidence: 1,
				Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "fixture", Passed: true}}},
				Model: maat.ModelStamp{Provider: "local:fixture", Version: "v1", Local: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			newer, err := maat.RecordSystemOne(collector, "fixture", plain)
			if err != nil {
				t.Fatal(err)
			}
			newer.Time = "2029-01-01T00:00:00Z"
			appendRow(newer)

			check := func(result HistorySearchResult) Case {
				t.Helper()
				if len(result.View.Cases) != 1 || result.Window.Total == nil || *result.Window.Total != 1 || result.Window.CandidateMatches != 1 || !result.Window.TotalComplete || result.Window.HasMore {
					t.Fatalf("linked qualification/counts = %+v", result)
				}
				got := result.View.Cases[0]
				if got.Evidence != screen.Evidence || got.Status != tc.status || (got.VerifiedRepairEvidence != "") != tc.verified || got.Resolution != "" {
					t.Fatalf("updater projection = %+v", got)
				}
				if tc.outcome == "" {
					if got.PendingRepairAttemptEvidence != linkedEvidence || got.RepairOutcomeEvidence != "" {
						t.Fatalf("pending projection = %+v", got)
					}
				} else if got.RepairOutcomeEvidence != linkedEvidence || got.PendingRepairAttemptEvidence != "" {
					t.Fatalf("terminal outcome did not supersede pending = %+v", got)
				}
				if tc.action == "" {
					if got.NextAction != nil {
						t.Fatalf("closed binary screen has action: %+v", got)
					}
				} else if got.NextAction == nil || got.NextAction.ActionID != tc.action {
					t.Fatalf("typed history route = %+v", got)
				}
				return got
			}
			exact := check(completeRepairHistorySearch(t, journal, HistoryQuery{Query: Query{Limit: 1}, Evidence: screen.Evidence}))
			filtered := check(completeRepairHistorySearch(t, journal, HistoryQuery{Query: Query{Kind: "system one screen", Status: tc.status, Text: linkedEvidence, Limit: 1}, Scope: maat.HistoryScopeSystemOne}))
			inMemory, found := casebookCaseByEvidence(Build(rows).Cases, screen.Evidence)
			if !found || !reflect.DeepEqual(exact, filtered) || !reflect.DeepEqual(exact, inMemory) {
				t.Fatalf("projection differs between memory/filter/page: %+v / %+v / %+v", inMemory, filtered, exact)
			}
			if action := exact.NextAction; action != nil {
				check(completeRepairHistorySearch(t, journal, HistoryQuery{Query: Query{Kind: "system one screen", Text: action.Title, Limit: 1}, Scope: maat.HistoryScopeSystemOne}))
			}
			opposite := StatusOpen
			if tc.status == StatusOpen {
				opposite = StatusResolved
			}
			refused := completeRepairHistorySearch(t, journal, HistoryQuery{Query: Query{Status: opposite, Limit: 1}, Evidence: screen.Evidence})
			if len(refused.View.Cases) != 0 || refused.Window.CandidateMatches != 0 {
				t.Fatalf("wrong status matched: %+v", refused)
			}
		})
	}
}
