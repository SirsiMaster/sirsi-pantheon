package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func captureRepairHistoryCommand(t *testing.T) []byte {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	defer func() { os.Stdout = old; _ = reader.Close(); _ = writer.Close() }()
	os.Stdout = writer
	runErr := maatCasebookCmd.RunE(maatCasebookCmd, nil)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if runErr != nil {
		t.Fatal(runErr)
	}
	return out
}

func TestMaatCasebookBinaryRepairHistoryCommandPreservesOutcomeAndRoutes(t *testing.T) {
	for _, determination := range []string{"pending", "resolved", "failed"} {
		t.Run(determination, func(t *testing.T) {
			fixture, _, _, _ := installMaatBinaryDriftFixture(t)
			screen := fixture.decisions[0]
			screen.Time = "2026-09-01T00:00:00Z"
			journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
			if err := journal.Append(screen); err != nil {
				t.Fatal(err)
			}
			for index := 0; index < 60; index++ {
				if err := journal.Append(maat.Decision{Time: "2027-01-01T00:00:00Z", Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "noise", Determination: "grant", Why: "newer noise", Evidence: fmt.Sprintf("noise:%d", index)}); err != nil {
					t.Fatal(err)
				}
			}
			link := maat.Decision{Time: "2026-09-01T00:01:00Z", Host: "fixture", Kind: "diagnostic repair attempt", Requester: "fixture", Resource: "binary-drift", Assessed: "inert updater", Determination: "in_progress", Why: "inert attempt", Evidence: fmt.Sprintf("diagnostic-repair-attempt:sha256=%x", sha256.Sum256([]byte(screen.Evidence))), OriginEvidence: screen.Evidence}
			if err := journal.Append(link); err != nil {
				t.Fatal(err)
			}
			if determination != "pending" {
				var err error
				link, err = maat.BuildDiagnosticRepairDecision("fixture", maat.DiagnosticRepair{Check: "binary-drift", Operation: "inert updater", Before: "drift", After: "fixture observation", Determination: determination, Detail: "inert history receipt", OriginEvidence: screen.Evidence})
				if err != nil {
					t.Fatal(err)
				}
				link.Time = "2026-09-01T00:02:00Z"
				if err := journal.Append(link); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(journal.Path)
			if err != nil {
				t.Fatal(err)
			}
			oldFactory := newMaatDecisionJournal
			oldKind, oldStatus, oldLimit := maatCasebookKind, maatCasebookStatus, maatCasebookLimit
			oldScope, oldEvidence, oldCursor, oldJSON := maatCasebookScope, maatCasebookEvidence, maatCasebookCursor, maatJSON
			t.Cleanup(func() {
				newMaatDecisionJournal = oldFactory
				maatCasebookKind, maatCasebookStatus, maatCasebookLimit = oldKind, oldStatus, oldLimit
				maatCasebookScope, maatCasebookEvidence, maatCasebookCursor, maatJSON = oldScope, oldEvidence, oldCursor, oldJSON
			})
			newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
			maatCasebookKind, maatCasebookStatus, maatCasebookLimit = "", "", 1
			maatCasebookScope, maatCasebookEvidence, maatCasebookCursor, maatJSON = maat.HistoryScopeAll, screen.Evidence, "", true
			var projection maatCasebookProjection
			if err := json.Unmarshal(captureRepairHistoryCommand(t), &projection); err != nil {
				t.Fatal(err)
			}
			if len(projection.Cases) != 1 || !projection.History.EvidenceMatchesComplete || projection.History.EvidenceMatches == nil || *projection.History.EvidenceMatches != 1 || !projection.History.TotalComplete {
				t.Fatalf("incomplete exact history projection: %+v", projection)
			}
			entry := projection.Cases[0]
			switch determination {
			case "pending":
				if entry.PendingRepairAttemptEvidence != link.Evidence || entry.NextAction == nil || entry.NextAction.ActionID != "inspect_binary_drift_attempt" {
					t.Fatalf("pending CLI route = %+v", entry)
				}
			case "resolved":
				if entry.VerifiedRepairEvidence != link.Evidence || entry.RepairOutcomeEvidence != link.Evidence || entry.Status != "resolved" || entry.NextAction != nil {
					t.Fatalf("resolved CLI projection = %+v", entry)
				}
			case "failed":
				if entry.RepairOutcomeEvidence != link.Evidence || entry.VerifiedRepairEvidence != "" || entry.Status != "open" || entry.NextAction == nil || entry.NextAction.ActionID != "rescreen_system_one" {
					t.Fatalf("failed CLI route = %+v", entry)
				}
			}
			maatJSON = false
			human := string(captureRepairHistoryCommand(t))
			if !strings.Contains(human, link.Evidence) {
				t.Fatalf("human output omitted linked receipt: %s", human)
			}
			after, err := os.ReadFile(journal.Path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("Casebook query mutated the journal")
			}
		})
	}
}
