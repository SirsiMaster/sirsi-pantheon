package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
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

type conflictFixtureStore struct{ values map[string]string }

func (s *conflictFixtureStore) GetState(key string) (string, bool, error) {
	v, ok := s.values[key]
	return v, ok, nil
}

func (s *conflictFixtureStore) SetState(key, value string) error {
	s.values[key] = value
	return nil
}

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

func TestMaatConflictCheckReportsFloorSampleAndKeepsBothReservationsLive(t *testing.T) {
	oldLedger, oldMachine, oldJSON := maatLedger, conflictMachine, maatJSON
	t.Cleanup(func() { maatLedger, conflictMachine, maatJSON = oldLedger, oldMachine, oldJSON })
	conflictMachine, maatJSON = "", true

	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	ledger := schedule.NewLedger(&conflictFixtureStore{values: map[string]string{}}).WithClock(func() time.Time { return now })
	full, err := ledger.Reserve(schedule.Reservation{
		Resource: "m5", Holder: "mercury", Work: "quiet fixture", Regime: schedule.RegimeQuiet,
		Start: "2026-09-26T09:30:00Z", EstEnd: "2026-09-26T11:00:00Z", LeaseTTLSec: 120,
	}, false)
	if err != nil || !full.Granted || full.Reservation == nil || full.Reservation.Share != schedule.ShareFull {
		t.Fatalf("quiet holder setup: result=%+v err=%v", full, err)
	}
	floor, err := ledger.Reserve(schedule.Reservation{
		Resource: "m5", Holder: "sne", Work: "build fixture", Regime: schedule.RegimeBuild,
		Start: "2026-09-26T09:30:00Z", EstEnd: "2026-09-26T11:00:00Z", LeaseTTLSec: 120,
	}, false)
	if err != nil || !floor.Granted || floor.Reservation == nil || floor.Reservation.Share != schedule.ShareFloor {
		t.Fatalf("build floor setup: result=%+v err=%v", floor, err)
	}

	schedule.SetActivityProbe(func(string) ([]schedule.Actor, error) {
		return []schedule.Actor{
			{Kind: "bench", Detail: "quiet fixture", Owner: "mercury"},
			{Kind: "build", Detail: "build fixture", Owner: "sne"},
		}, nil
	})
	t.Cleanup(func() { schedule.SetActivityProbe(nil) })
	schedule.SetPressureFn(func() (guard.PressureLevel, string) { return guard.PressureNormal, "fixture" })
	t.Cleanup(func() { schedule.SetPressureFn(nil) })

	journalPath := filepath.Join(t.TempDir(), "decisions.jsonl")
	t.Setenv("SIRSI_MAAT_DECISIONS_PATH", journalPath)
	t.Setenv("SIRSI_AGENT_ID", "codex-pantheon")
	maatLedger = func() (*schedule.Ledger, error) { return ledger, nil }

	output, err := os.CreateTemp(t.TempDir(), "conflict-check-*.json")
	if err != nil {
		t.Fatal(err)
	}
	previousStdout := os.Stdout
	os.Stdout = output
	t.Cleanup(func() { os.Stdout = previousStdout })
	err = maatConflictCheckCmd.RunE(maatConflictCheckCmd, []string{"m5"})
	os.Stdout = previousStdout
	if err != nil {
		t.Fatalf("clean policy report must preserve the CLI success path: %v", err)
	}
	if err := output.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	var report schedule.ConflictReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("CLI JSON must decode: %v; output=%s", err, raw)
	}
	observation := report.MeasurementObservation
	if !report.Clean || observation.Status != schedule.SampleSharedFloorObserved || observation.Coverage != schedule.ObservationCoveragePointSample || len(observation.FloorShares) != 1 {
		t.Fatalf("CLI must report a clean policy result with observed floor activity: %+v", report)
	}
	share := observation.FloorShares[0]
	if share.ReservationID != floor.Reservation.ID || share.Holder != "sne" || share.Cores != floor.Reservation.Cores {
		t.Fatalf("CLI observation must identify the exact admitted floor reservation, got %+v", share)
	}

	rows, err := (&maat.FileDecisionJournal{Path: journalPath}).Recent(10)
	if err != nil {
		t.Fatalf("CLI decision must satisfy the strict journal reader: %v", err)
	}
	if len(rows) != 1 || rows[0].Determination != "clean" || !strings.Contains(rows[0].Why, "quiet interval unverified") {
		t.Fatalf("journal must preserve policy determination and scoped observation, got %+v", rows)
	}

	active, err := ledger.Status("m5")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []*schedule.Reservation{full.Reservation, floor.Reservation} {
		found := false
		for _, row := range active {
			if row.ID == expected.ID && row.Status == schedule.StatusActive {
				found = true
			}
		}
		if !found {
			t.Fatalf("CLI must leave admitted reservation %s active, got %+v", expected.ID, active)
		}
	}
	now = now.Add(30 * time.Second)
	for _, expected := range []*schedule.Reservation{full.Reservation, floor.Reservation} {
		renewed, err := ledger.Heartbeat(expected.ID)
		if err != nil || renewed.ID != expected.ID || renewed.Status != schedule.StatusActive || renewed.HeartbeatAt != now.Format(time.RFC3339) {
			t.Fatalf("CLI must leave reservation %s active and renewable: renewed=%+v err=%v", expected.ID, renewed, err)
		}
	}
}
