package maat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const diagnosticRequestTestID = "690c358a-d905-4546-871d-de40a76cc0ac"

func TestDiagnosticRequestUncommittedResultBytesStayPendingUntilDataOnlyCommit(t *testing.T) {
	s, m, _, j, root := diagnosticRecordFixture(t)
	if _, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
		t.Fatal(err)
	}
	id, _ := m.Digest()
	fault := ""
	j.before = func(Decision) {
		raw, err := os.ReadFile(filepath.Join(root, "attempts", id+diagnosticIntentSuffix))
		if err != nil {
			t.Error(err)
			return
		}
		var intent DiagnosticRecordIntent
		if err := json.Unmarshal(raw, &intent); err != nil {
			t.Error(err)
			return
		}
		ih, err := diagnosticSHA(intent)
		if err != nil {
			t.Error(err)
			return
		}
		v, err := m.Observation.Verdict()
		if err != nil {
			t.Error(err)
			return
		}
		vd, err := diagnosticSHA(v)
		if err != nil {
			t.Error(err)
			return
		}
		result := DiagnosticRecordResult{DiagnosticRecordSchema, id, ih, "maat-system-one:sha256=" + vd, vd, "recorded_verified"}
		bytes, err := json.Marshal(result)
		if err != nil {
			t.Error(err)
			return
		}
		write, err := newWriteIntent("attempts", id+diagnosticResultSuffix, bytes)
		if err != nil {
			t.Error(err)
			return
		}
		fault = filepath.Join(root, "writes", write.Key+".committed.json")
		if err := os.Mkdir(fault, 0o700); err != nil {
			t.Error(err)
		}
	}
	r, err := s.BeginDiagnosticRequest(diagnosticRequestTestID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Record(m.Observation, j, func() error { return nil })
	if closeErr := r.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err == nil || fault == "" {
		t.Fatal("missing exact commit fault")
	}
	if err := os.Remove(fault); err != nil {
		t.Fatal(err)
	} // own deliberately empty fault directory only
	if _, err := os.Stat(filepath.Join(root, "attempts", id+diagnosticResultSuffix)); err != nil {
		t.Fatal("fixture never reached result bytes", err)
	}
	read, err := s.ReadDiagnosticRequest(diagnosticRequestTestID)
	if err != nil || read.State != "pending" || read.Result != nil {
		t.Fatal("uncommitted result promoted", read, err)
	}
	history, err := s.DiagnosticRecordHistory()
	if err != nil || len(history) != 1 || history[0].State != "pending" || history[0].Result != nil {
		t.Fatal("history promoted uncommitted result", history, err)
	}
	read, err = s.ResolveDiagnosticRequest(diagnosticRequestTestID, j)
	if err != nil || read.State != "recorded_verified" {
		t.Fatal(read, err)
	}
	if j.count() != 1 {
		t.Fatal("data-only commit replayed append")
	}
}

func TestDiagnosticRequestHistoryKeepsPriorUncorrelatedCompletionWithoutInventedRequest(t *testing.T) {
	s, m, _, j, _ := diagnosticRecordFixture(t)
	m.Observation.Schema = diagnosticObservationSchemaV1
	r, err := s.PreflightDiagnosticRecord(m)
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.RecordDiagnostic(m, h, j, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	requests, err := s.DiagnosticRequests()
	if err != nil || len(requests) != 0 {
		t.Fatal("historical record acquired a request", requests, err)
	}
	history, err := s.DiagnosticRecordHistory()
	if err != nil || len(history) != 1 || history[0].Result == nil || history[0].Result.ID != result.ID || history[0].State != "recorded_verified" || history[0].Intent.Manifest.Observation.Schema != diagnosticObservationSchemaV1 {
		t.Fatal(history, err)
	}
	if j.count() != 1 {
		t.Fatal("history read replayed")
	}
}

func TestDiagnosticRequestResolveWaitsForOwnedInvocationEvenThroughOriginalHandle(t *testing.T) {
	s, m, _, j, _ := diagnosticRecordFixture(t)
	if _, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
		t.Fatal(err)
	}
	session, err := s.BeginDiagnosticRequest(diagnosticRequestTestID)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	j.before = func(Decision) { close(entered); <-release }
	writerDone := make(chan error, 1)
	go func() {
		_, e := session.Record(m.Observation, j, func() error { return nil })
		e = errors.Join(e, session.Close())
		writerDone <- e
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		unblock()
		<-writerDone
		t.Fatal("writer did not enter owned fixture")
	}
	started := make(chan struct{})
	resolverDone := make(chan error, 1)
	go func() {
		close(started)
		read, e := s.ResolveDiagnosticRequest(diagnosticRequestTestID, j)
		if e == nil && read.State != "recorded_verified" {
			e = errors.New("resolver lost exact completion")
		}
		resolverDone <- e
	}()
	<-started
	select {
	case e := <-resolverDone:
		unblock()
		<-writerDone
		t.Fatal("original handle escaped session-owned flock", e)
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	if e := <-writerDone; e != nil {
		t.Fatal(e)
	}
	if e := <-resolverDone; e != nil {
		t.Fatal(e)
	} // join before any fixture/store cleanup
	if j.count() != 1 {
		t.Fatal("concurrent resolution appended")
	}
}

func TestDiagnosticRequestResultWALRecoveryIsExactAndDataOnly(t *testing.T) {
	s, m, _, j, root := diagnosticRecordFixture(t)
	if _, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
		t.Fatal(err)
	}
	id, _ := m.Digest()
	path := filepath.Join(root, "attempts", id+diagnosticResultSuffix)
	j.before = func(Decision) {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Error(err)
		}
	}
	r, err := s.BeginDiagnosticRequest(diagnosticRequestTestID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Record(m.Observation, j, func() error { return nil })
	if closeErr := r.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err == nil {
		t.Fatal("result persistence fault became success")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	} // only this fixture's still-empty fault directory
	read, err := s.ReadDiagnosticRequest(diagnosticRequestTestID)
	if err != nil || read.State != "pending" {
		t.Fatal(read, err)
	}
	read, err = s.ResolveDiagnosticRequest(diagnosticRequestTestID, j)
	if err != nil || read.State != "recorded_verified" {
		t.Fatal(read, err)
	}
	if j.count() != 1 {
		t.Fatal("WAL recovery resent")
	}
}

func TestDiagnosticRequestLookupFailureKeepsFenceAndAllowsDataOnlyReadRetry(t *testing.T) {
	s, m, _, j, _ := diagnosticRecordFixture(t)
	j.badLookup = true
	if _, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
		t.Fatal(err)
	}
	r, err := s.BeginDiagnosticRequest(diagnosticRequestTestID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Record(m.Observation, j, func() error { return nil })
	if closeErr := r.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err == nil {
		t.Fatal("bad exact lookup admitted")
	}
	if _, err = s.ResolveDiagnosticRequest(diagnosticRequestTestID, j); err == nil {
		t.Fatal("bad lookup became completed")
	}
	if _, err = s.BeginDiagnosticRequest(diagnosticRequestTestID); err == nil {
		t.Fatal("uncertain resolution reopened invocation")
	}
	j.badLookup = false
	read, err := s.ResolveDiagnosticRequest(diagnosticRequestTestID, j)
	if err != nil || read.State != "recorded_verified" {
		t.Fatal(read, err)
	}
	if j.count() != 1 {
		t.Fatal("lookup retry resent")
	}
}

func TestDiagnosticRequestWholeReplyLostFindsCompletedOutcomeAfterReopen(t *testing.T) {
	s, m, _, j, _ := diagnosticRecordFixture(t)
	prepared, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID)
	if err != nil || prepared.State != "prepared" {
		t.Fatal(prepared, err)
	}
	r, err := s.BeginDiagnosticRequest(diagnosticRequestTestID)
	if err != nil {
		t.Fatal(err)
	}
	_, recordErr := r.Record(m.Observation, j, func() error { return nil })
	if closeErr := r.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if recordErr != nil {
		t.Fatal(recordErr)
	}
	other, err := OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	read, err := other.ReadDiagnosticRequest(diagnosticRequestTestID)
	if err != nil || read.State != "recorded_verified" || read.Result == nil || read.Intent == nil || read.Result.ID != read.Intent.ID {
		t.Fatal(read, err)
	}
	all, err := other.DiagnosticRequests()
	if err != nil || len(all) != 1 || all[0].Request.ID != diagnosticRequestTestID {
		t.Fatal(all, err)
	}
	if _, err := other.BeginDiagnosticRequest(diagnosticRequestTestID); err == nil {
		t.Fatal("completed invocation replayed")
	}
	if _, err := other.ResolveDiagnosticRequest(diagnosticRequestTestID, j); err != nil {
		t.Fatal(err)
	}
	if j.count() != 1 {
		t.Fatal("lookup/resolve reran append")
	}
}

func TestDiagnosticRequestPrepareOrStartLostExplicitFenceRejectsLateSubmission(t *testing.T) {
	for _, phase := range []string{"before-prepare", "prepared", "started"} {
		t.Run(phase, func(t *testing.T) {
			s, _, _, j, _ := diagnosticRecordFixture(t)
			if phase != "before-prepare" {
				if _, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "started" {
				r, err := s.BeginDiagnosticRequest(diagnosticRequestTestID)
				if err != nil {
					t.Fatal(err)
				}
				if closeErr := r.Close(); closeErr != nil {
					t.Fatal(closeErr)
				}
			}
			if phase == "before-prepare" {
				if _, err := s.ReadDiagnosticRequest(diagnosticRequestTestID); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing lookup invented safe result", err)
				}
			}
			read, err := s.ResolveDiagnosticRequest(diagnosticRequestTestID, j)
			if err != nil || read.State != "fenced_not_recorded" {
				t.Fatal(read, err)
			}
			// Late metadata is idempotent, but cannot reopen the fenced operation.
			late, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID)
			if err != nil || late.State != "fenced_not_recorded" {
				t.Fatal(late, err)
			}
			if _, err := s.BeginDiagnosticRequest(diagnosticRequestTestID); err == nil {
				t.Fatal("late submission escaped durable fence")
			}
			if j.count() != 0 {
				t.Fatal("fence observed or appended")
			}
		})
	}
}

func TestDiagnosticRequestPendingLostAckReconcilesWithoutObservationOrAppend(t *testing.T) {
	s, m, _, j, _ := diagnosticRecordFixture(t)
	j.lostAck = true
	if _, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
		t.Fatal(err)
	}
	r, err := s.BeginDiagnosticRequest(diagnosticRequestTestID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Record(m.Observation, j, func() error { return nil })
	if closeErr := r.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if !errors.Is(err, ErrDiagnosticDeliveryUnknown) {
		t.Fatal(err)
	}
	read, err := s.ReadDiagnosticRequest(diagnosticRequestTestID)
	if err != nil || read.State != "pending" {
		t.Fatal(read, err)
	}
	read, err = s.ResolveDiagnosticRequest(diagnosticRequestTestID, j)
	if err != nil || read.State != "recorded_verified" {
		t.Fatal(read, err)
	}
	if j.count() != 1 {
		t.Fatal("pending request resent")
	}
}

func TestDiagnosticRequestFencedAbsentRowIsNotRecordedNotNeverCalled(t *testing.T) {
	s, m, h, j, _ := diagnosticRecordFixture(t)
	j.refuse = true
	if _, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
		t.Fatal(err)
	}
	r, err := s.BeginDiagnosticRequest(diagnosticRequestTestID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Record(m.Observation, j, func() error { return nil })
	if closeErr := r.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err == nil {
		t.Fatal("refused append succeeded")
	}
	read, err := s.ResolveDiagnosticRequest(diagnosticRequestTestID, j)
	if err != nil || read.State != "not_recorded" || read.Result == nil || read.Result.DecisionEvidence != "" {
		t.Fatal(read, err)
	}
	if _, err = s.RecordDiagnostic(m, h, j, func() error { return nil }); !errors.Is(err, ErrDiagnosticNotRecorded) {
		t.Fatal("legacy same-observation replay not fenced", err)
	}
	if j.count() != 1 {
		t.Fatal("resolution or legacy retry appended")
	}
}

func TestDiagnosticRequestForeignMalformedOrCorruptBindingRemainsUnknown(t *testing.T) {
	for _, mode := range []string{"foreign-store", "corrupt-request", "malformed-key"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _, j, root := diagnosticRecordFixture(t)
			if _, err := s.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
				t.Fatal(err)
			}
			id := diagnosticRequestTestID
			switch mode {
			case "foreign-store":
				s.storeUUID = "f1f3d9e2-58cb-42cf-88b2-93142eea4d8f"
			case "corrupt-request":
				if err := os.WriteFile(filepath.Join(root, "attempts", id+diagnosticRequestSuffix), []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "malformed-key":
				id = strings.Repeat("x", 36)
			}
			if _, err := s.ReadDiagnosticRequest(id); err == nil {
				t.Fatal("unknown request accepted")
			}
			if _, err := s.ResolveDiagnosticRequest(id, j); err == nil {
				t.Fatal("invalid request manufactured fence")
			}
			if j.count() != 0 {
				t.Fatal("bad request reached journal")
			}
		})
	}
}
