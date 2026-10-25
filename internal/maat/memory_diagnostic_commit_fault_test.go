package maat

import (
	"errors"
	"testing"
)

var errDiagnosticResultCommitInterrupted = errors.New("owned fixture: diagnostic result target complete before commit")

type diagnosticResultCommitFault struct {
	hits  int
	key   string
	bytes []byte
}

// The actual governed writer performs every guard/prepared/target/fsync/readback
// step. This session-owned hook then returns one exact interruption; it creates
// no fake WAL/target bytes or malformed early directory and has no global hook.
func interruptDiagnosticResultCommit(t *testing.T, session *DiagnosticRequestSession, id, state string) *diagnosticResultCommitFault {
	t.Helper()
	fault := &diagnosticResultCommitFault{}
	store := session.store
	store.beforeWriteCommit = func(intent writeIntent) error {
		if intent.Namespace != "attempts" || intent.Leaf != id+diagnosticResultSuffix {
			return nil
		}
		data, err := readExact(store.attemptsFD, intent.Leaf)
		if err != nil {
			return err
		}
		var result DiagnosticRecordResult
		if decodeDiagnosticCanonical(data, &result) != nil || result.ID != id || result.State != state {
			return errors.New("owned fixture: actual result target has unexpected identity/state")
		}
		actual, err := newWriteIntent(intent.Namespace, intent.Leaf, data)
		if err != nil {
			return err
		}
		if actual != intent {
			return errors.New("owned fixture: actual complete target differs from governed write intent")
		}
		prepared, err := readExact(store.writesFD, intent.Key+".prepared.json")
		if err != nil {
			return err
		}
		var retained writeIntent
		if decodeDiagnosticCanonical(prepared, &retained) != nil || retained != intent {
			return errors.New("owned fixture: target lacks exact durable prepared metadata")
		}
		fault.hits++
		fault.key = intent.Key
		fault.bytes = append([]byte(nil), data...)
		return errDiagnosticResultCommitInterrupted
	}
	return fault
}
