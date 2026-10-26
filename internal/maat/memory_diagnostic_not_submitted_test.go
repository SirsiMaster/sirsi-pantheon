package maat

import (
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
	fault := interruptDiagnosticResultCommit(t, session, id, "not_submitted")
	verify := func() error {
		checks++
		if checks == 1 {
			return nil
		}
		return errors.New("owned second provenance check refuses before append")
	}
	_, recordErr := session.Record(m.Observation, j, verify)
	if closeErr := session.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if !errors.Is(recordErr, ErrDiagnosticNotSubmitted) || !errors.Is(recordErr, errDiagnosticResultCommitInterrupted) || checks != 2 || fault.hits != 1 || base.count() != 0 {
		t.Fatal("fault did not hit exact pre-append terminal commit", recordErr, checks, fault.hits, base.count())
	}
	path := filepath.Join(root, "attempts", id+diagnosticResultSuffix)
	retained, err := os.ReadFile(path)
	if err != nil || string(retained) != string(fault.bytes) {
		t.Fatal("actual complete terminal bytes not retained", recordErr, err)
	}
	var terminal DiagnosticRecordResult
	if decodeDiagnosticCanonical(retained, &terminal) != nil || terminal.State != "not_submitted" {
		t.Fatal("wrong retained terminal target")
	}
	commitPath := filepath.Join(root, "writes", fault.key+".committed.json")
	if _, err := os.Stat(commitPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interruption unexpectedly committed", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
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
	commitBytes, err := os.ReadFile(commitPath)
	if err != nil {
		t.Fatal("exact terminal WAL did not commit", err)
	}
	var commit writeCommit
	if decodeDiagnosticCanonical(commitBytes, &commit) != nil || commit.Schema != MemorySchema || commit.IntentKey != fault.key {
		t.Fatal("wrong committed terminal WAL tuple")
	}
	if _, err := reopened.BeginDiagnosticRequest(diagnosticRequestTestID); err == nil {
		t.Fatal("old request reopened")
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(retained) {
		t.Fatal("terminal bytes replaced", err)
	}
}
