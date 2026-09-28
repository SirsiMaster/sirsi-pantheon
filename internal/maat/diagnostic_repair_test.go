package maat

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordDiagnosticRepairRequiresPostRepairObservation(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "maat", "decisions.jsonl")}
	decision, err := RecordDiagnosticRepair(journal, "sirsi maat repair launchd-disabled", DiagnosticRepair{
		Check:         "launchd Disabled Override",
		Operation:     "enable exact managed labels",
		Before:        "two managed labels disabled",
		After:         "no recoverable managed labels disabled",
		Determination: "resolved",
		Detail:        "enabled: ai.sirsi.menubar",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != "diagnostic verified repair" || decision.Determination != "resolved" || !strings.HasPrefix(decision.Evidence, "diagnostic-repair:sha256=") {
		t.Fatalf("repair decision = %#v", decision)
	}
	rows, err := journal.Recent(1)
	if err != nil || len(rows) != 1 || rows[0].Evidence != decision.Evidence {
		t.Fatalf("recorded repair = %#v, err=%v", rows, err)
	}
}

func TestRecordDiagnosticRepairRejectsUnverifiedOrUnboundedInput(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "maat", "decisions.jsonl")}
	base := DiagnosticRepair{
		Check: "launchd Disabled Override", Operation: "bounded recovery", Before: "disabled", After: "healthy", Determination: "resolved",
	}
	if _, err := RecordDiagnosticRepair(journal, "", base); err == nil {
		t.Fatal("accepted repair without requester")
	}
	base.Determination = "ok"
	if _, err := RecordDiagnosticRepair(journal, "requester", base); err == nil {
		t.Fatal("accepted unverified determination")
	}
	base.Determination = "failed"
	base.After = strings.Repeat("x", 16*1024+1)
	if _, err := RecordDiagnosticRepair(journal, "requester", base); err == nil {
		t.Fatal("accepted oversized post-repair observation")
	}
}
