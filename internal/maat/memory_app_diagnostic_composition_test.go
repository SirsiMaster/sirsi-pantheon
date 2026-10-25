package maat

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func compositionAttemptBytes(t *testing.T, root string) map[string][]byte {
	t.Helper()
	result := map[string][]byte{}
	for _, namespace := range []string{"attempts", "writes"} {
		entries, err := os.ReadDir(filepath.Join(root, namespace))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := filepath.Join(namespace, entry.Name())
			data, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			result[name] = data
		}
	}
	return result
}

func assertCompositionBytes(t *testing.T, root string, before map[string][]byte, exact bool) {
	t.Helper()
	after := compositionAttemptBytes(t, root)
	if exact && len(after) != len(before) {
		t.Fatal("read-only refusal created or removed retained facts")
	}
	for name, data := range before {
		if !bytes.Equal(after[name], data) {
			t.Fatalf("retained O7 history changed: %s", name)
		}
	}
}

func TestAppUpdateComposesCommittedCorrelatedDiagnosticHistory(t *testing.T) {
	store, manifest, _, journal, root := diagnosticRecordFixture(t)
	if _, err := store.PrepareDiagnosticRequest(diagnosticRequestTestID); err != nil {
		t.Fatal(err)
	}
	session, err := store.BeginDiagnosticRequest(diagnosticRequestTestID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	result, recordErr := session.Record(manifest.Observation, journal, func() error { return nil })
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if recordErr != nil || result.State != "recorded_verified" || journal.count() != 1 {
		t.Fatalf("fixture did not retain one real correlated record: %+v %v", result, recordErr)
	}
	before := compositionAttemptBytes(t, root)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	read, err := reopened.ReadDiagnosticRequest(diagnosticRequestTestID)
	if err != nil || read.State != "recorded_verified" || read.Result == nil || read.Result.ID != result.ID {
		t.Fatalf("reopened correlated history lost exact completion: %+v %v", read, err)
	}
	app := fixtureAppUpdateManifest(t, reopened.storeUUID)
	stored := retainAppUpdateReceipt(t, reopened, app)
	attempt, err := reopened.BeginAppUpdateAttempt(app, stored)
	if err != nil || attempt.PreflightReceipt.ReceiptSHA256 == "" {
		t.Fatalf("valid settled diagnostic history blocked the independent app adapter: %+v %v", attempt, err)
	}
	assertCompositionBytes(t, root, before, false)
	if journal.count() != 1 {
		t.Fatal("inspection/admission replayed the diagnostic journal append")
	}
}

func TestAppUpdateDiagnosticCompositionKeepsCorruptUnknownAndPreparedDebtClosed(t *testing.T) {
	for _, kind := range []string{"corrupt-diagnostic", "unknown-leaf", "prepared-diagnostic-WAL"} {
		t.Run(kind, func(t *testing.T) {
			store, _, _, _, root := diagnosticRecordFixture(t)
			sentinel := errors.New("owned diagnostic metadata commit interruption")
			if kind == "prepared-diagnostic-WAL" {
				store.beforeWriteCommit = func(intent writeIntent) error {
					if intent.Namespace == "attempts" {
						return sentinel
					}
					return nil
				}
			}
			_, prepareErr := store.PrepareDiagnosticRequest(diagnosticRequestTestID)
			store.beforeWriteCommit = nil
			if kind == "prepared-diagnostic-WAL" {
				if !errors.Is(prepareErr, sentinel) {
					t.Fatal("fixture did not reach the actual retained prepared prefix", prepareErr)
				}
			} else if prepareErr != nil {
				t.Fatal(prepareErr)
			}
			if kind != "prepared-diagnostic-WAL" {
				name := "unowned-attempt.json"
				if kind == "corrupt-diagnostic" {
					name = diagnosticRequestTestID + diagnosticRequestSuffix
				}
				if err := os.WriteFile(filepath.Join(root, "attempts", name), []byte("{broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before := compositionAttemptBytes(t, root)
			if _, err := store.PendingAppUpdateAttempt(); err == nil || errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
				t.Fatalf("unverifiable shared history was treated as a clean namespace: %v", err)
			}
			assertCompositionBytes(t, root, before, true)
		})
	}
}
