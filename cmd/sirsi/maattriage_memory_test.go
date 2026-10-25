package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

type triageMemoryJournal struct {
	*maat.FileDecisionJournal
	calls int
	lost  bool
}

func (j *triageMemoryJournal) AppendOnce(d maat.Decision) (bool, error) {
	j.calls++
	ok, err := j.FileDecisionJournal.AppendOnce(d)
	if err == nil && j.lost {
		return false, errors.New("owned lost ack")
	}
	return ok, err
}

func triageMemoryFixture(t *testing.T) (string, *guard.DoctorReport, *triageMemoryJournal, *int) {
	t.Helper()
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	oldRead, oldHost, oldJournal, oldJSON := maatTriageReadDiagnosis, maatTriageHostname, newMaatDecisionJournal, JsonOutput
	t.Cleanup(func() {
		maatTriageReadDiagnosis, maatTriageHostname, newMaatDecisionJournal, JsonOutput = oldRead, oldHost, oldJournal, oldJSON
	})
	for _, name := range []string{"confirm", "json", "inspect-records", "reconcile-record"} {
		f := maatTriageCmd.Flags().Lookup(name)
		value, changed := f.Value.String(), f.Changed
		t.Cleanup(func() {
			if err := f.Value.Set(value); err != nil {
				t.Error(err)
			}
			f.Changed = changed
		})
	}
	if err := maatTriageCmd.ParseFlags([]string{"--confirm=false", "--json", "--inspect-records=false", "--reconcile-record="}); err != nil {
		t.Fatal(err)
	}
	JsonOutput = false
	report := &guard.DoctorReport{Timestamp: time.Now().UTC(), Score: 88, Duration: "/Users/PRIVATE_DURATION_CANARY", Findings: []guard.DiagnosticFinding{
		{Check: "RAM Pressure", Severity: guard.SeverityWarn, Message: "SECRET_MESSAGE_CANARY --token private", Detail: "PROMPT_DETAIL_CANARY /Users/private", Fix: "sh -c PRIVATE_FIX_CANARY"},
		{Check: "/Users/PRIVATE_CHECK_CANARY --argv", Severity: guard.SeverityInfo, Message: "private"},
	}}
	reads := new(int)
	maatTriageReadDiagnosis = func() (*guard.DoctorReport, error) { *reads++; return report, nil }
	maatTriageHostname = func() (string, error) { return "/Users/PRIVATE_HOST_CANARY", nil }
	journal := &triageMemoryJournal{FileDecisionJournal: &maat.FileDecisionJournal{Path: filepath.Join(home, "owned-journal.jsonl")}}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	return home, report, journal, reads
}

func runTriageFixture(t *testing.T, args ...string) (string, error) {
	t.Helper()
	if err := maatTriageCmd.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	var err error
	out := captureStdout(t, func() { err = maatTriageCmd.RunE(maatTriageCmd, maatTriageCmd.Flags().Args()) })
	return out, err
}

func assertDiagnosticPrivacy(t *testing.T, text string) {
	t.Helper()
	for _, canary := range []string{"SECRET_MESSAGE_CANARY", "PROMPT_DETAIL_CANARY", "PRIVATE_FIX_CANARY", "PRIVATE_CHECK_CANARY", "PRIVATE_DURATION_CANARY", "PRIVATE_HOST_CANARY"} {
		if strings.Contains(text, canary) {
			t.Fatalf("diagnostic privacy canary retained/output: %s", canary)
		}
	}
}

func TestMaatTriageObservationAvailableButMissingMemoryCannotRecord(t *testing.T) {
	home, _, journal, _ := triageMemoryFixture(t)
	out, err := runTriageFixture(t, "--confirm=false")
	if err != nil {
		t.Fatal(err)
	}
	var preview struct {
		SchemaVersion int    `json:"schema_version"`
		Memory        string `json:"memory_state"`
		Provenance    string `json:"diagnostic_provenance"`
		Admission     string `json:"memory_preflight"`
		Recorded      bool   `json:"recorded"`
	}
	if err := json.Unmarshal([]byte(out), &preview); err != nil || preview.SchemaVersion != 1 || preview.Memory != "uninitialized" || preview.Provenance != "fresh" || preview.Admission != "not_performed" || preview.Recorded {
		t.Fatalf("preview confused observation and memory: %s %v", out, err)
	}
	assertDiagnosticPrivacy(t, out)
	out, err = runTriageFixture(t, "--confirm")
	if err == nil || journal.calls != 0 {
		t.Fatal("missing memory appended")
	}
	var refusal map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &refusal); err != nil {
		t.Fatal(err)
	}
	if _, ok := refusal["schema_version"]; ok {
		t.Fatal("native recorder could decode refusal as recorded verdict")
	}
	if len(refusal["observation"]) == 0 {
		t.Fatal("safe observation disappeared")
	}
	if _, err := os.Stat(filepath.Join(home, ".sirsi")); !os.IsNotExist(err) {
		t.Fatal("triage initialized missing memory", err)
	}
	assertDiagnosticPrivacy(t, out)
}

func TestMaatTriageActualConfirmedCommandRetainsSanitizedRecordOnce(t *testing.T) {
	home, _, journal, _ := triageMemoryFixture(t)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	out, err := runTriageFixture(t, "--confirm")
	if err != nil {
		t.Fatal(err)
	}
	assertDiagnosticPrivacy(t, out)
	var result struct {
		State    string `json:"record_state"`
		ID       string `json:"record_id"`
		Evidence string `json:"decision_evidence"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.State != "recorded_verified" || len(result.ID) != 64 || result.Evidence == "" {
		t.Fatalf("missing exact record result %s %v", out, err)
	}
	if _, err := runTriageFixture(t, "--confirm"); err != nil || journal.calls != 1 {
		t.Fatal("exact observation appended twice", err)
	}
	rows, err := journal.Recent(100)
	if err != nil || len(rows) != 1 || !strings.HasPrefix(rows[0].OriginEvidence, "maat-diagnostic-record:"+result.ID+":preflight-sha256=") {
		t.Fatal("row not linked to durable memory receipt", err)
	}
	err = filepath.WalkDir(home, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assertDiagnosticPrivacy(t, string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMaatTriageChangedSelectedProvenanceRefusesBeforeJournalEffect(t *testing.T) {
	_, report, journal, _ := triageMemoryFixture(t)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) {
		report.Findings[0].Message += " changed after preview"
		return journal, nil
	}
	out, err := runTriageFixture(t, "--confirm")
	if err == nil || journal.calls != 0 {
		t.Fatal("changed report was recorded")
	}
	assertDiagnosticPrivacy(t, out)
}

func TestMaatTriageLostAckInspectableAndReconciledWithoutObservationOrResend(t *testing.T) {
	_, _, journal, reads := triageMemoryFixture(t)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	journal.lost = true
	out, err := runTriageFixture(t, "--confirm")
	if err == nil || journal.calls != 1 {
		t.Fatal("lost ack did not retain uncertainty")
	}
	var result struct {
		ID string `json:"record_id"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.ID) != 64 {
		t.Fatal("unknown record lost identity", err)
	}
	if _, err := runTriageFixture(t, "--confirm"); err == nil || journal.calls != 1 {
		t.Fatal("unknown record auto-resent")
	}
	before := *reads
	out, err = runTriageFixture(t, "--confirm=false", "--inspect-records")
	if err != nil || *reads != before {
		t.Fatal("inspect re-observed host", err)
	}
	if !strings.Contains(out, result.ID) {
		t.Fatal("pending record not discoverable")
	}
	if _, err := runTriageFixture(t, "--inspect-records=false", "--reconcile-record", result.ID, "--confirm=false"); err == nil {
		t.Fatal("unconfirmed reconciliation accepted")
	}
	out, err = runTriageFixture(t, "--confirm")
	if err != nil || *reads != before || journal.calls != 1 {
		t.Fatal("data-only reconcile resubmitted", err)
	}
	var verified maat.DiagnosticRecordResult
	if err := json.Unmarshal([]byte(out), &verified); err != nil || verified.ID != result.ID {
		t.Fatal("wrong readback result", err)
	}
	assertDiagnosticPrivacy(t, out)
}

func TestMaatTriageUnknownProvenanceAndCorruptMemoryNeverAppend(t *testing.T) {
	for _, mode := range []string{"past", "future", "missing-time", "corrupt-memory", "missing-statuses", "oversize-source"} {
		t.Run(mode, func(t *testing.T) {
			home, report, journal, _ := triageMemoryFixture(t)
			if _, err := maat.InitializeDefaultStore(); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "past":
				report.Timestamp = time.Now().Add(-3 * time.Minute)
			case "future":
				report.Timestamp = time.Now().Add(time.Minute)
			case "missing-time":
				report.Timestamp = time.Time{}
			case "missing-statuses":
				report.Findings = nil
				report.Score = 100
			case "oversize-source":
				report.Duration = strings.Repeat("x", (1<<20)+1)
			case "corrupt-memory":
				if err := os.WriteFile(filepath.Join(home, ".sirsi", "maat", "failure-memory", "writes", "unrecognized.json"), []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			out, err := runTriageFixture(t, "--confirm")
			if err == nil || journal.calls != 0 {
				t.Fatal("unknown provenance/memory appended")
			}
			assertDiagnosticPrivacy(t, out)
		})
	}
}

func TestMaatTriageScopedIncidentRefusesRecordButNotLocalObservation(t *testing.T) {
	for _, version := range []string{maat.ActiveIncidentGuardVersion, "unsupported-v9"} {
		t.Run(version, func(t *testing.T) {
			_, _, journal, _ := triageMemoryFixture(t)
			if _, err := maat.InitializeDefaultStore(); err != nil {
				t.Fatal(err)
			}
			s, err := maat.OpenDefaultStore()
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			proof, err := s.PutEvidence([]byte("owned diagnostic regression evidence"))
			if err != nil {
				t.Fatal(err)
			}
			signature := maat.FailureSignature{Namespace: "pantheon", Version: "v1", FailureClass: "diagnostic-record", Operation: "record-system-one-diagnostic", Component: "pantheon", Invariant: "owned record failed", ErrorCode: "OWNED_FIXTURE"}
			incident, err := maat.NewIncident(signature, proof, maat.Scope{Component: "pantheon", Profile: "local-macos", Operation: "record-system-one-diagnostic"}, maat.ActiveIncidentGuardID, version, []maat.RecoveryAction{{ID: "review", Label: "Review exact local evidence", Instruction: "Retain an owner review before a fresh observation; this text is not executable."}})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Append(incident); err != nil {
				t.Fatal(err)
			}
			if _, err := runTriageFixture(t, "--confirm=false"); err != nil || journal.calls != 0 {
				t.Fatal("scoped prevention blocked pure observation", err)
			}
			if _, err := runTriageFixture(t, "--confirm"); err == nil || journal.calls != 0 {
				t.Fatal("matching recurrence/unsupported guard appended")
			}
		})
	}
}
