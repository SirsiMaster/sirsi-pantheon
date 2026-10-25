package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestMaatReceiptCommandReadsActualRetainedDefaultStoreWithoutCreating(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	oldSHA, oldStore, oldJSON, oldGlobal := maatMemoryReceiptSHA, maatMemoryReceiptStore, maatJSON, JsonOutput
	t.Cleanup(func() {
		maatMemoryReceiptSHA, maatMemoryReceiptStore, maatJSON, JsonOutput = oldSHA, oldStore, oldJSON, oldGlobal
	})
	maatMemoryReceiptSHA, maatMemoryReceiptStore, maatJSON, JsonOutput = strings.Repeat("a", 64), "", true, false
	if err := maatFailureMemoryReceiptCmd.RunE(maatFailureMemoryReceiptCmd, nil); err == nil {
		t.Fatal("absent default custody returned success")
	}
	if _, err := os.Stat(filepath.Join(home, ".sirsi")); !os.IsNotExist(err) {
		t.Fatalf("read created default layout: %v", err)
	}
	uuid, err := maat.InitializeDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	s, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Preflight(maat.Scope{Component: "pantheon", Profile: "local-macos", Operation: "install-managed-liveness-watch"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	maatMemoryReceiptSHA = h.Reference().ReceiptSHA256
	stdout := captureStdout(t, func() {
		if err := maatFailureMemoryReceiptCmd.RunE(maatFailureMemoryReceiptCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	var result maat.PreflightReceiptRead
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result.ReceiptSHA256 != maatMemoryReceiptSHA || result.StoreBinding.StoreUUID != uuid || result.ActionAuthorized || result.Freshness != "historical" || result.RegistryRelation != "same" {
		t.Fatalf("incorrect historical wire: %+v", result)
	}
	digest, err := result.Receipt.Digest()
	if err != nil || digest != maatMemoryReceiptSHA {
		t.Fatalf("wire changed receipt bytes: %s %v", digest, err)
	}
	maatMemoryReceiptSHA = strings.Repeat("f", 64)
	stdout = captureStdout(t, func() {
		if err := maatFailureMemoryReceiptCmd.RunE(maatFailureMemoryReceiptCmd, nil); err == nil {
			t.Fatal("missing receipt returned success")
		}
	})
	var missing struct {
		ErrorCode     string `json:"error_code"`
		ReceiptSHA256 string `json:"receipt_sha256"`
	}
	if err := json.Unmarshal([]byte(stdout), &missing); err != nil || missing.ErrorCode != "historical-receipt-unavailable" || missing.ReceiptSHA256 != maatMemoryReceiptSHA {
		t.Fatalf("missing historical receipt lost truthful error: %s %v", stdout, err)
	}
}

func TestMaatPreflightInspectionDoesNotAdoptOrPopulateExplicitEmptyStore(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	old := maatMemoryStore
	t.Cleanup(func() { maatMemoryStore = old })
	maatMemoryStore = root
	if err := maatFailureMemoryPreflightCmd.RunE(maatFailureMemoryPreflightCmd, nil); err == nil {
		t.Fatal("empty explicit store silently adopted")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("inspection mutated empty root: %v %v", entries, err)
	}
}

func TestMaatFailureMemoryPreflightRequiresExplicitCasebookConfirmation(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatalf("prepare private owned memory fixture: %v", err)
	}
	store, err := maat.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := store.PutEvidence([]byte("managed label disabled"))
	if err != nil {
		t.Fatal(err)
	}
	signature := maat.FailureSignature{Namespace: "pantheon", Version: "v1", FailureClass: "launchd", Operation: "repair", Component: "router", Invariant: "enabled", ErrorCode: "disabled"}
	scope := maat.Scope{Component: "router", Profile: "macos-local", Operation: "repair"}
	incident, err := maat.NewIncident(signature, evidence, scope, maat.ActiveIncidentGuardID, maat.ActiveIncidentGuardVersion, []maat.RecoveryAction{{ID: "inspect", Label: "Inspect", Instruction: "Inspect the approved recovery evidence."}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	oldStore, oldComponent, oldProfile, oldOperation, oldConfirm, oldJSON, oldGlobalJSON, oldJournal := maatMemoryStore, maatMemoryComponent, maatMemoryProfile, maatMemoryOperation, maatMemoryConfirm, maatJSON, JsonOutput, newMaatDecisionJournal
	t.Cleanup(func() {
		maatMemoryStore, maatMemoryComponent, maatMemoryProfile, maatMemoryOperation = oldStore, oldComponent, oldProfile, oldOperation
		maatMemoryConfirm, maatJSON, JsonOutput, newMaatDecisionJournal = oldConfirm, oldJSON, oldGlobalJSON, oldJournal
	})
	journal := &capturedDecisionJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatMemoryStore, maatMemoryComponent, maatMemoryProfile, maatMemoryOperation = root, scope.Component, scope.Profile, scope.Operation
	maatMemoryConfirm, maatJSON, JsonOutput = false, true, false
	if err := maatFailureMemoryPreflightCmd.RunE(maatFailureMemoryPreflightCmd, nil); err != nil {
		t.Fatalf("preview preflight: %v", err)
	}
	if len(journal.decisions) != 0 {
		t.Fatalf("preview must not write Casebook: %+v", journal.decisions)
	}
	maatMemoryConfirm = true
	if err := maatFailureMemoryPreflightCmd.RunE(maatFailureMemoryPreflightCmd, nil); err != nil {
		t.Fatalf("confirmed preflight: %v", err)
	}
	if len(journal.decisions) != 1 || journal.decisions[0].Kind != "failure memory preflight" || journal.decisions[0].Determination != string(maat.PreflightReject) {
		t.Fatalf("confirmed preflight must project one factual decision: %+v", journal.decisions)
	}
}

func TestMaatFailureMemoryCommandCarriesExactScope(t *testing.T) {
	oldStore, oldComponent, oldProfile, oldOperation := maatMemoryStore, maatMemoryComponent, maatMemoryProfile, maatMemoryOperation
	t.Cleanup(func() {
		maatMemoryStore, maatMemoryComponent, maatMemoryProfile, maatMemoryOperation = oldStore, oldComponent, oldProfile, oldOperation
	})
	maatMemoryStore, maatMemoryComponent, maatMemoryProfile, maatMemoryOperation = "/private/tmp/maat-memory", "router", "macos local", "repair"
	got := failureMemoryCommand(true)
	for _, want := range []string{"--store '/private/tmp/maat-memory'", "--component 'router'", "--profile 'macos local'", "--operation 'repair'", "--confirm"} {
		if !strings.Contains(got, want) {
			t.Fatalf("command %q missing %q", got, want)
		}
	}
}
