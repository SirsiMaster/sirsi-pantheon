package maat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type diagnosticLookupCountingJournal struct {
	*diagnosticTestJournal
	lookups int
}

func (j *diagnosticLookupCountingJournal) FindByEvidence(kind, evidence string) (Decision, bool, error) {
	j.lookups++
	return j.diagnosticTestJournal.FindByEvidence(kind, evidence)
}

func TestDiagnosticRequestCompleteNotSubmittedResultCommitRecoversBeforeJournalInference(t *testing.T) {
	s, m, _, base, root := diagnosticRecordFixture(t)
	j := &diagnosticLookupCountingJournal{diagnosticTestJournal: base}
	if _, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
		t.Fatal(err)
	}
	session, err := s.BeginDiagnosticRequest(diagnosticRequestTestID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	id, _ := m.Digest()
	checks := 0
	fault := ""
	commitKey := ""
	verify := func() error {
		checks++
		if checks == 1 {
			return nil
		}
		raw, err := os.ReadFile(filepath.Join(root, "attempts", id+diagnosticIntentSuffix))
		if err != nil {
			return err
		}
		var intent DiagnosticRecordIntent
		if err = json.Unmarshal(raw, &intent); err != nil {
			return err
		}
		ih, err := diagnosticSHA(intent)
		if err != nil {
			return err
		}
		result := DiagnosticRecordResult{Schema: DiagnosticRecordSchema, ID: id, IntentSHA256: ih, State: "not_submitted"}
		bytes, err := json.Marshal(result)
		if err != nil {
			return err
		}
		write, err := newWriteIntent("attempts", id+diagnosticResultSuffix, bytes)
		if err != nil {
			return err
		}
		commitKey = write.Key
		fault = filepath.Join(root, "writes", write.Key+".committed.json")
		if err = os.Mkdir(fault, 0o700); err != nil {
			return err
		}
		return errors.New("owned second provenance check refuses before append")
	}
	_, recordErr := session.Record(m.Observation, j, verify)
	if closeErr := session.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if !errors.Is(recordErr, ErrDiagnosticNotSubmitted) || checks != 2 || fault == "" || base.count() != 0 {
		t.Fatal("fault did not hit exact pre-append result commit", recordErr, checks, base.count())
	}
	retained, err := os.ReadFile(filepath.Join(root, "attempts", id+diagnosticResultSuffix))
	if err != nil {
		t.Fatal("complete terminal bytes not retained", err)
	}
	var terminal DiagnosticRecordResult
	if err = json.Unmarshal(retained, &terminal); err != nil || terminal.State != "not_submitted" {
		t.Fatal("wrong retained terminal target", terminal, err)
	}
	if err = os.Remove(fault); err != nil {
		t.Fatal(err)
	} // exact fixture-owned empty directory only
	reopened, err := OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	read, err := reopened.ReadDiagnosticRequest(diagnosticRequestTestID)
	if err != nil || read.State != "pending" || read.Result != nil {
		t.Fatal("uncommitted terminal bytes promoted", read, err)
	}
	read, err = reopened.ResolveDiagnosticRequest(diagnosticRequestTestID, j)
	if err != nil || read.State != "not_submitted" || read.Result == nil || *read.Result != terminal {
		t.Fatal("exact retained result did not recover", read, err)
	}
	if base.count() != 0 || j.lookups != 0 || checks != 2 {
		t.Fatal("data-only recovery inferred journal absence or re-observed", base.count(), j.lookups, checks)
	}
	if _, err := os.Stat(filepath.Join(root, "writes", commitKey+".committed.json")); err != nil {
		t.Fatal("exact WAL not committed", err)
	}
	if _, err := reopened.BeginDiagnosticRequest(diagnosticRequestTestID); err == nil {
		t.Fatal("old request reopened")
	}
	if after, err := os.ReadFile(filepath.Join(root, "attempts", id+diagnosticResultSuffix)); err != nil || string(after) != string(retained) {
		t.Fatal("terminal bytes replaced", err)
	}
}
