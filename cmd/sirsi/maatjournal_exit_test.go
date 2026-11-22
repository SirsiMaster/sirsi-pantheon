package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type failingDecisionJournal struct{}

func (failingDecisionJournal) Append(maat.Decision) error { return errors.New("journal is locked") }
func (failingDecisionJournal) Recent(int) ([]maat.Decision, error) {
	return nil, nil
}

// captureStdStreams runs fn and returns what it wrote to stdout and stderr.
func captureStdStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	os.Stdout, os.Stderr = outW, errW
	fn()
	os.Stdout, os.Stderr = oldOut, oldErr
	_ = outW.Close()
	_ = errW.Close()
	o, _ := io.ReadAll(outR)
	e, _ := io.ReadAll(errR)
	return string(o), string(e)
}

func runReserveWithJournal(t *testing.T, journal maat.DecisionJournal) (rows []schedule.Reservation, stdout, stderr string, runErr error) {
	t.Helper()
	oldLedger, oldJournal := maatLedger, newMaatDecisionJournal
	oldHolder, oldWork, oldRegime, oldStart, oldEnd := resHolder, resWork, resRegime, resStart, resEstEnd
	oldCores, oldQueue, oldJSON, oldNonce := resCores, resQueue, maatJSON, maatOperationNonce
	t.Cleanup(func() {
		maatLedger, newMaatDecisionJournal = oldLedger, oldJournal
		resHolder, resWork, resRegime, resStart, resEstEnd = oldHolder, oldWork, oldRegime, oldStart, oldEnd
		resCores, resQueue, maatJSON, maatOperationNonce = oldCores, oldQueue, oldJSON, oldNonce
	})
	store, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ledger := schedule.NewLedger(store)
	maatLedger = func() (*schedule.Ledger, error) { return ledger, nil }
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	resHolder, resWork, resRegime = "journal-fixture", "journal-exit", "build"
	resStart, resEstEnd = time.Now().UTC().Format(time.RFC3339), ""
	resCores, resQueue, maatJSON, maatOperationNonce = 0, false, true, ""
	stdout, stderr = captureStdStreams(t, func() {
		runErr = maatReserveCmd.RunE(maatReserveCmd, []string{"m5"})
	})
	rows, err = ledger.Status("m5")
	if err != nil {
		t.Fatal(err)
	}
	return rows, stdout, stderr, runErr
}

// The router's grant is authoritative: a failed local journal append must not
// turn a committed reservation into a non-zero exit.
func TestReserveGrantSurvivesJournalFailure(t *testing.T) {
	rows, stdout, stderr, err := runReserveWithJournal(t, failingDecisionJournal{})
	if err != nil {
		t.Fatalf("reserve exited non-zero though the grant committed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected the committed reservation, got %+v", rows)
	}
	var out map[string]json.RawMessage
	if jerr := json.Unmarshal([]byte(stdout), &out); jerr != nil {
		t.Fatalf("stdout is not the reservation JSON: %v\n%s", jerr, stdout)
	}
	if !strings.Contains(string(out["journal_error"]), "journal is locked") {
		t.Fatalf("journal_error missing from JSON: %s", stdout)
	}
	if _, ok := out["granted"]; !ok {
		t.Fatalf("reservation fields lost: %s", stdout)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "journal is locked") {
		t.Fatalf("stderr warning missing: %q", stderr)
	}
}

// Negative control: a healthy journal prints no journal_error and no warning.
func TestReserveHealthyJournalHasNoJournalError(t *testing.T) {
	rows, stdout, stderr, err := runReserveWithJournal(t, &capturedDecisionJournal{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("healthy reserve: err=%v rows=%+v", err, rows)
	}
	if strings.Contains(stdout, "journal_error") || stderr != "" {
		t.Fatalf("spurious journal warning: stdout=%s stderr=%q", stdout, stderr)
	}
}

// Same rule for cede request: the ledger transition is authoritative.
func TestCedeRequestSurvivesJournalFailure(t *testing.T) {
	oldLedger, oldAppend, oldJSON := maatLedger, appendCedeDecision, maatJSON
	oldReq, oldHolder, oldAsk, oldMin, oldReason := cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeReason
	t.Cleanup(func() {
		maatLedger, appendCedeDecision, maatJSON = oldLedger, oldAppend, oldJSON
		cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeReason = oldReq, oldHolder, oldAsk, oldMin, oldReason
	})
	store, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ledger := schedule.NewLedger(store)
	maatLedger = func() (*schedule.Ledger, error) { return ledger, nil }
	appendCedeDecision = func(cedeDecisionRecord) error { return errors.New("journal is locked") }
	cedeRequester, cedeHolder, cedeAsk, cedeMinutes, cedeReason = "req-fixture", "holder-fixture", "machine", 10, "fixture"
	maatJSON = true
	var runErr error
	stdout, stderr := captureStdStreams(t, func() {
		runErr = maatCedeRequestCmd.RunE(maatCedeRequestCmd, []string{"m5"})
	})
	if runErr != nil {
		t.Fatalf("cede request failed though it committed: %v", runErr)
	}
	if !strings.Contains(stdout, "journal_error") || !strings.Contains(stderr, "warning:") {
		t.Fatalf("journal failure not surfaced: stdout=%s stderr=%q", stdout, stderr)
	}
}
