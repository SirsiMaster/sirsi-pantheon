package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// failingStateStore always errors, simulating an unreachable router store/ledger.
type failingStateStore struct{}

func (failingStateStore) GetState(key string) (string, bool, error) {
	return "", false, os.ErrDeadlineExceeded
}
func (failingStateStore) SetState(key, value string) error { return os.ErrDeadlineExceeded }

// No holders means CheckConflicts returns before any host activity probe.
// Reject writes: this fixture must never create a reservation.
type emptyConflictStateStore struct{}

func (emptyConflictStateStore) GetState(string) (string, bool, error) { return "", false, nil }
func (emptyConflictStateStore) SetState(string, string) error         { return os.ErrPermission }

func TestMaatConflictCheckJournalStrictCompatibility(t *testing.T) {
	oldLedger, oldMachine, oldJSON := maatLedger, conflictMachine, maatJSON
	t.Cleanup(func() { maatLedger, conflictMachine, maatJSON = oldLedger, oldMachine, oldJSON })
	conflictMachine, maatJSON = "", true
	for _, checkerFails := range []bool{false, true} {
		for _, caller := range []string{"codex-pantheon", "", " \t"} {
			name := "no-holder/" + caller
			if checkerFails {
				name = "checker-error/" + caller
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv("SIRSI_AGENT_ID", caller)
				path := filepath.Join(t.TempDir(), "decisions.jsonl")
				t.Setenv("SIRSI_MAAT_DECISIONS_PATH", path)
				maatLedger = func() (*schedule.Ledger, error) {
					if checkerFails {
						return schedule.NewLedger(failingStateStore{}), nil
					}
					return schedule.NewLedger(emptyConflictStateStore{}), nil
				}
				err := maatConflictCheckCmd.RunE(maatConflictCheckCmd, []string{"fixture-resource"})
				if checkerFails {
					if err == nil || !strings.Contains(err.Error(), "unverified") {
						t.Fatalf("checker error = %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				// Use the real strict reader, not a second fixture validator.
				rows, err := (&maat.FileDecisionJournal{Path: path}).Recent(10)
				if err != nil {
					t.Fatalf("generated projection must satisfy strict reader: %v", err)
				}
				if len(rows) != 1 {
					t.Fatalf("got %d records, want 1", len(rows))
				}
				wantRequester := "sirsi maat conflict-check"
				if caller == "codex-pantheon" {
					wantRequester = caller
				}
				wantDetermination := "clean"
				if checkerFails {
					wantDetermination = "unverified"
				}
				got := rows[0]
				if got.Requester != wantRequester || got.Affected != "" || got.Determination != wantDetermination || got.Kind != "conflict-check" {
					t.Fatalf("unexpected projection: %+v", got)
				}
			})
		}
	}
}

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
