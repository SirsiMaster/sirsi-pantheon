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
	oldFactory, oldInput, oldJSON, oldMaatJSON := newMaatDecisionJournal, maatScreenInput, JsonOutput, maatJSON
	t.Cleanup(func() {
		newMaatDecisionJournal, maatScreenInput, JsonOutput, maatJSON = oldFactory, oldInput, oldJSON, oldMaatJSON
	})
	journal := &maatTestJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatScreenInput, JsonOutput, maatJSON = input, true, false
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
	oldInput := maatScreenInput
	t.Cleanup(func() { maatScreenInput = oldInput })
	maatScreenInput = input
	if err := maatScreenCmd.RunE(maatScreenCmd, nil); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown input error = %v", err)
	}
}
