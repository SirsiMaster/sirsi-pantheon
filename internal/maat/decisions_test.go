package maat

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if got := integrity.Issues[0].Reason; !strings.Contains(got, "requester is required") {
		t.Fatalf("issue reason = %q, want requester violation", got)
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
