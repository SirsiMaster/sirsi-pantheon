package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestMaatFailureMemoryPreflightRequiresExplicitCasebookConfirmation(t *testing.T) {
	root := t.TempDir()
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
	incident, err := maat.NewIncident(signature, evidence, scope, "launchd-guard", "v1", []maat.RecoveryAction{{ID: "inspect", Label: "Inspect", Instruction: "Inspect the approved recovery evidence."}})
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
