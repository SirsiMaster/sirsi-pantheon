package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// failingStateStore always errors, simulating an unreachable router store/ledger.
type failingStateStore struct{}

func (failingStateStore) GetState(key string) (string, bool, error) {
	return "", false, os.ErrDeadlineExceeded
}
func (failingStateStore) SetState(key, value string) error { return os.ErrDeadlineExceeded }

func TestMaatConflictCheckClassifiesCheckerFailureAsUnverifiedNotFail(t *testing.T) {
	oldLedger, oldMachine, oldJSON := maatLedger, conflictMachine, maatJSON
	t.Cleanup(func() { maatLedger, conflictMachine, maatJSON = oldLedger, oldMachine, oldJSON })
	maatLedger = func() (*schedule.Ledger, error) {
		return schedule.NewLedger(failingStateStore{}), nil
	}
	conflictMachine, maatJSON = "", false

	decisionsPath := filepath.Join(t.TempDir(), "decisions.jsonl")
	oldDecisionsPath := os.Getenv("SIRSI_MAAT_DECISIONS_PATH")
	if err := os.Setenv("SIRSI_MAAT_DECISIONS_PATH", decisionsPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Setenv("SIRSI_MAAT_DECISIONS_PATH", oldDecisionsPath) })

	err := maatConflictCheckCmd.RunE(maatConflictCheckCmd, []string{"m1"})
	if err == nil {
		t.Fatal("conflict-check against a failing store must error")
	}
	if !strings.Contains(err.Error(), "unverified") {
		t.Fatalf("error must say the determination is unverified (not a confirmed conflict), got %q", err)
	}

	raw, rerr := os.ReadFile(decisionsPath)
	if rerr != nil {
		t.Fatalf("decision must be recorded even on checker failure: %v", rerr)
	}
	if !strings.Contains(string(raw), `"determination":"unverified"`) {
		t.Fatalf("decision log must record determination=unverified, got %q", raw)
	}
	if strings.Contains(string(raw), `"determination":"conflict"`) || strings.Contains(string(raw), `"determination":"clean"`) {
		t.Fatalf("a checker failure must never be recorded as clean/conflict, got %q", raw)
	}
}
