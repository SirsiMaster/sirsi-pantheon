//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/spf13/cobra"
)

func TestMaatCasebookRepairHonorsRootJSONFlag(t *testing.T) {
	oldFactory, oldConfirm, oldLocal, oldGlobal := newMaatDecisionJournal, maatCasebookRepairConfirm, maatJSON, JsonOutput
	t.Cleanup(func() {
		newMaatDecisionJournal, maatCasebookRepairConfirm, maatJSON, JsonOutput = oldFactory, oldConfirm, oldLocal, oldGlobal
	})
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	valid := `{"time":"2026-10-01T00:00:00Z","host":"fixture","kind":"reservation","requester":"fixture","assessed":"fixture","determination":"grant","why":"fixture"}`
	invalid := `{"time":"2026-10-01T00:01:00Z","host":"fixture","kind":"assessment","assessed":"fixture","determination":"warn","why":"fixture"}`
	if err := os.WriteFile(path, []byte(valid+"\n"+invalid+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := &maat.FileDecisionJournal{Path: path}
	calls := 0
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { calls++; return journal, nil }
	maatJSON, JsonOutput, maatCasebookRepairConfirm = false, false, false
	root := &cobra.Command{Use: "fixture", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&JsonOutput, "json", false, "JSON output")
	parent := &cobra.Command{Use: "maat"}
	casebook := &cobra.Command{Use: "casebook"}
	casebook.Flags().BoolVar(&maatJSON, "json", false, "local JSON output")
	repair := &cobra.Command{Use: "repair", RunE: maatCasebookRepairCmd.RunE}
	repair.Flags().BoolVar(&maatCasebookRepairConfirm, "confirm", false, "confirm fixture repair")
	casebook.AddCommand(repair)
	parent.AddCommand(casebook)
	root.AddCommand(parent)
	root.SetArgs([]string{"maat", "casebook", "repair", "--confirm", "--json"})
	var executeErr error
	out := captureStdout(t, func() { executeErr = root.Execute() })
	if executeErr != nil {
		t.Fatal(executeErr)
	}
	var receipt struct {
		Status string                    `json:"status"`
		Repair maat.JournalRepairReceipt `json:"repair"`
	}
	if err := json.Unmarshal([]byte(out), &receipt); err != nil {
		t.Fatalf("root --json did not produce one repair receipt: %q: %v", out, err)
	}
	if !JsonOutput || maatJSON || calls != 1 || receipt.Status != "ok" || receipt.Repair.RemovedCount != 1 || receipt.Repair.RetainedCount != 1 || receipt.Repair.BackupPath == "" {
		t.Fatalf("root/local JSON=%t/%t, journal calls=%d, receipt=%+v", JsonOutput, maatJSON, calls, receipt)
	}
	if _, err := os.Stat(receipt.Repair.BackupPath); err != nil {
		t.Fatalf("preserved fixture backup: %v", err)
	}
}
