package maat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type diagnosticTestJournal struct {
	file      *FileDecisionJournal
	mu        sync.Mutex
	calls     int
	lostAck   bool
	refuse    bool
	badLookup bool
	before    func(Decision)
}

func (j *diagnosticTestJournal) Append(d Decision) error {
	return errors.New("test requires existing atomic append-once path")
}
func (j *diagnosticTestJournal) Recent(n int) ([]Decision, error) { return j.file.Recent(n) }
func (j *diagnosticTestJournal) AppendOnce(d Decision) (bool, error) {
	j.mu.Lock()
	j.calls++
	lost, refuse, before := j.lostAck, j.refuse, j.before
	j.mu.Unlock()
	if before != nil {
		before(d)
	}
	if refuse {
		return false, errors.New("owned transport refused without delivery")
	}
	appended, err := j.file.AppendOnce(d)
	if err == nil && lost {
		return false, errors.New("owned append acknowledgement lost")
	}
	return appended, err
}
func (j *diagnosticTestJournal) FindByEvidence(kind, evidence string) (Decision, bool, error) {
	d, ok, err := j.file.FindByEvidence(kind, evidence)
	j.mu.Lock()
	bad := j.badLookup
	j.mu.Unlock()
	if bad && d.SystemOne != nil {
		v := *d.SystemOne
		v.Subject.HeadSHA = strings.Repeat("f", 64)
		d.SystemOne = &v
	}
	return d, ok, err
}
func (j *diagnosticTestJournal) count() int { j.mu.Lock(); defer j.mu.Unlock(); return j.calls }

func diagnosticRecordFixture(t *testing.T) (*Store, DiagnosticRecordManifest, StoredPreflightReceipt, *diagnosticTestJournal, string) {
	t.Helper()
	home := privateMemoryStoreRoot(t)
	t.Setenv("HOME", home)
	uuid, err := InitializeDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	m := DiagnosticRecordManifest{Schema: DiagnosticRecordSchema, UID: uint32(os.Geteuid()), StoreUUID: uuid,
		Observation: DiagnosticObservation{Schema: DiagnosticObservationSchema, SourceSHA256: strings.Repeat("a", 64), HostSHA256: strings.Repeat("b", 64), ObservedAtUTC: time.Now().UTC(), Score: 93, Statuses: []DiagnosticStatus{{Check: "RAM Pressure", Severity: 2}}}}
	r, err := s.PreflightDiagnosticRecord(m)
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	j := &diagnosticTestJournal{file: &FileDecisionJournal{Path: filepath.Join(home, "journal.jsonl")}}
	return s, m, h, j, filepath.Join(home, ".sirsi", "maat", "failure-memory")
}

func TestDiagnosticRecordRetainsIntentBeforeOneAppendAndExactReadback(t *testing.T) {
	s, m, h, j, root := diagnosticRecordFixture(t)
	id, _ := m.Digest()
	j.before = func(d Decision) {
		if _, err := os.Stat(filepath.Join(root, "attempts", id+diagnosticIntentSuffix)); err != nil {
			t.Error("append preceded durable intent", err)
		}
		if d.OriginEvidence != diagnosticRecordOrigin(id, h.Reference().ReceiptSHA256) {
			t.Error("journal lost exact memory linkage")
		}
	}
	result, err := s.RecordDiagnostic(m, h, j, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != id || j.count() != 1 {
		t.Fatal("record not exactly once")
	}
	again, err := s.RecordDiagnostic(m, h, j, func() error { return nil })
	if err != nil || again != result || j.count() != 1 {
		t.Fatal("same observation duplicated", err)
	}
	rows, err := j.Recent(100)
	if err != nil || len(rows) != 1 {
		t.Fatal("journal postcondition", len(rows), err)
	}
	if pending, err := s.PendingDiagnosticRecords(); err != nil || len(pending) != 0 {
		t.Fatal("completed record still debt", err)
	}
	if _, err := s.PendingLivenessWatchAttempt(); !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
		t.Fatal("closed diagnostic namespace broke liveness", err)
	}
	if debts, err := s.PendingLivenessWatchProjectionDebts(); err != nil || len(debts) != 0 {
		t.Fatal("diagnostic data became liveness debt", err)
	}
}

func TestDiagnosticRecordAmbiguousAppendReopensAndReconcilesWithoutResend(t *testing.T) {
	s, m, h, j, _ := diagnosticRecordFixture(t)
	j.lostAck = true
	if _, err := s.RecordDiagnostic(m, h, j, func() error { return nil }); !errors.Is(err, ErrDiagnosticDeliveryUnknown) {
		t.Fatal("lost acknowledgement became success", err)
	}
	pending, err := s.PendingDiagnosticRecords()
	if err != nil || len(pending) != 1 {
		t.Fatal("unknown delivery not discoverable", err)
	}
	if _, err := s.RecordDiagnostic(m, h, j, func() error { return nil }); !errors.Is(err, ErrDiagnosticDeliveryUnknown) || j.count() != 1 {
		t.Fatal("ambiguous record auto-resent", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	result, err := reopened.ReconcileDiagnosticRecord(pending[0].ID, j)
	if err != nil || result.ID != pending[0].ID || j.count() != 1 {
		t.Fatal("read-only recovery failed", err)
	}
	if _, err := reopened.ReconcileDiagnosticRecord(pending[0].ID, j); err != nil || j.count() != 1 {
		t.Fatal("reconcile appended", err)
	}
}

func TestDiagnosticRecordChangedProvenanceAndRegistryRefuseBeforeAppend(t *testing.T) {
	for _, mode := range []string{"provenance", "registry", "expired", "manifest"} {
		t.Run(mode, func(t *testing.T) {
			s, m, h, j, _ := diagnosticRecordFixture(t)
			verify := func() error { return nil }
			switch mode {
			case "provenance":
				verify = func() error { return errors.New("changed owned report") }
			case "registry":
				proof, err := s.PutEvidence([]byte("owned new diagnostic incident"))
				if err != nil {
					t.Fatal(err)
				}
				incident, err := NewIncident(testSignature(), proof, diagnosticScope(), ActiveIncidentGuardID, ActiveIncidentGuardVersion, testActions())
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Append(incident); err != nil {
					t.Fatal(err)
				}
			case "expired":
				m.Observation.ObservedAtUTC = m.Observation.ObservedAtUTC.Add(-3 * time.Minute)
			case "manifest":
				m.Observation.SourceSHA256 = strings.Repeat("c", 64)
			}
			if _, err := s.RecordDiagnostic(m, h, j, verify); err == nil || j.count() != 0 {
				t.Fatal("unverified observation appended", err)
			}
		})
	}
}

func TestDiagnosticRecordWrongReadbackRemainsUnknownAndNeverResends(t *testing.T) {
	s, m, h, j, _ := diagnosticRecordFixture(t)
	j.badLookup = true
	if _, err := s.RecordDiagnostic(m, h, j, func() error { return nil }); !errors.Is(err, ErrDiagnosticDeliveryUnknown) {
		t.Fatal(err)
	}
	id, _ := m.Digest()
	if _, err := s.ReconcileDiagnosticRecord(id, j); !errors.Is(err, ErrDiagnosticDeliveryUnknown) || j.count() != 1 {
		t.Fatal("wrong provenance accepted or resent", err)
	}
	j.badLookup = false
	if _, err := s.ReconcileDiagnosticRecord(id, j); err != nil || j.count() != 1 {
		t.Fatal("restored exact readback failed", err)
	}
}

func TestDiagnosticRecordNoJournalRowCannotBeReconstructedByReconciliation(t *testing.T) {
	s, m, h, j, root := diagnosticRecordFixture(t)
	j.refuse = true
	if _, err := s.RecordDiagnostic(m, h, j, func() error { return nil }); !errors.Is(err, ErrDiagnosticDeliveryUnknown) {
		t.Fatal("ambiguous transport failure accepted", err)
	}
	id, _ := m.Digest()
	before := receiptRecoveryFiles(t, root)
	if _, err := s.ReconcileDiagnosticRecord(id, j); !errors.Is(err, ErrDiagnosticDeliveryUnknown) || j.count() != 1 {
		t.Fatal("missing journal row recreated", err)
	}
	if string(before) != string(receiptRecoveryFiles(t, root)) {
		t.Fatal("unknown readback wrote data")
	}
}

func TestDiagnosticRecordKnownPreAppendRefusalClosesWithoutReplay(t *testing.T) {
	s, m, h, j, _ := diagnosticRecordFixture(t)
	checks := 0
	result, err := s.RecordDiagnostic(m, h, j, func() error {
		checks++
		if checks == 2 {
			return errors.New("provenance changed before append")
		}
		return nil
	})
	if !errors.Is(err, ErrDiagnosticNotSubmitted) || result.State != "not_submitted" || j.count() != 0 {
		t.Fatal("known zero-call refusal was not retained", err)
	}
	if pending, err := s.PendingDiagnosticRecords(); err != nil || len(pending) != 0 {
		t.Fatal("known not-submitted state stranded new review", err)
	}
	if read, err := s.ReconcileDiagnosticRecord(result.ID, j); err != nil || read.State != "not_submitted" || j.count() != 0 {
		t.Fatal("data-only no-submit readback failed", err)
	}
	if _, err := s.RecordDiagnostic(m, h, j, func() error { return nil }); !errors.Is(err, ErrDiagnosticNotSubmitted) || j.count() != 0 {
		t.Fatal("old declined observation replayed", err)
	}
	m.Observation.SourceSHA256 = strings.Repeat("d", 64)
	m.Observation.ObservedAtUTC = time.Now().UTC()
	r, err := s.PreflightDiagnosticRecord(m)
	if err != nil {
		t.Fatal(err)
	}
	h, err = s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := s.RecordDiagnostic(m, h, j, func() error { return nil }); err != nil || result.State != "recorded_verified" || j.count() != 1 {
		t.Fatal("fresh explicit observation could not proceed", err)
	}
}

func TestDiagnosticRecordResultWriteDebtRecoversDataOnly(t *testing.T) {
	s, m, h, j, root := diagnosticRecordFixture(t)
	id, _ := m.Digest()
	path := filepath.Join(root, "attempts", id+diagnosticResultSuffix)
	j.before = func(Decision) {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Error(err)
		}
	}
	if _, err := s.RecordDiagnostic(m, h, j, func() error { return nil }); err == nil || j.count() != 1 {
		t.Fatal("failed readback persistence became success", err)
	}
	// Remove only our deliberately created, still-empty fault directory. No
	// production cleanup/adoption path or permission waiver is added.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if pending, err := s.PendingDiagnosticRecords(); err != nil || len(pending) != 1 || pending[0].ID != id {
		t.Fatal("result WAL debt lost its recovery entry identity", err)
	}
	if result, err := s.ReconcileDiagnosticRecord(id, j); err != nil || result.State != "recorded_verified" || j.count() != 1 {
		t.Fatal("exact WAL result retry resubmitted journal", err)
	}
}

func TestDiagnosticRecordConcurrentSameObservationAppendsOnce(t *testing.T) {
	s, m, h, j, _ := diagnosticRecordFixture(t)
	second, err := OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, store := range []*Store{s, second} {
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			_, err := store.RecordDiagnostic(m, h, j, func() error { return nil })
			results <- err
		}(store)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if j.count() != 1 {
		t.Fatal("concurrent confirmation duplicated append")
	}
}

func TestDiagnosticObservationClosedProjectionRejectsUnknownRawFields(t *testing.T) {
	s, m, _, _, _ := diagnosticRecordFixture(t)
	for _, mode := range []string{"raw-check", "raw-repair", "duplicate", "missing-time", "future", "past"} {
		t.Run(mode, func(t *testing.T) {
			copy := m
			copy.Observation.Statuses = append([]DiagnosticStatus(nil), m.Observation.Statuses...)
			switch mode {
			case "raw-check":
				copy.Observation.Statuses[0].Check = "/Users/private/canary --prompt secret"
			case "raw-repair":
				copy.Observation.Statuses[0].RepairID = "sh -c private-command"
			case "duplicate":
				copy.Observation.Statuses = append(copy.Observation.Statuses, copy.Observation.Statuses[0])
			case "missing-time":
				copy.Observation.ObservedAtUTC = time.Time{}
			case "future":
				copy.Observation.ObservedAtUTC = time.Now().Add(time.Minute)
			case "past":
				copy.Observation.ObservedAtUTC = time.Now().Add(-3 * time.Minute)
			}
			if _, err := s.PreflightDiagnosticRecord(copy); err == nil {
				t.Fatal("unknown provenance/control admitted")
			}
		})
	}
}
