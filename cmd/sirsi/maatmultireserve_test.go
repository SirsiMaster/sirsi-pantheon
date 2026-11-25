package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestParseExemptPIDs(t *testing.T) {
	res := []string{"m1", "m2"}
	for _, tc := range []struct {
		specs []string
		want  map[string]int
		err   bool
	}{
		{nil, map[string]int{}, false},
		{[]string{"77"}, map[string]int{"m1": 77, "m2": 77}, false},
		{[]string{"m1=11", "m2=22"}, map[string]int{"m1": 11, "m2": 22}, false},
		{[]string{"77", "m2=22"}, map[string]int{"m1": 77, "m2": 22}, false}, // named wins whatever the order
		{[]string{"m2=22", "77"}, map[string]int{"m1": 77, "m2": 22}, false},
		{[]string{"m9=1"}, nil, true},   // not being reserved
		{[]string{"m1=abc"}, nil, true}, // not a pid
		{[]string{"x"}, nil, true},
	} {
		got, err := parseExemptPIDs(tc.specs, res)
		if (err != nil) != tc.err || (!tc.err && !reflect.DeepEqual(got, tc.want)) {
			t.Fatalf("%v: got %v %v", tc.specs, got, err)
		}
	}
}

// `reserve r1 r2 --json`: one object {granted, reservations[...]}; a waiter-first
// refusal on either resource prints {granted:false, reason_code, error}, returns
// the error (non-zero exit) and reserves neither.
func TestReserveCLIMulti(t *testing.T) {
	oldLedger, oldJournal := maatLedger, newMaatDecisionJournal
	oldHolder, oldWork, oldRegime, oldStart, oldEnd := resHolder, resWork, resRegime, resStart, resEstEnd
	oldCores, oldQueue, oldJSON, oldNonce, oldWait, oldExempt := resCores, resQueue, maatJSON, maatOperationNonce, resWait, resExempt
	t.Cleanup(func() {
		maatLedger, newMaatDecisionJournal = oldLedger, oldJournal
		resHolder, resWork, resRegime, resStart, resEstEnd = oldHolder, oldWork, oldRegime, oldStart, oldEnd
		resCores, resQueue, maatJSON, maatOperationNonce, resWait, resExempt = oldCores, oldQueue, oldJSON, oldNonce, oldWait, oldExempt
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
	exempt := []string{"m5=1"}
	run := func(holder string, queue bool, resources ...string) (map[string]any, error) {
		resHolder, resWork, resRegime, resStart = holder, "w", "build", ""
		resEstEnd = now.Add(time.Hour).Format(time.RFC3339)
		resCores, resQueue, maatJSON, maatOperationNonce, resWait, resExempt = 0, queue, true, "", 0, exempt
		exempt = nil // only the first (granting) run carries one
		var runErr error
		out := captureStdout(t, func() { runErr = maatReserveCmd.RunE(maatReserveCmd, resources) })
		var got map[string]any
		if out != "" {
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("stdout must be one JSON object: %q %v", out, err)
			}
		}
		return got, runErr
	}

	got, err := run("a", false, "m5", "rail-a")
	rows, _ := got["reservations"].([]any)
	if err != nil || got["granted"] != true || len(rows) != 2 {
		t.Fatalf("grant: %v %v", got, err)
	}
	first, _ := rows[0].(map[string]any)
	if first["resource"] != "m5" || first["share"] != "full" || first["regime"] != "build" || first["id"] == "" {
		t.Fatalf("row shape: %v", first)
	}
	if held, _ := ledger.Status(""); len(held) != 2 {
		t.Fatalf("both resources reserved: %+v", held)
	}

	// --queue does not combine with several resources.
	if _, err := run("a", true, "m6", "m7"); err == nil {
		t.Fatal("--queue with several resources must be refused")
	}

	// waiter-first on rail-b refuses {m8, rail-b} whole
	win := func(resource, holder string) schedule.Reservation {
		return schedule.Reservation{Resource: resource, Holder: holder, Work: "cells", Regime: schedule.RegimeBuild,
			Start: now.Add(-time.Second).Format(time.RFC3339), EstEnd: now.Add(time.Hour).Format(time.RFC3339), LeaseTTLSec: 120}
	}
	held, err := ledger.Reserve(win("rail-b", "rep"), false)
	if err != nil || !held.Granted {
		t.Fatal(held, err)
	}
	if q, err := ledger.Reserve(win("rail-b", "waiter"), true); err != nil || !q.Queued {
		t.Fatal(q, err)
	}
	if _, err := ledger.Release(held.Reservation.ID); err != nil {
		t.Fatal(err)
	}
	before, _ := ledger.Status("")
	got, err = run("rep", false, "m8", "rail-b")
	if !errors.Is(err, schedule.ErrWaiterFirst) || got["granted"] != false || got["reason_code"] != "waiter-first" || got["error"] == "" {
		t.Fatalf("refusal: %v %v", got, err)
	}
	if rs, _ := got["reservations"].([]any); len(rs) != 0 {
		t.Fatalf("a refusal reserves nothing: %v", got)
	}
	if after, _ := ledger.Status(""); len(after) != len(before) {
		t.Fatalf("m8 must not be held after a refusal: %d -> %d rows", len(before), len(after))
	}
}
