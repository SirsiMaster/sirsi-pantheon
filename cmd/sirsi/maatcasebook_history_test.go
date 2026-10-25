package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
)

func TestMaatCasebookCommandFindsOlderScopedSystemOneRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	journal := &maat.FileDecisionJournal{Path: path}
	verdict, err := maat.Screen(maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "commit", Repo: "fixture/repo", Ref: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		FeatherWeight: 80, Confidence: 1,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "fixture", Passed: true}}},
		Model: maat.ModelStamp{Provider: "local:fixture", Version: "fixture", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	const evidence = "maat-system-one:older-screen"
	if err := journal.Append(maat.Decision{
		Time: "2026-09-01T00:00:00Z", Host: "fixture", Kind: "system one screen", Requester: "fixture",
		Assessed: "old screen", Determination: string(verdict.Gate), Why: "old retained screen", Evidence: evidence, SystemOne: &verdict,
	}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 60; index++ {
		if err := journal.Append(maat.Decision{
			Time: time.Date(2026, 10, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "reservation",
			Determination: "grant", Why: "unrelated", Evidence: "reservation:noise",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Optional wire-proof custody exports the real 61-row journal, never a
	// projected response. Main supplies a unique path in its owned fixture root.
	if destination := os.Getenv("PANTHEON_TEST_MAAT_HISTORY_JOURNAL_FIXTURE"); destination != "" {
		if !filepath.IsAbs(destination) {
			t.Fatal("history journal fixture export requires an absolute owned fixture path")
		}
		source, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		const maxFixtureBytes = 1 << 20
		raw, readErr := io.ReadAll(io.LimitReader(source, maxFixtureBytes+1))
		closeErr := source.Close()
		if readErr != nil || closeErr != nil || len(raw) == 0 || len(raw) > maxFixtureBytes {
			t.Fatalf("read bounded history journal fixture: bytes=%d read=%v close=%v", len(raw), readErr, closeErr)
		}
		fixture, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := fixture.Chmod(0o600); err != nil {
			closeErr := fixture.Close()
			t.Fatalf("set private history fixture mode: %v; close=%v", err, closeErr)
		}
		written, writeErr := fixture.Write(raw)
		closeErr = fixture.Close()
		if writeErr != nil || written != len(raw) || closeErr != nil {
			t.Fatalf("export complete history journal fixture: wrote=%d want=%d write=%v close=%v", written, len(raw), writeErr, closeErr)
		}
	}
	oldFactory := newMaatDecisionJournal
	oldKind, oldStatus, oldLimit := maatCasebookKind, maatCasebookStatus, maatCasebookLimit
	oldScope, oldEvidence, oldCursor, oldJSON := maatCasebookScope, maatCasebookEvidence, maatCasebookCursor, maatJSON
	oldStdout := os.Stdout
	t.Cleanup(func() {
		newMaatDecisionJournal = oldFactory
		maatCasebookKind, maatCasebookStatus, maatCasebookLimit = oldKind, oldStatus, oldLimit
		maatCasebookScope, maatCasebookEvidence, maatCasebookCursor, maatJSON = oldScope, oldEvidence, oldCursor, oldJSON
		os.Stdout = oldStdout
	})
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatCasebookKind, maatCasebookStatus, maatCasebookLimit = "", "", 5
	maatCasebookScope, maatCasebookEvidence, maatCasebookCursor, maatJSON = maat.HistoryScopeSystemOne, "", "", false
	maatJSON = false
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	if err := maatCasebookCmd.RunE(maatCasebookCmd, nil); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), evidence) || strings.Contains(string(output), "no Ma'at cases match") {
		t.Fatalf("scoped CLI output did not retain older evidence: %s", output)
	}

	maatCasebookScope, maatCasebookEvidence, maatCasebookLimit, maatJSON = maat.HistoryScopeAll, evidence, 2, true
	jsonReader, jsonWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = jsonWriter
	if err := maatCasebookCmd.RunE(maatCasebookCmd, nil); err != nil {
		jsonWriter.Close()
		t.Fatal(err)
	}
	if err := jsonWriter.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldStdout
	encodedProjection, err := io.ReadAll(jsonReader)
	jsonReader.Close()
	if err != nil {
		t.Fatal(err)
	}
	var projection maatCasebookProjection
	if err := json.Unmarshal(encodedProjection, &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection.Cases) != 1 || projection.Cases[0].Evidence != evidence || !projection.History.TotalComplete || !projection.History.CandidateMatchesComplete || projection.History.CandidateMatches != 1 || !projection.History.EvidenceMatchesComplete || projection.History.EvidenceMatches == nil || *projection.History.EvidenceMatches != 1 {
		t.Fatalf("exact-evidence CLI query did not return complete historical evidence: %+v", projection)
	}
}

func TestMaatCasebookExactEvidenceQueryDoesNotUseRecentWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	journal := &maat.FileDecisionJournal{Path: path}
	for index := 0; index < 55; index++ {
		if err := journal.Append(maat.Decision{
			Time: time.Date(2026, 10, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "reservation",
			Determination: "grant", Why: "unrelated", Evidence: "reservation:noise",
		}); err != nil {
			t.Fatal(err)
		}
	}
	const evidence = "exact:historical"
	if err := journal.Append(maat.Decision{
		Time: "2026-09-01T00:00:00Z", Host: "fixture", Kind: "diagnostic", Requester: "fixture",
		Assessed: "historical diagnostic", Determination: "block", Why: "needs review", Evidence: evidence,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := casebookSearchExact(journal, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.View.Cases) != 1 || result.View.Cases[0].Evidence != evidence || result.Window.TotalComplete != true {
		t.Fatalf("exact historical result = %+v", result)
	}
}

func casebookSearchExact(journal *maat.FileDecisionJournal, evidence string) (casebook.HistorySearchResult, error) {
	return casebook.SearchHistory(journal, casebook.HistoryQuery{
		Query: casebook.Query{Limit: 100}, Scope: maat.HistoryScopeAll, Evidence: evidence,
	})
}

func TestMaatAcceptResolutionHistoricalCommandConfirmsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	journal := &maat.FileDecisionJournal{Path: path}
	review, err := maat.RecordDiagnosticOwnerReview(journal, "fixture", maat.DiagnosticOwnerReview{Check: "old review", Message: "retained source evidence"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for index := 0; index < 1001; index++ {
		if err := encoder.Encode(maat.Decision{Time: "2099-01-01T00:00:00Z", Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "unrelated", Determination: "grant", Why: "later unrelated traffic", Evidence: "noise:" + strconv.Itoa(index)}); err != nil {
			file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	oldFactory := newMaatDecisionJournal
	oldEvidence, oldNote, oldConfirm := maatAcceptanceEvidence, maatAcceptanceNote, maatAcceptanceConfirm
	t.Cleanup(func() {
		newMaatDecisionJournal = oldFactory
		maatAcceptanceEvidence, maatAcceptanceNote, maatAcceptanceConfirm = oldEvidence, oldNote, oldConfirm
	})
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatAcceptanceEvidence, maatAcceptanceNote, maatAcceptanceConfirm = review.Evidence, "explicit owner conclusion", false
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := maatAcceptResolutionCmd.RunE(maatAcceptResolutionCmd, nil); err == nil {
		t.Fatal("historical owner acceptance ran without confirmation")
	}
	unchanged, err := os.Stat(path)
	if err != nil || unchanged.Size() != before.Size() {
		t.Fatal("unconfirmed historical action changed the journal")
	}
	maatAcceptanceConfirm = true
	if err := maatAcceptResolutionCmd.RunE(maatAcceptResolutionCmd, nil); err != nil {
		t.Fatal(err)
	}
	committed, err := os.Stat(path)
	if err != nil || committed.Size() <= before.Size() {
		t.Fatal("confirmed historical action did not append its outcome")
	}
	if err := maatAcceptResolutionCmd.RunE(maatAcceptResolutionCmd, nil); err == nil {
		t.Fatal("actual historical command replayed owner acceptance")
	}
	after, err := os.Stat(path)
	if err != nil || after.Size() != committed.Size() {
		t.Fatal("refused historical replay changed the journal")
	}
}
