package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/billingevidence"
)

func TestBillingEvidenceCommandMissingObservation(t *testing.T) {
	cmd := newBillingEvidenceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report billingevidence.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "unknown" || report.AvailableBudget != nil || report.Enforcement != "none" {
		t.Fatalf("invented budget: %+v", report)
	}
}

func TestBillingEvidenceCommandRejectsMissingFileAndInvalidAge(t *testing.T) {
	for _, args := range [][]string{{t.TempDir() + "/absent.json"}, {"--max-age", "0s"}} {
		cmd := newBillingEvidenceCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
