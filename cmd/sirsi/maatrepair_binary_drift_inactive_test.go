package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

func binaryDriftInactiveFixture(t *testing.T, host string, mixed bool) (*maat.FileDecisionJournal, string) {
	t.Helper()
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "casebook.jsonl")}
	report := &guard.DoctorReport{Timestamp: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC), Score: 90, Findings: []guard.DiagnosticFinding{{
		Check: "binary-drift", Severity: guard.SeverityWarn, Message: "fixture drift",
	}}}
	if mixed {
		report.Findings = append(report.Findings, guard.DiagnosticFinding{Check: "Disk Space", Severity: guard.SeverityWarn, Message: "unrelated fixture finding"})
	}
	screen, _, err := maatHostSystemOneScreen(report, host, 1)
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := maat.Screen(screen)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := maat.RecordSystemOne(journal, "fixture", verdict)
	if err != nil {
		t.Fatal(err)
	}
	return journal, decision.Evidence
}

func runInactiveDispositionFixture(journal *maat.FileDecisionJournal, evidence, host string, drift *selfupdate.DriftReport, scanErr error) (maatBinaryDriftInactiveDispositionReceipt, error) {
	times := []time.Time{time.Date(2026, 10, 31, 12, 1, 0, 0, time.UTC), time.Date(2026, 10, 31, 12, 1, 2, 0, time.UTC)}
	index := 0
	return recordMaatBinaryDriftInactiveDispositionWith(context.Background(), journal, evidence,
		func() (string, error) { return host, nil },
		func() time.Time { value := times[index]; index++; return value },
		func(context.Context) (*selfupdate.DriftReport, error) { return drift, scanErr })
}

func TestRecordBinaryDriftInactiveDispositionRequiresExactFreshClearAndReplaysDurably(t *testing.T) {
	journal, evidence := binaryDriftInactiveFixture(t, "fixture-host", false)
	clear := &selfupdate.DriftReport{Healthy: true}
	first, err := runInactiveDispositionFixture(journal, evidence, "fixture-host", clear, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.CaseEvidence != evidence || first.FindingState != "no longer active" || first.RepairPerformed || first.Host != "fixture-host" || first.ObservedThroughUTC.Before(first.ObservedFromUTC) {
		t.Fatalf("unexpected inactive disposition receipt: %+v", first)
	}
	rows, _, err := journal.RecentTolerant(10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("typed disposition was not retained: rows=%d err=%v", len(rows), err)
	}
	var retained *maat.Decision
	for index := range rows {
		if rows[index].Evidence == first.DispositionEvidence {
			retained = &rows[index]
			break
		}
	}
	if retained == nil || retained.BinaryDriftInactiveDisposition == nil || retained.Determination != "accepted_not_repaired" {
		t.Fatalf("typed disposition was not retained in newest-first rows: %+v", rows)
	}
	replay, err := runInactiveDispositionFixture(journal, evidence, "fixture-host", clear, nil)
	if err != nil || replay.DispositionEvidence != first.DispositionEvidence || replay.ObservedFromUTC != first.ObservedFromUTC {
		t.Fatalf("durable duplicate replay changed the accepted disposition: receipt=%+v err=%v", replay, err)
	}
	rows, _, err = journal.RecentTolerant(10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("duplicate replay appended another row: rows=%d err=%v", len(rows), err)
	}
}

func TestRecordBinaryDriftInactiveDispositionCanceledBeforeObservationDoesNotAppend(t *testing.T) {
	for _, test := range []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		want    error
	}{
		{name: "canceled", context: func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		}, want: context.Canceled},
		{name: "expired deadline", context: func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			journal, evidence := binaryDriftInactiveFixture(t, "fixture-host", false)
			ctx, cancel := test.context()
			cancel()
			scanCalled := false
			_, err := recordMaatBinaryDriftInactiveDispositionWith(ctx, journal, evidence,
				func() (string, error) { return "fixture-host", nil },
				func() time.Time { return time.Date(2026, 10, 31, 12, 1, 0, 0, time.UTC) },
				func(context.Context) (*selfupdate.DriftReport, error) {
					scanCalled = true
					return &selfupdate.DriftReport{Healthy: true}, nil
				},
			)
			if !errors.Is(err, test.want) || scanCalled {
				t.Fatalf("expired confirmation context reached observation: scan=%t err=%v", scanCalled, err)
			}
			rows, _, readErr := journal.RecentTolerant(10)
			if readErr != nil || len(rows) != 1 {
				t.Fatalf("canceled recheck appended a disposition: rows=%d err=%v", len(rows), readErr)
			}
		})
	}
}

func TestRecordBinaryDriftInactiveDispositionRefusesActiveUnknownWrongHostAndMixedCase(t *testing.T) {
	tests := []struct {
		name  string
		host  string
		mixed bool
		drift *selfupdate.DriftReport
		err   error
	}{
		{name: "active", host: "fixture-host", drift: &selfupdate.DriftReport{Healthy: false}},
		{name: "unknown", host: "fixture-host"},
		{name: "observation failure", host: "fixture-host", drift: &selfupdate.DriftReport{Healthy: true}, err: errors.New("fixture observation failure")},
		{name: "wrong host", host: "other-host", drift: &selfupdate.DriftReport{Healthy: true}},
		{name: "mixed screen", host: "fixture-host", mixed: true, drift: &selfupdate.DriftReport{Healthy: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			journal, evidence := binaryDriftInactiveFixture(t, "fixture-host", test.mixed)
			_, err := runInactiveDispositionFixture(journal, evidence, test.host, test.drift, test.err)
			if err == nil {
				t.Fatal("unsafe or unknown source was accepted")
			}
			rows, _, readErr := journal.RecentTolerant(10)
			if readErr != nil || len(rows) != 1 {
				t.Fatalf("refused action appended a disposition: rows=%d err=%v", len(rows), readErr)
			}
		})
	}
}

func TestRecordBinaryDriftInactiveDispositionRejectsUnusableBuildReportSiblings(t *testing.T) {
	for _, sibling := range []selfupdate.Sibling{
		{Info: modversion.Info{Binary: "sirsi", Version: ""}, Err: "fixture probe error"},
		{Info: modversion.Info{Binary: "sirsi", Version: ""}},
		{Info: modversion.Info{Binary: "sirsi", Version: "dev"}},
	} {
		t.Run(sibling.Err+"/"+sibling.Version, func(t *testing.T) {
			report := selfupdate.BuildReport(modversion.Info{Binary: "sirsi", Version: "1.2.3"}, []selfupdate.Sibling{sibling}, "")
			if !report.Healthy {
				t.Fatal("fixture no longer demonstrates BuildReport's healthy-but-unusable sibling shape")
			}
			journal, evidence := binaryDriftInactiveFixture(t, "fixture-host", false)
			if _, err := runInactiveDispositionFixture(journal, evidence, "fixture-host", report, nil); err == nil {
				t.Fatal("inactive disposition accepted an unusable sibling observation")
			}
			rows, _, err := journal.RecentTolerant(10)
			if err != nil || len(rows) != 1 {
				t.Fatalf("unusable observation appended a disposition: rows=%d err=%v", len(rows), err)
			}
		})
	}
}

func TestRecordBinaryDriftInactiveDispositionRefusesChangedEvidenceAndJournalFailure(t *testing.T) {
	t.Run("changed evidence", func(t *testing.T) {
		journal, evidence := binaryDriftInactiveFixture(t, "fixture-host", false)
		_, err := runInactiveDispositionFixture(journal, evidence+"stale", "fixture-host", &selfupdate.DriftReport{Healthy: true}, nil)
		if err == nil {
			t.Fatal("stale evidence was accepted")
		}
	})
	t.Run("strict journal failure", func(t *testing.T) {
		journal, evidence := binaryDriftInactiveFixture(t, "fixture-host", false)
		if err := os.WriteFile(journal.Path, []byte("{not-json}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := runInactiveDispositionFixture(journal, evidence, "fixture-host", &selfupdate.DriftReport{Healthy: true}, nil)
		if err == nil {
			t.Fatal("corrupt retained journal was ignored")
		}
	})
	t.Run("prior owner review", func(t *testing.T) {
		journal, evidence := binaryDriftInactiveFixture(t, "fixture-host", false)
		if _, err := maat.RecordDiagnosticOwnerReview(journal, "fixture", maat.DiagnosticOwnerReview{Check: "binary-drift", Message: "already reviewed", OriginEvidence: evidence}); err != nil {
			t.Fatal(err)
		}
		_, err := runInactiveDispositionFixture(journal, evidence, "fixture-host", &selfupdate.DriftReport{Healthy: true}, nil)
		if err == nil {
			t.Fatal("prior owner review was overwritten")
		}
	})
	t.Run("prior updater attempt", func(t *testing.T) {
		journal, evidence := binaryDriftInactiveFixture(t, "fixture-host", false)
		if err := journal.Append(maat.Decision{
			Time: time.Date(2026, 10, 31, 12, 0, 1, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture-host", Kind: "diagnostic repair attempt", Requester: "fixture",
			Resource: "binary-drift", Assessed: "fixture attempt", Determination: "started",
			Why: "inert prior attempt", Evidence: "fixture:attempt", OriginEvidence: evidence,
		}); err != nil {
			t.Fatal(err)
		}
		_, err := runInactiveDispositionFixture(journal, evidence, "fixture-host", &selfupdate.DriftReport{Healthy: true}, nil)
		if err == nil {
			t.Fatal("prior updater attempt was overwritten")
		}
	})
}

func TestRecordBinaryDriftInactiveDispositionRechecksJournalAfterFreshScan(t *testing.T) {
	tests := []struct {
		name   string
		append func(*testing.T, *maat.FileDecisionJournal, string)
	}{
		{
			name: "duplicate source evidence",
			append: func(t *testing.T, journal *maat.FileDecisionJournal, evidence string) {
				t.Helper()
				rows, _, err := journal.RecentTolerant(10)
				if err != nil || len(rows) != 1 || rows[0].Evidence != evidence {
					t.Fatalf("read source fixture: rows=%d err=%v", len(rows), err)
				}
				if err := journal.Append(rows[0]); err != nil {
					t.Fatalf("append duplicate source fixture: %v", err)
				}
			},
		},
		{
			name: "new owner review",
			append: func(t *testing.T, journal *maat.FileDecisionJournal, evidence string) {
				t.Helper()
				if _, err := maat.RecordDiagnosticOwnerReview(journal, "fixture", maat.DiagnosticOwnerReview{
					Check: "binary-drift", Message: "appended during recheck", OriginEvidence: evidence,
				}); err != nil {
					t.Fatalf("append owner review fixture: %v", err)
				}
			},
		},
		{
			name: "different inactive disposition",
			append: func(t *testing.T, journal *maat.FileDecisionJournal, evidence string) {
				t.Helper()
				disposition, err := maat.BuildBinaryDriftInactiveDisposition("fixture", maat.BinaryDriftInactiveDisposition{
					OriginEvidence: evidence, Host: "fixture-host",
					ObservedFromUTC:    time.Date(2026, 10, 31, 12, 0, 1, 0, time.UTC),
					ObservedThroughUTC: time.Date(2026, 10, 31, 12, 0, 2, 0, time.UTC),
					ObservationSHA256:  strings.Repeat("b", 64),
				})
				if err != nil {
					t.Fatalf("build typed disposition fixture: %v", err)
				}
				if err := journal.Append(disposition); err != nil {
					t.Fatalf("append typed disposition fixture: %v", err)
				}
			},
		},
		{
			name: "new malformed row",
			append: func(t *testing.T, journal *maat.FileDecisionJournal, _ string) {
				t.Helper()
				file, err := os.OpenFile(journal.Path, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatalf("open fixture journal: %v", err)
				}
				if _, err := file.WriteString("{invalid appended row}\n"); err != nil {
					_ = file.Close()
					t.Fatalf("append malformed fixture row: %v", err)
				}
				if err := file.Close(); err != nil {
					t.Fatalf("close malformed fixture row: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			journal, evidence := binaryDriftInactiveFixture(t, "fixture-host", false)
			calls := 0
			_, err := recordMaatBinaryDriftInactiveDispositionWith(context.Background(), journal, evidence,
				func() (string, error) { return "fixture-host", nil },
				func() time.Time { return time.Date(2026, 10, 31, 12, 1, 0, 0, time.UTC) },
				func(context.Context) (*selfupdate.DriftReport, error) {
					calls++
					test.append(t, journal, evidence)
					return &selfupdate.DriftReport{Healthy: true}, nil
				},
			)
			if calls != 1 || err == nil {
				t.Fatalf("fresh scan mutation was not refused by current journal validation: calls=%d err=%v", calls, err)
			}
			if test.name != "new malformed row" {
				rows, _, readErr := journal.RecentTolerant(10)
				if readErr != nil || len(rows) != 2 {
					t.Fatalf("refusal changed retained rows unexpectedly: rows=%d err=%v", len(rows), readErr)
				}
			}
		})
	}
}
