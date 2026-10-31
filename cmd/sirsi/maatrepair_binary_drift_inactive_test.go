package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
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
	if err != nil || len(rows) != 2 || rows[1].BinaryDriftInactiveDisposition == nil || rows[1].Determination != "accepted_not_repaired" {
		t.Fatalf("typed disposition was not retained: rows=%d err=%v", len(rows), err)
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
}
