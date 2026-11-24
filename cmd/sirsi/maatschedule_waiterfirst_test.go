package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// A waiter-first refusal under --json prints reason_code + waiter (no
// reservation) AND still returns the error, so the exit status (1, via main) and
// the stderr line stay exactly what an ordinary refusal has.
func TestReserveCLIWaiterFirstJSONRefusalStillFailsClosed(t *testing.T) {
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
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return &capturedDecisionJournal{}, nil }

	now := time.Now().UTC()
	window := func(holder, work string) schedule.Reservation {
		return schedule.Reservation{Resource: "m5", Holder: holder, Work: work, Regime: schedule.RegimeBuild,
			Start: now.Add(-time.Second).Format(time.RFC3339), EstEnd: now.Add(time.Hour).Format(time.RFC3339), LeaseTTLSec: 120}
	}
	held, err := ledger.Reserve(window("rep", "build"), false)
	if err != nil || !held.Granted {
		t.Fatal(held, err)
	}
	if q, err := ledger.Reserve(window("waiter", "cells"), true); err != nil || !q.Queued {
		t.Fatal(q, err)
	}
	if _, err := ledger.Release(held.Reservation.ID); err != nil {
		t.Fatal(err)
	}

	resHolder, resWork, resRegime = "rep", "build-again", "build"
	resStart, resEstEnd = "", now.Add(time.Hour).Format(time.RFC3339)
	resCores, resQueue, maatJSON, maatOperationNonce = 0, false, true, ""
	var runErr error
	out := captureStdout(t, func() { runErr = maatReserveCmd.RunE(maatReserveCmd, []string{"m5"}) })
	if !errors.Is(runErr, schedule.ErrWaiterFirst) {
		t.Fatalf("the command must still return the refusal: %v", runErr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout must be one JSON object: %q %v", out, err)
	}
	w, _ := got["waiter"].(map[string]any)
	if got["reason_code"] != "waiter-first" || got["granted"] != false || w["holder"] != "waiter" || w["work"] != "cells" || w["queued_at"] == "" {
		t.Fatalf("json: %s", out)
	}
	if _, has := got["reservation"]; has {
		t.Fatalf("a refusal must carry no reservation: %s", out)
	}
	if !strings.Contains(got["error"].(string), "goes next") {
		t.Fatalf("error text: %v", got["error"])
	}
	if rows, _ := ledger.Status("m5"); len(rows) != 2 {
		t.Fatalf("nothing may be saved by the refusal: %+v", rows)
	}
}
