package casebook

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestSearchHistoryValidatesInactiveDispositionAcrossPagedLinks(t *testing.T) {
	for _, test := range []struct {
		name           string
		mixed          bool
		resolutionHost string
		duplicate      bool
		wantResolved   bool
	}{
		{name: "exact source with linked acceptance beyond candidate window", resolutionHost: "fixture-host", wantResolved: true},
		{name: "wrong host", resolutionHost: "other-host"},
		{name: "mixed source", mixed: true, resolutionHost: "fixture-host"},
		{name: "duplicate origin outside one-row window", resolutionHost: "fixture-host", duplicate: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
			head := strings.Repeat("a", 40)
			findings := []maat.ScreenFinding{{
				ID: maat.CanonicalHostFindingID("binary-drift"), Severity: "major", Category: "host-health",
				Claim: "binary drift", Evidence: maat.CanonicalHostFindingEvidence(head, "binary-drift"), Confidence: 1,
			}}
			if test.mixed {
				findings = append(findings, maat.ScreenFinding{ID: "independent", Severity: "major", Category: "host-health", Claim: "other issue", Evidence: "fixture:other", Confidence: 1})
			}
			verdict, err := maat.Screen(maat.SystemOneScreen{
				Subject:       maat.VerdictSubject{Kind: "host", Repo: "fixture-host", Ref: "diagnostic", HeadSHA: head},
				FeatherWeight: 90, Confidence: 1, Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "fixture", Passed: true}}},
				Findings: findings, Model: maat.ModelStamp{Provider: "local:deterministic", Version: "fixture", Local: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			source, err := maat.RecordSystemOne(journal, "fixture", verdict)
			if err != nil {
				t.Fatal(err)
			}
			if test.duplicate {
				if err := journal.Append(source); err != nil {
					t.Fatal(err)
				}
			}
			disposition, err := maat.BuildBinaryDriftInactiveDisposition("fixture owner", maat.BinaryDriftInactiveDisposition{
				OriginEvidence: source.Evidence, Host: test.resolutionHost,
				ObservedFromUTC: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC), ObservedThroughUTC: time.Date(2026, 10, 31, 12, 0, 1, 0, time.UTC),
				ObservationSHA256: strings.Repeat("b", 64),
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := journal.Append(disposition); err != nil {
				t.Fatal(err)
			}

			query := HistoryQuery{Query: Query{Kind: "system one screen", Status: StatusResolved, Limit: 1}, Evidence: source.Evidence}
			resolved := completeRepairHistorySearch(t, journal, query)
			if test.wantResolved {
				if len(resolved.View.Cases) != 1 || resolved.View.Cases[0].Evidence != source.Evidence || resolved.View.Cases[0].Status != StatusResolved || resolved.View.Cases[0].BinaryDriftInactiveDisposition == nil {
					t.Fatalf("linked exact source did not resolve through paged history: %+v", resolved.View.Cases)
				}
				return
			}
			if len(resolved.View.Cases) != 0 {
				t.Fatalf("invalid typed acceptance passed the resolved filter: %+v", resolved.View.Cases)
			}
			open := completeRepairHistorySearch(t, journal, HistoryQuery{Query: Query{Kind: "system one screen", Limit: 1}, Evidence: source.Evidence})
			if len(open.View.Cases) != 1 || open.View.Cases[0].Status != StatusOpen {
				t.Fatalf("invalid typed acceptance closed the source in paged output: %+v", open.View.Cases)
			}
		})
	}
}

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
