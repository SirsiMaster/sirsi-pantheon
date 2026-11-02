package maat

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFileDecisionJournalAppendIdempotentReadsBackExactEvidence(t *testing.T) {
	journal := &FileDecisionJournal{Path: t.TempDir() + "/decisions.jsonl"}
	decision, err := BuildDiagnosticRepairDecision("sirsi maat repair", DiagnosticRepair{
		Check: "liveness-watch", Operation: "bounded action", Before: "absent", After: "present",
		Determination: "resolved", Detail: "attempt 123",
	})
	if err != nil {
		t.Fatal(err)
	}
	decision.Evidence = "maat-liveness-watch-attempt:123:" + decision.Evidence
	if err := journal.AppendIdempotent(decision); err != nil {
		t.Fatal(err)
	}
	if err := journal.AppendIdempotent(decision); err != nil {
		t.Fatalf("exact retry was not idempotent: %v", err)
	}
	rows, err := journal.Recent(10)
	if err != nil || len(rows) != 1 || rows[0].Evidence != decision.Evidence {
		t.Fatalf("journal rows = %#v, err=%v", rows, err)
	}
	changed := decision
	changed.Why += " changed"
	if err := journal.AppendIdempotent(changed); err == nil || !strings.Contains(err.Error(), "different decision") {
		t.Fatalf("evidence collision error = %v", err)
	}
}

func TestFileDecisionJournalIdempotentReadbackRejectsMalformedTail(t *testing.T) {
	path := t.TempDir() + "/decisions.jsonl"
	journal := &FileDecisionJournal{Path: path}
	decision, err := BuildDiagnosticRepairDecision("sirsi maat repair", DiagnosticRepair{
		Check: "liveness-watch", Operation: "bounded action", Before: "absent", After: "present",
		Determination: "resolved", Detail: "attempt 456",
	})
	if err != nil {
		t.Fatal(err)
	}
	decision.Evidence = "maat-liveness-watch-attempt:456:" + decision.Evidence
	if err := journal.Append(decision); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{broken tail}\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := journal.AppendIdempotent(decision); err == nil {
		t.Fatal("idempotent readback accepted a valid match ahead of a malformed journal tail")
	}
}

type fakeDecisionAppendFile struct {
	shortWrite bool
	writeErr   error
	syncErr    error
	closeErr   error
	calls      []string
}

func (f *fakeDecisionAppendFile) Write(record []byte) (int, error) {
	f.calls = append(f.calls, "write")
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.shortWrite {
		return len(record) - 1, nil
	}
	return len(record), nil
}

func (f *fakeDecisionAppendFile) Sync() error {
	f.calls = append(f.calls, "sync")
	return f.syncErr
}

func (f *fakeDecisionAppendFile) Close() error {
	f.calls = append(f.calls, "close")
	return f.closeErr
}

func TestAppendDecisionRecordCompletesWriteSyncCloseAndPrioritizesFailures(t *testing.T) {
	writeFailure, syncFailure, closeFailure := errors.New("write failed"), errors.New("sync failed"), errors.New("close failed")
	for _, tt := range []struct {
		name       string
		shortWrite bool
		writeErr   error
		syncErr    error
		closeErr   error
		wantErr    error
		wantStage  string
		wantCalls  string
	}{
		{name: "complete", wantCalls: "write,sync,close"},
		{name: "write error precedes close", writeErr: writeFailure, syncErr: syncFailure, closeErr: closeFailure, wantErr: writeFailure, wantStage: "append", wantCalls: "write,close"},
		{name: "short write is rejected", shortWrite: true, closeErr: closeFailure, wantErr: io.ErrShortWrite, wantStage: "append", wantCalls: "write,close"},
		{name: "sync error precedes close", syncErr: syncFailure, closeErr: closeFailure, wantErr: syncFailure, wantStage: "sync", wantCalls: "write,sync,close"},
		{name: "close error is returned", closeErr: closeFailure, wantErr: closeFailure, wantStage: "close", wantCalls: "write,sync,close"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := &fakeDecisionAppendFile{shortWrite: tt.shortWrite, writeErr: tt.writeErr, syncErr: tt.syncErr, closeErr: tt.closeErr}
			err := appendDecisionRecord(file, []byte("fixture record\n"))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("append error = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
			if tt.wantStage != "" && !strings.Contains(err.Error(), tt.wantStage) {
				t.Fatalf("append error %q does not identify stage %q", err, tt.wantStage)
			}
			if got := strings.Join(file.calls, ","); got != tt.wantCalls {
				t.Fatalf("I/O calls = %q, want %q", got, tt.wantCalls)
			}
		})
	}
}

func TestFileDecisionJournalAppendsAndReturnsNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "decisions.jsonl")
	j := &FileDecisionJournal{Path: path}
	for _, d := range []Decision{
		{Time: "2026-09-26T10:00:00Z", Host: "m5", Kind: "reservation", Requester: "alpha", Resource: "m5", Assessed: "free", Determination: "grant", Why: "no overlap"},
		{Time: "2026-09-26T10:01:00Z", Host: "m5", Kind: "reservation", Requester: "beta", Resource: "m5", Assessed: "held", Affected: "alpha", Determination: "refuse", Why: "active reservation"},
	} {
		if err := j.Append(d); err != nil {
			t.Fatal(err)
		}
	}
	got, err := j.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Requester != "beta" || got[0].Affected != "alpha" {
		t.Fatalf("Recent(1) = %+v, want newest refusal", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestFileDecisionJournalBinaryDriftClaimIsOperationWideAndReopensAfterTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "decisions.jsonl")
	first := &FileDecisionJournal{Path: path}
	second := &FileDecisionJournal{Path: path}
	claim := func(caseID string) Decision {
		origin := "screen:" + caseID
		return Decision{Time: "2026-10-24T12:00:00Z", Host: "fixture", Kind: "diagnostic repair attempt", Requester: "sirsi maat repair binary-drift", Resource: "binary-drift", Assessed: "confirmed supported Homebrew updater", Determination: "in_progress", Why: "durable claim", Evidence: fmt.Sprintf("diagnostic-repair-attempt:sha256=%x", sha256.Sum256([]byte(origin))), OriginEvidence: origin}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, item := range []struct {
		journal *FileDecisionJournal
		caseID  string
	}{{first, "a"}, {second, "b"}} {
		wg.Add(1)
		go func(journal *FileDecisionJournal, caseID string) {
			defer wg.Done()
			<-start
			appended, err := journal.AppendBinaryDriftAttemptOnce(claim(caseID))
			if err == nil && !appended {
				err = fmt.Errorf("claim unexpectedly deduplicated")
			}
			results <- err
		}(item.journal, item.caseID)
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	var selected Decision
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent distinct-screen updater claims admitted %d; want exactly one", successes)
	}
	if appended, err := second.AppendBinaryDriftAttemptOnce(claim("fresh-screen-during-active-attempt")); err == nil || appended {
		t.Fatalf("fresh screen bypassed unresolved operation claim: appended=%t err=%v", appended, err)
	}
	rows, err := first.Recent(10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("concurrent claim history = %+v, %v", rows, err)
	}
	selected = rows[0]
	if err := first.Append(Decision{Time: "2026-10-24T12:01:00Z", Host: "fixture", Kind: "diagnostic verified repair", Requester: "sirsi maat repair binary-drift", Resource: "binary-drift", Assessed: "fresh local observation", Determination: "failed", Why: "terminal fixture result", Evidence: "receipt:" + selected.OriginEvidence, OriginEvidence: selected.OriginEvidence}); err != nil {
		t.Fatal(err)
	}
	if appended, err := second.AppendBinaryDriftAttemptOnce(claim("c")); err != nil || !appended {
		t.Fatalf("new reviewed operation after terminal result = %t, %v", appended, err)
	}
}

func TestFileDecisionJournalAppendOnceIsCrossCallIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "decisions.jsonl")
	journal := &FileDecisionJournal{Path: path}
	decision := Decision{
		Time: "2026-10-24T12:00:00Z", Host: "fixture", Kind: "knowledge owner review",
		Requester: "maat", Resource: "record-1", Assessed: "retained knowledge archive item",
		Determination: "accepted", Why: "owner review fixture", Evidence: "maat-knowledge-annotation:17",
	}
	appended, err := journal.AppendOnce(decision)
	if err != nil || !appended {
		t.Fatalf("first AppendOnce = %t, %v; want append", appended, err)
	}
	appended, err = journal.AppendOnce(decision)
	if err != nil || appended {
		t.Fatalf("retry AppendOnce = %t, %v; want existing row", appended, err)
	}
	rows, err := journal.Recent(10)
	if err != nil || len(rows) != 1 || rows[0].Evidence != decision.Evidence {
		t.Fatalf("idempotent history = %+v, %v", rows, err)
	}
}

func TestFileDecisionJournalStrictLookupFindsEvidenceOutsideRecentWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	journal := &FileDecisionJournal{Path: path}
	old := Decision{
		Time: "2026-10-24T12:00:00Z", Host: "fixture", Kind: "diagnostic repair attempt",
		Requester: "sirsi maat repair binary-drift", Resource: "binary-drift", Assessed: "confirmed supported Homebrew updater",
		Determination: "in_progress", Why: "durable launch claim", Evidence: "attempt:old", OriginEvidence: "maat-system-one:sha256:" + strings.Repeat("a", 64),
	}
	if err := journal.Append(old); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 105; i++ {
		if err := journal.Append(Decision{
			Time: "2026-10-24T12:01:00Z", Host: "fixture", Kind: "knowledge owner review",
			Requester: "maat", Resource: "fixture", Assessed: "retained fixture", Determination: "accepted", Why: "fixture row",
			Evidence: fmt.Sprintf("filler:%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if rows, err := journal.Recent(100); err != nil || len(rows) != 100 {
		t.Fatalf("recent-window fixture = %d rows, %v", len(rows), err)
	}
	got, found, err := journal.FindByEvidence(old.Kind, old.Evidence)
	if err != nil || !found || got.OriginEvidence != old.OriginEvidence {
		t.Fatalf("full-journal exact lookup = %+v, found=%t, err=%v", got, found, err)
	}
	found, err = journal.HasOriginDecision(old.Kind, old.Resource, old.OriginEvidence)
	if err != nil || !found {
		t.Fatalf("full-journal origin lookup = %t, %v", found, err)
	}
}

func TestFileDecisionJournalAppendOnceRefusesMalformedHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	original := []byte("{not json}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	decision := Decision{
		Time: "2026-10-24T12:00:00Z", Host: "fixture", Kind: "knowledge owner review",
		Requester: "maat", Resource: "record-1", Assessed: "retained knowledge archive item",
		Determination: "accepted", Why: "owner review fixture", Evidence: "maat-knowledge-annotation:17",
	}
	if appended, err := (&FileDecisionJournal{Path: path}).AppendOnce(decision); err == nil || appended {
		t.Fatalf("AppendOnce malformed history = %t, %v; want refusal", appended, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(original) {
		t.Fatalf("malformed history changed: %q (%v)", got, err)
	}
}

func TestFileDecisionJournalRejectsMalformedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	if err := os.WriteFile(path, []byte("{not json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&FileDecisionJournal{Path: path}).Recent(10); err == nil {
		t.Fatal("Recent accepted malformed JSONL")
	}
}

func TestFileDecisionJournalStrictReadReportsFirstInvalidLateRowWithoutContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	var contents bytes.Buffer
	valid, err := json.Marshal(Decision{
		Time: "2026-10-27T00:00:00Z", Host: "fixture", Kind: "reservation", Requester: "fixture",
		Assessed: "fixture", Determination: "grant", Why: "valid retained row",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5000; i++ {
		contents.Write(valid)
		contents.WriteByte('\n')
	}
	invalid, err := json.Marshal(Decision{
		Time: "2026-10-27T00:00:01Z", Host: "fixture", Kind: "reservation", Assessed: "fixture",
		Determination: "refuse", Why: "PRIVATE fixture body must not appear in error",
	})
	if err != nil {
		t.Fatal(err)
	}
	contents.Write(invalid)
	contents.WriteByte('\n')
	if err := os.WriteFile(path, contents.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = (&FileDecisionJournal{Path: path}).Recent(1)
	var validation *JournalValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("strict Recent error = %T %v, want typed validation error", err, err)
	}
	wantDigest := sha256.Sum256(invalid)
	if validation.Issue.Line != 5001 || validation.Issue.Digest != fmt.Sprintf("sha256:%x", wantDigest) || validation.Issue.Reason != "required field is missing" {
		t.Fatalf("strict issue = %+v, want line 5001, exact digest, and content-independent required-field category", validation.Issue)
	}
	if strings.Contains(err.Error(), "PRIVATE fixture body") {
		t.Fatal("strict validation error exposed rejected journal contents")
	}
}

func TestFileDecisionJournalInvalidTimestampReasonDoesNotExposeValue(t *testing.T) {
	const privateTimestamp = "PRIVATE-TIMESTAMP-SENTINEL-2042-13-99"
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	invalid, err := json.Marshal(Decision{
		Time: privateTimestamp, Host: "fixture", Kind: "reservation", Requester: "fixture",
		Assessed: "fixture", Determination: "refuse", Why: "invalid timestamp fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(invalid, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = (&FileDecisionJournal{Path: path}).Recent(1)
	var validation *JournalValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("strict Recent error = %T %v, want typed validation error", err, err)
	}
	if validation.Issue.Reason != "invalid time" {
		t.Fatalf("strict issue reason = %q, want content-independent category", validation.Issue.Reason)
	}
	issueJSON, marshalErr := json.Marshal(JournalIntegrity{
		InvalidCount: 1, CountIsLowerBound: true, Issues: []JournalIssue{validation.Issue},
	})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(err.Error(), privateTimestamp) || bytes.Contains(issueJSON, []byte(privateTimestamp)) {
		t.Fatalf("timestamp value leaked through strict error or JSON issue: %s / %s", err, issueJSON)
	}
}

func TestFileDecisionJournalTolerantReadRetainsHealthyRowsAndIssues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	valid := `{"time":"2026-09-26T10:01:00Z","host":"m5","kind":"reservation","requester":"alpha","assessed":"free","determination":"grant","why":"no overlap"}`
	legacy := `{"time":"2026-09-26T10:02:00Z","host":"m5","kind":"assessment","assessed":"legacy","determination":"warn","why":"missing requester"}`
	if err := os.WriteFile(path, []byte(valid+"\n"+legacy+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rows, integrity, err := (&FileDecisionJournal{Path: path}).RecentTolerant(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Requester != "alpha" {
		t.Fatalf("tolerant rows = %+v, want healthy alpha row", rows)
	}
	if integrity.InvalidCount != 1 || len(integrity.Issues) != 1 {
		t.Fatalf("integrity = %+v, want one issue", integrity)
	}
	wantDigest := sha256.Sum256([]byte(legacy))
	if got, want := integrity.Issues[0].Digest, fmt.Sprintf("sha256:%x", wantDigest); got != want {
		t.Fatalf("issue digest = %q, want %q", got, want)
	}
	if got := integrity.Issues[0].Reason; got != "required field is missing" {
		t.Fatalf("issue reason = %q, want content-independent required-field category", got)
	}
	if _, err := (&FileDecisionJournal{Path: path}).Recent(10); err == nil {
		t.Fatal("strict Recent accepted legacy requester-free row")
	}
}

func TestFileDecisionJournalRepairPreservesOriginalAndStrictlyRebuildsActiveProjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	valid := `{"time":"2026-09-26T10:01:00Z","host":"m5","kind":"reservation","requester":"alpha","assessed":"free","determination":"grant","why":"no overlap"}`
	legacy := `{"time":"2026-09-26T10:02:00Z","host":"m5","kind":"assessment","assessed":"legacy","determination":"warn","why":"missing requester"}`
	original := valid + "\n" + legacy + "\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	receipt, err := (&FileDecisionJournal{Path: path}).RepairInvalidRecords()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.RemovedCount != 1 || receipt.RetainedCount != 1 {
		t.Fatalf("receipt = %+v, want one removed and one retained", receipt)
	}
	backup, err := os.ReadFile(receipt.BackupPath)
	if err != nil {
		t.Fatalf("read preserved backup: %v", err)
	}
	if got, want := string(backup), original; got != want {
		t.Fatalf("preserved backup = %q, want original %q", got, want)
	}
	if info, err := os.Stat(receipt.BackupPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode/stat = %v/%v, want 0600", info, err)
	}
	rows, err := (&FileDecisionJournal{Path: path}).Recent(10)
	if err != nil {
		t.Fatalf("strict read after repair: %v", err)
	}
	if len(rows) != 1 || rows[0].Requester != "alpha" {
		t.Fatalf("rows after repair = %+v", rows)
	}
	if _, integrity, err := (&FileDecisionJournal{Path: path}).RecentTolerant(10); err != nil || integrity.InvalidCount != 0 {
		t.Fatalf("tolerant read after repair = integrity %+v err %v", integrity, err)
	}
}

func TestFileDecisionJournalReadsExistingMaatDecisionShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	// This is the existing cross-lane JSONL shape. New Pantheon records must
	// join it rather than requiring a private schema wrapper.
	raw := `{"affected":"m9-smoke","assessed":"held 2026-09-26T10:44:12-04:00 → 10:49","determination":"released","evidence":"20260926T144413Z-m9-smoke-smoke-test","host":"Mac","kind":"reservation release","requester":"smoke-test","resource":"m9-smoke","time":"2026-09-26T14:44:22Z","why":"smoke test"}` + "\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := (&FileDecisionJournal{Path: path}).Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Assessed == "" || got[0].Why != "smoke test" || got[0].Affected != "m9-smoke" {
		t.Fatalf("existing decision shape = %+v", got)
	}
}

func TestRecordReportProjectsEveryAssessmentAndSummary(t *testing.T) {
	j := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	report := NewReport([]Assessment{
		{Domain: DomainCanon, Subject: "abc feature", Standard: "ADR reference", Verdict: VerdictPass, FeatherWeight: 100, Message: "linked"},
		{Domain: DomainCoverage, Subject: "internal/maat", Standard: "80%", Verdict: VerdictFail, FeatherWeight: 0, Message: "coverage low", Remediation: "add tests"},
	})
	report.AssessedAt = report.AssessedAt.UTC()
	if err := RecordReport(j, "sirsi maat audit", report); err != nil {
		t.Fatal(err)
	}
	got, err := j.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("recorded %d decisions, want two assessments plus summary", len(got))
	}
	seen := map[string]bool{}
	for _, decision := range got {
		seen[decision.Kind] = true
		if decision.Requester != "sirsi maat audit" || decision.Evidence == "" {
			t.Fatalf("record = %+v", decision)
		}
	}
	for _, kind := range []string{"assessment canon", "assessment coverage", "assessment report"} {
		if !seen[kind] {
			t.Fatalf("missing %q in %+v", kind, got)
		}
	}
}
