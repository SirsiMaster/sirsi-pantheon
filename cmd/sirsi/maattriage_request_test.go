package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

const triageRequestTestID = "690c358a-d905-4546-871d-de40a76cc0ac"

func TestMaatTriageRequestWholeReplyLossReadbackNeverReobservesOrAppends(t *testing.T) {
	_, _, journal, reads := triageMemoryFixture(t)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	if _, err := runTriageFixture(t, "--prepare-record-request", triageRequestTestID); err != nil {
		t.Fatal(err)
	}
	if *reads != 0 || journal.calls != 0 {
		t.Fatal("preparation ran diagnostic/journal")
	}
	if _, err := runTriageFixture(t, "--record-request", triageRequestTestID, "--confirm"); err != nil {
		t.Fatal(err)
	}
	// Deliberately discard the complete record stdout, retaining only the UUID
	// known before submission. Recovery is the actual read-only command entry.
	before := *reads
	out, err := runTriageFixture(t, "--confirm=false", "--read-record-request", triageRequestTestID)
	var lookup maat.DiagnosticRequestRead
	if err != nil || json.Unmarshal([]byte(out), &lookup) != nil || lookup.State != "recorded_verified" || lookup.Result == nil {
		t.Fatal(out, err)
	}
	if *reads != before || journal.calls != 1 {
		t.Fatal("lookup re-observed or appended")
	}
	if _, err := runTriageFixture(t, "--record-request", triageRequestTestID, "--confirm"); err == nil || *reads != before || journal.calls != 1 {
		t.Fatal("duplicate request reached observation")
	}
	list, err := runTriageFixture(t, "--confirm=false", "--inspect-record-requests")
	if err != nil || !strings.Contains(list, triageRequestTestID) || !strings.Contains(list, "recorded_verified") {
		t.Fatal(list, err)
	}
	assertDiagnosticPrivacy(t, out+list)
}

func TestMaatTriageRequestResolveFencesUnknownBeforeLateSubmission(t *testing.T) {
	_, _, journal, reads := triageMemoryFixture(t)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	if _, err := runTriageFixture(t, "--resolve-record-request", triageRequestTestID); err == nil {
		t.Fatal("unconfirmed fence accepted")
	}
	out, err := runTriageFixture(t, "--resolve-record-request", triageRequestTestID, "--confirm")
	if err != nil || !strings.Contains(out, "fenced_not_recorded") {
		t.Fatal(out, err)
	}
	if _, err := runTriageFixture(t, "--record-request", triageRequestTestID, "--confirm"); err == nil || *reads != 0 || journal.calls != 0 {
		t.Fatal("late unknown submission escaped fence")
	}
}

func TestMaatTriageRequestRejectsMixedOrMalformedOperationsBeforeObservation(t *testing.T) {
	_, _, journal, reads := triageMemoryFixture(t)
	for _, args := range [][]string{
		{"--record-request", triageRequestTestID, "--confirm=false"},
		{"--read-record-request", triageRequestTestID, "--prepare-record-request", triageRequestTestID},
		{"--record-request", "../not-a-uuid", "--confirm"},
	} {
		if _, err := runTriageFixture(t, args...); err == nil {
			t.Fatal("invalid request accepted", args)
		}
	}
	if *reads != 0 || journal.calls != 0 {
		t.Fatal("invalid request reached work")
	}
}

func TestMaatTriagePrivateRecordedHostReachesExistingBinaryDriftConsumer(t *testing.T) {
	_, report, journal, _ := triageMemoryFixture(t)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	maatTriageHostname = func() (string, error) { return "fixture-mac", nil }
	report.Findings = []guard.DiagnosticFinding{
		{Check: maatRepairBinaryDriftCheck, Severity: guard.SeverityWarn, Message: "PRIVATE_MESSAGE", Fix: "PRIVATE_FIX"},
		{Check: "Memory Death Spiral", Severity: guard.SeverityWarn},
		{Check: "/Users/PRIVATE_UNKNOWN_CHECK --token SECRET_CANARY", Severity: guard.SeverityWarn},
	}
	out, err := runTriageFixture(t, "--confirm")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		maat.MaatVerdict
		DecisionEvidence string `json:"decision_evidence"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Subject.HostBinding != maat.DiagnosticHostBinding || result.Subject.Repo == "fixture-mac" || !maat.DiagnosticHostMatches(result.Subject, "fixture-mac") {
		t.Fatal("typed private host binding missing")
	}
	if err := validateMaatBinaryDriftCase(journal, result.DecisionEvidence); err != nil {
		t.Fatal("new actual recorded verdict lost existing repair custody", err)
	}
	maatTriageHostname = func() (string, error) { return "foreign-mac", nil }
	if err := validateMaatBinaryDriftCase(journal, result.DecisionEvidence); err == nil {
		t.Fatal("foreign host accepted")
	}
	if strings.Contains(out, "PRIVATE_MESSAGE") || strings.Contains(out, "PRIVATE_FIX") || strings.Contains(out, "fixture-mac") {
		t.Fatal("private producer data leaked")
	}
	assertDiagnosticPrivacy(t, out)
	if strings.Contains(out, "PRIVATE_UNKNOWN_CHECK") || strings.Contains(out, "SECRET_CANARY") {
		t.Fatal("unknown identity leaked")
	}
	// A qualification runner may bind these ACTUAL confirmed CLI bytes into
	// the native consumer fixture. No exporter is present in product source.
	if path := os.Getenv("PANTHEON_TEST_DIAGNOSTIC_WIRE_OUTPUT"); path != "" {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.WriteString(out)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal(writeErr, closeErr)
		}
	}
}
