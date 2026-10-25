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

func TestMaatReceiptRecoveryRequiresExactConsentAndRunsNoProjectionOrInstaller(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	s, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Preflight(maat.Scope{Component: "pantheon", Profile: "local-macos", Operation: "install-managed-liveness-watch"})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	digest := stored.Reference().ReceiptSHA256
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".sirsi", "maat", "failure-memory")
	prepared, err := filepath.Glob(filepath.Join(root, "writes", "*.prepared.json"))
	if err != nil {
		t.Fatal(err)
	}
	commit := ""
	for _, path := range prepared {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var intent struct {
			Key       string `json:"key"`
			Namespace string `json:"namespace"`
			Leaf      string `json:"leaf"`
		}
		if err := json.Unmarshal(data, &intent); err != nil {
			t.Fatal(err)
		}
		if intent.Namespace == "evidence" && intent.Leaf == digest {
			if commit != "" {
				t.Fatal("duplicate fixture intent")
			}
			commit = filepath.Join(root, "writes", intent.Key+".committed.json")
		}
	}
	if commit == "" {
		t.Fatal("real retention did not produce exact fixture intent")
	}
	// Materialize the complete-target-before-commit prefix from the actual
	// canonical writer; production has no delete/reset or raw receipt input.
	if err := os.Remove(commit); err != nil {
		t.Fatal(err)
	}
	oldSHA, oldStore, oldRecover, oldConfirm := maatMemoryReceiptSHA, maatMemoryReceiptStore, maatMemoryReceiptRecover, maatMemoryReceiptConfirm
	oldJSON, oldGlobal, oldJournal, oldInstall := maatJSON, JsonOutput, newMaatDecisionJournal, maatRepairInstallLiveness
	t.Cleanup(func() {
		maatMemoryReceiptSHA, maatMemoryReceiptStore, maatMemoryReceiptRecover, maatMemoryReceiptConfirm = oldSHA, oldStore, oldRecover, oldConfirm
		maatJSON, JsonOutput, newMaatDecisionJournal, maatRepairInstallLiveness = oldJSON, oldGlobal, oldJournal, oldInstall
	})
	journalOpens, installs := 0, 0
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { journalOpens++; return &capturedDecisionJournal{}, nil }
	maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { installs++; return "unexpected fixture effect", nil }
	maatMemoryReceiptSHA, maatMemoryReceiptStore, maatJSON, JsonOutput = digest, "", true, false
	maatMemoryReceiptRecover, maatMemoryReceiptConfirm = true, false
	if err := maatFailureMemoryReceiptCmd.RunE(maatFailureMemoryReceiptCmd, nil); err == nil {
		t.Fatal("recovery ran without bound confirmation")
	}
	if _, err := os.Stat(commit); !os.IsNotExist(err) {
		t.Fatalf("unconfirmed recovery changed intent: %v", err)
	}
	maatMemoryReceiptRecover, maatMemoryReceiptConfirm = false, false
	if err := maatFailureMemoryReceiptCmd.RunE(maatFailureMemoryReceiptCmd, nil); err == nil {
		t.Fatal("ordinary read completed a prepared write")
	}
	maatMemoryReceiptRecover, maatMemoryReceiptConfirm = true, true
	stdout := captureStdout(t, func() {
		if err := maatFailureMemoryReceiptCmd.RunE(maatFailureMemoryReceiptCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	var read maat.PreflightReceiptRead
	if err := json.Unmarshal([]byte(stdout), &read); err != nil {
		t.Fatal(err)
	}
	if read.ReceiptSHA256 != digest || read.ActionAuthorized || read.Freshness != "historical" || !read.Receipt.EvaluatedAtUTC.Equal(r.EvaluatedAtUTC) {
		t.Fatalf("recovery made a false grant: %+v", read)
	}
	if _, err := os.Stat(commit); err != nil {
		t.Fatalf("exact recovery did not finish commit: %v", err)
	}
	if journalOpens != 0 || installs != 0 {
		t.Fatalf("recovery crossed action/projection boundary: journal=%d installer=%d", journalOpens, installs)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "attempts")); err != nil || len(entries) != 0 {
		t.Fatalf("recovery created action attempt: %v %v", entries, err)
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
