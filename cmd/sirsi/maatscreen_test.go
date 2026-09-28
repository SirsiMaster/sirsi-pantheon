package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestMaatScreenRecordsClosedTypedVerdict(t *testing.T) {
	input := filepath.Join(t.TempDir(), "screen.json")
	if err := os.WriteFile(input, []byte(`{
  "subject":{"kind":"commit","repo":"SirsiMaster/sirsi-pantheon","ref":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
  "feather_weight":92,
  "confidence":0.96,
  "findings":[],
  "floor":{"passed":true,"checks":[{"name":"gofmt","passed":true}]},
  "model":{"provider":"local:deterministic","version":"v1","local":true,"latency_ms":2}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	oldFactory, oldInput, oldConfirm, oldJSON, oldMaatJSON := newMaatDecisionJournal, maatScreenInput, maatScreenConfirm, JsonOutput, maatJSON
	t.Cleanup(func() {
		newMaatDecisionJournal, maatScreenInput, maatScreenConfirm, JsonOutput, maatJSON = oldFactory, oldInput, oldConfirm, oldJSON, oldMaatJSON
	})
	journal := &maatTestJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatScreenInput, maatScreenConfirm, JsonOutput, maatJSON = input, true, true, false
	if err := maatScreenCmd.RunE(maatScreenCmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(journal.decisions) != 1 || journal.decisions[0].SystemOne == nil || journal.decisions[0].SystemOne.Gate != maat.GatePass {
		t.Fatalf("screen decisions = %+v", journal.decisions)
	}
}

func TestMaatScreenRejectsUnknownOrTrailingInput(t *testing.T) {
	input := filepath.Join(t.TempDir(), "screen.json")
	if err := os.WriteFile(input, []byte(`{"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	oldInput, oldConfirm := maatScreenInput, maatScreenConfirm
	t.Cleanup(func() { maatScreenInput, maatScreenConfirm = oldInput, oldConfirm })
	maatScreenInput, maatScreenConfirm = input, true
	if err := maatScreenCmd.RunE(maatScreenCmd, nil); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown input error = %v", err)
	}
}

func TestMaatScreenRequiresConfirmationBeforeJournalWrite(t *testing.T) {
	input := filepath.Join(t.TempDir(), "screen.json")
	if err := os.WriteFile(input, []byte(`{
  "subject":{"kind":"commit","repo":"SirsiMaster/sirsi-pantheon","ref":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
  "feather_weight":92,
  "confidence":0.96,
  "findings":[],
  "floor":{"passed":true,"checks":[{"name":"gofmt","passed":true}]},
  "model":{"provider":"local:deterministic","version":"v1","local":true,"latency_ms":2}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	oldFactory, oldInput, oldConfirm := newMaatDecisionJournal, maatScreenInput, maatScreenConfirm
	t.Cleanup(func() { newMaatDecisionJournal, maatScreenInput, maatScreenConfirm = oldFactory, oldInput, oldConfirm })
	journal := &maatTestJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatScreenInput, maatScreenConfirm = input, false
	err := maatScreenCmd.RunE(maatScreenCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("missing confirmation error = %v", err)
	}
	if len(journal.decisions) != 0 {
		t.Fatalf("screen without confirmation wrote journal decisions: %+v", journal.decisions)
	}
}
