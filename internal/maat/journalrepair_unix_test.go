//go:build !windows

package maat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileDecisionJournalRepairRefusesNameSubstitutionBeforeInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	valid := `{"time":"2026-09-26T10:01:00Z","host":"m5","kind":"reservation","requester":"alpha","assessed":"free","determination":"grant","why":"no overlap"}`
	legacy := `{"time":"2026-09-26T10:02:00Z","host":"m5","kind":"assessment","assessed":"legacy","determination":"warn","why":"missing requester"}`
	original := valid + "\n" + legacy + "\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	oldHook := journalRepairBeforeInstall
	t.Cleanup(func() { journalRepairBeforeInstall = oldHook })
	journalRepairBeforeInstall = func() {
		moved := path + ".moved-original"
		if err := os.Rename(path, moved); err != nil {
			t.Fatalf("move original inside substitution hook: %v", err)
		}
		if err := os.WriteFile(path, []byte("attacker replacement\n"), 0o600); err != nil {
			t.Fatalf("create attacker replacement inside substitution hook: %v", err)
		}
	}

	_, err := (&FileDecisionJournal{Path: path}).RepairInvalidRecords()
	if err == nil || !strings.Contains(err.Error(), "governed journal name changed") {
		t.Fatalf("repair substitution error = %v, want governed-name refusal", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "attacker replacement\n" {
		t.Fatalf("substituted journal was overwritten: %q", got)
	}
	if _, err := os.Stat(path + ".moved-original"); err != nil {
		t.Fatalf("original source should remain preserved after refused repair: %v", err)
	}
}
